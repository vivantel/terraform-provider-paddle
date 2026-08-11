// Package client is a minimal HTTP client for the Paddle Billing API
// (https://developer.paddle.com/api-reference/overview). It only implements
// the Products, Prices, Discounts, Discount Groups, Notification Settings,
// Checkout Domains, Adjustments, and Subscriptions (actions only, no full
// CRUD — see docs/decisions/0010-v3-scope-lifecycle-actions.md) endpoints
// this provider currently needs.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	SandboxBaseURL    = "https://sandbox-api.paddle.com"
	ProductionBaseURL = "https://api.paddle.com"

	// defaultTimeout bounds a single request. http.DefaultClient has no
	// timeout at all, so a stalled connection to Paddle would otherwise hang
	// a terraform apply indefinitely — Terraform doesn't impose its own
	// deadline on provider RPCs.
	defaultTimeout = 30 * time.Second
)

// Retry tuning. Package vars, not consts, so tests can shrink them for
// speed rather than a production-realistic test suite taking 10+ seconds
// asserting backoff timing.
var (
	retryMaxAttempts = 5
	retryBaseBackoff = 500 * time.Millisecond
	retryMaxBackoff  = 10 * time.Second
	// retryMaxRetryAfter caps how long a server-supplied Retry-After header
	// can make us wait, independent of retryMaxBackoff — a legitimate
	// heavy-throttling response might reasonably ask for longer than our
	// own exponential ceiling, but an unbounded wait on external input is
	// still worth capping defensively.
	retryMaxRetryAfter = 30 * time.Second
	// retryOverallBudget bounds the whole do() call — every attempt, every
	// backoff wait, combined — not just each individual HTTP request.
	// Without this, a persistently *slow* (not fast-failing) backend could
	// block a single call for minutes: up to retryMaxAttempts *
	// defaultTimeout of request time, plus backoff, all sequential.
	// context.WithTimeout always takes the earlier of two deadlines, so
	// wrapping with this budget only ever tightens an already-bounded
	// caller context — it never loosens an unbounded one beyond this.
	retryOverallBudget = 60 * time.Second
)

type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL:    baseURL,
		APIKey:     apiKey,
		HTTPClient: &http.Client{Timeout: defaultTimeout},
	}
}

// APIError is returned when Paddle responds with a non-2xx status. Paddle's
// error envelope is {"error": {"type": "...", "code": "...", "detail": "..."},
// "meta": {"request_id": "..."}} — surfaced verbatim rather than parsed further,
// since the exact error shape varies more than the success shape does.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("paddle API error (status %d): %s", e.StatusCode, e.Body)
}

// IsNotFound reports whether err is a *APIError for a 404 response,
// unwrapping as needed. Shared by every resource's Read() (an object
// deleted outside Terraform should be dropped from state, not error) and
// Delete() (archiving an already-gone object should succeed, not fail) —
// previously each Read() reimplemented this identically with a magic 404,
// and Delete() didn't check it at all.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

// FriendlyErrorMessage returns a human-readable message for err, meant for
// surfacing directly in a Terraform diagnostic — resources call this
// instead of err.Error() when adding a Diagnostics error for anything that
// came back from this client. If err is (or wraps) an *APIError whose Body
// parses into Paddle's documented {"error":{"code":...,"detail":...}}
// envelope, this returns the detail (with the code appended in
// parentheses, if present) — "currency_code must be a valid ISO 4217 code
// (invalid_currency_code)" instead of the raw JSON blob. Any other case —
// a non-APIError, malformed JSON, or JSON that doesn't match the
// documented shape — falls back to err.Error() unchanged. This is
// deliberately best-effort: the APIError type's own comment already notes
// the exact error shape varies more than the success shape does, so this
// must fail safe to the existing behavior rather than risk surfacing an
// empty or misleading message when a response doesn't match what's
// expected.
func FriendlyErrorMessage(err error) string {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return err.Error()
	}
	var envelope struct {
		Error struct {
			Code   string `json:"code"`
			Detail string `json:"detail"`
		} `json:"error"`
	}
	if jsonErr := json.Unmarshal([]byte(apiErr.Body), &envelope); jsonErr != nil || envelope.Error.Detail == "" {
		return err.Error()
	}
	if envelope.Error.Code != "" {
		return fmt.Sprintf("%s (%s)", envelope.Error.Detail, envelope.Error.Code)
	}
	return envelope.Error.Detail
}

// do sends a request, retrying on 429 (rate limited) and 5xx (transient
// upstream failure) with bounded exponential backoff, within an overall
// budget across the whole call (see retryOverallBudget). A Retry-After
// header on a 429 takes precedence over the computed backoff when present.
// Any other non-2xx status, or the final attempt's failure, returns
// *APIError unchanged — callers that check for a 404 via IsNotFound (see
// every resource's Read()/Delete()) don't need to change.
//
// Logs at tflog.Debug level (visible via TF_LOG=debug) — method, path,
// attempt number, and response status only. Request/response bodies are
// deliberately never logged: custom_data or other fields may contain data
// a user considers sensitive, and the API key must never appear in a log
// line at any level (it's set as a header directly on req, never passed
// to a logging call).
func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	ctx, cancel := context.WithTimeout(ctx, retryOverallBudget)
	defer cancel()

	bodyBytes, err := marshalBody(body)
	if err != nil {
		return err
	}

	// No upper bound on the loop header itself — every branch inside
	// already returns explicitly (including the attempt == retryMaxAttempts
	// case below), so an infinite `for` here is exactly as bounded in
	// practice as `for attempt := 1; attempt <= retryMaxAttempts; attempt++`
	// was, but without needing a vestigial `return` after the loop just to
	// satisfy the compiler's control-flow analysis of a bound it can't
	// prove is never exceeded.
	for attempt := 1; ; attempt++ {
		result, err := c.doOnce(ctx, method, path, bodyBytes, out, attempt)
		if err != nil {
			return err
		}
		if result == nil {
			return nil // success, out already decoded by doOnce
		}
		if !isRetryableStatus(result.apiErr.StatusCode) || attempt == retryMaxAttempts {
			return result.apiErr
		}
		if err := waitBeforeRetry(ctx, attempt, result.retryAfterHeader); err != nil {
			return err
		}
	}
}

// doNoRetryResult is the outcome of one non-2xx HTTP round trip inside
// doOnce — nil means success (out already decoded).
type doNoRetryResult struct {
	apiErr           *APIError
	retryAfterHeader string
}

// doOnce performs exactly one HTTP round trip: build the request from
// bodyBytes (already marshaled once by the caller, so a retry loop doesn't
// re-marshal and each attempt gets a fresh io.Reader over the same bytes),
// send it, and on a 2xx response decode into out. Returns (nil, nil) on
// success. Returns a non-nil *doNoRetryResult (despite the name — this
// helper itself has no retry policy, its callers do) for any non-2xx
// response, leaving the retry/no-retry decision to the caller. Returns a
// non-nil error only for failures that never produced an inspectable HTTP
// response at all (marshal/build/transport/decode failures).
//
// Shared by do() (loops this with retry/backoff on retryable statuses) and
// doNoRetry() (calls it exactly once) so both stay behaviorally identical
// about request construction, logging, and response decoding — only retry
// policy differs between the two callers.
func (c *Client) doOnce(ctx context.Context, method, path string, bodyBytes []byte, out any, attempt int) (*doNoRetryResult, error) {
	var reqBody io.Reader
	if bodyBytes != nil {
		// A fresh Reader per attempt: bytes.Reader is consumed after one
		// send, and body must survive being sent again on retry.
		reqBody = bytes.NewReader(bodyBytes)
	}

	tflog.Debug(ctx, "paddle: sending request", map[string]any{
		"method":  method,
		"path":    path,
		"attempt": attempt,
	})

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		// Transport-level errors (DNS, connection refused, context
		// cancellation) aren't retried — only HTTP responses we can
		// actually inspect a status code on are.
		tflog.Debug(ctx, "paddle: request failed before a response", map[string]any{
			"method": method, "path": path, "attempt": attempt, "error": err.Error(),
		})
		return nil, fmt.Errorf("do request: %w", err)
	}

	respBody, err := io.ReadAll(resp.Body)
	// The body is already fully read above (or io.ReadAll's own error is
	// what's returned below); a Close() failure here has nothing left to
	// lose and nothing meaningful to do about — the read result already
	// determines this attempt's outcome.
	_ = resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	tflog.Debug(ctx, "paddle: received response", map[string]any{
		"method": method, "path": path, "attempt": attempt, "status": resp.StatusCode,
	})

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &doNoRetryResult{
			apiErr:           &APIError{StatusCode: resp.StatusCode, Body: string(respBody)},
			retryAfterHeader: resp.Header.Get("Retry-After"),
		}, nil
	}

	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return nil, fmt.Errorf("unmarshal response body: %w", err)
		}
	}
	return nil, nil
}

func marshalBody(body any) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}
	return b, nil
}

func isRetryableStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode >= 500
}

// NonRetryableError wraps a failure from doNoRetry — an ambiguous outcome
// (a transport-level failure, or a 429/5xx response) from a single-attempt,
// no-retry call. Used for Paddle endpoints that create a financial
// adjustment or change live subscription state
// (docs/guardrails/money-moving-actions-no-blanket-retry.md): Paddle has no
// idempotency-key mechanism, so blindly retrying an ambiguous failure
// (request sent, response lost to a timeout/5xx) risks double-executing a
// real financial operation. Distinct from a plain *APIError so callers can
// tell "Paddle definitively rejected this" (a clean non-retryable 4xx,
// nothing happened, safe to fix and reapply) apart from "the outcome is
// unknown" (this type — verify against Paddle directly before trying
// again). A clean 4xx from doNoRetry is returned as a plain *APIError, not
// wrapped — see doNoRetry's own comment.
type NonRetryableError struct {
	// Err is the underlying failure: an *APIError with a retryable status
	// (429/5xx — Paddle responded, just not conclusively), or a
	// transport-level error (no response at all — the most ambiguous case,
	// since the request may or may not have reached Paddle).
	Err error
}

func (e *NonRetryableError) Error() string {
	return fmt.Sprintf("paddle: ambiguous failure, verify the actual outcome against Paddle directly before retrying: %s", e.Err)
}

func (e *NonRetryableError) Unwrap() error {
	return e.Err
}

// doNoRetry sends a request exactly once — no retry on 429/5xx, unlike
// do(). Required for any Paddle endpoint that isn't safe to blindly repeat
// (docs/guardrails/money-moving-actions-no-blanket-retry.md): a refund,
// credit, or subscription state change.
//
// A transport-level failure or a retryable-status response (429/5xx —
// Paddle may have processed the request before failing to respond
// cleanly, so the outcome is genuinely unknown) is wrapped in
// *NonRetryableError, so the caller (an action's Invoke()) can surface a
// distinct message telling the user to verify the actual outcome against
// Paddle before trying again, rather than silently retrying (that's what
// do() is for, and it must not be used for these calls). A clean,
// non-retryable 4xx (e.g. a validation error) is returned as a plain
// *APIError, unwrapped — Paddle definitively rejected the request, nothing
// happened, nothing ambiguous about it, safe to fix and reapply like any
// other *APIError from do().
func (c *Client) doNoRetry(ctx context.Context, method, path string, body any, out any) error {
	ctx, cancel := context.WithTimeout(ctx, retryOverallBudget)
	defer cancel()

	bodyBytes, err := marshalBody(body)
	if err != nil {
		return err
	}

	result, err := c.doOnce(ctx, method, path, bodyBytes, out, 1)
	if err != nil {
		return &NonRetryableError{Err: err}
	}
	if result == nil {
		return nil // success, out already decoded
	}
	if isRetryableStatus(result.apiErr.StatusCode) {
		return &NonRetryableError{Err: result.apiErr}
	}
	return result.apiErr
}

// waitBeforeRetry sleeps for the retry delay, or returns early with ctx's
// error if it's cancelled/times out during the wait — a caller that gave
// up shouldn't be kept waiting through a multi-second backoff.
func waitBeforeRetry(ctx context.Context, attempt int, retryAfterHeader string) error {
	d := backoffDelay(attempt)
	if ra, ok := parseRetryAfter(retryAfterHeader); ok {
		d = ra
	}
	// Even a zero/negative delay must still check ctx first — an
	// already-cancelled context shouldn't let one more request through
	// just because there was nothing to sleep for.
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if d <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// backoffDelay computes a full-jitter exponential delay for the given
// 1-indexed attempt number: a random duration in [0, min(base*2^(n-1), max)).
func backoffDelay(attempt int) time.Duration {
	d := retryBaseBackoff * time.Duration(1<<uint(attempt-1))
	if d > retryMaxBackoff || d <= 0 {
		d = retryMaxBackoff
	}
	return time.Duration(rand.Int63n(int64(d)))
}

// parseRetryAfter parses a Retry-After header per RFC 7231 §7.1.3 — either
// an integer number of seconds, or an HTTP-date. Returns ok=false if the
// header is absent or unparsable, so the caller falls back to the computed
// exponential backoff.
func parseRetryAfter(header string) (time.Duration, bool) {
	if header == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(header); err == nil {
		if secs < 0 {
			return 0, false
		}
		d := time.Duration(secs) * time.Second
		if d > retryMaxRetryAfter {
			d = retryMaxRetryAfter
		}
		return d, true
	}
	if t, err := http.ParseTime(header); err == nil {
		d := time.Until(t)
		if d <= 0 {
			return 0, true
		}
		if d > retryMaxRetryAfter {
			d = retryMaxRetryAfter
		}
		return d, true
	}
	return 0, false
}

// paginationMeta is the shared shape of Paddle list endpoints' `meta`
// object (docs/decisions/0009 — only the `has_more` field is needed here,
// per_page/estimated_total/next aren't used by anything in this client).
type paginationMeta struct {
	Pagination struct {
		HasMore bool `json:"has_more"`
	} `json:"pagination"`
}

// listPath builds a list-endpoint path with Paddle's `after` cursor
// (the ID of the last item from the previous page) and a large per_page to
// minimize round trips — used only by List* methods below, all of which
// exist for acceptance test sweepers, not resource/data-source CRUD.
func listPath(base, after string) string {
	if after == "" {
		return base + "?per_page=200"
	}
	return base + "?per_page=200&after=" + after
}

// ── Products — https://developer.paddle.com/api-reference/products ─────────

type Product struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	TaxCategory string `json:"tax_category"`
	// Description and ImageURL deliberately lack `omitempty`: a nil pointer
	// must marshal as an explicit JSON null so PATCH can clear a
	// previously-set value, rather than omitting the field (which Paddle
	// interprets as "leave unchanged").
	Description *string        `json:"description"`
	Type        string         `json:"type,omitempty"`
	ImageURL    *string        `json:"image_url"`
	CustomData  map[string]any `json:"custom_data,omitempty"`
	Status      string         `json:"status,omitempty"`
}

// statusPatch is a minimal PATCH body used for archiving. It must not reuse
// Product/Price directly: those structs' required fields (Name, TaxCategory,
// ProductID, Description, UnitPrice) have no omitempty, so building one from
// a zero-value struct would send empty-string values for them — Paddle
// rejects an empty name/tax_category outright, and would otherwise silently
// blank required-looking fields on price archival.
type statusPatch struct {
	Status string `json:"status"`
}

type productEnvelope struct {
	Data Product `json:"data"`
}

func (c *Client) CreateProduct(ctx context.Context, p Product) (*Product, error) {
	var env productEnvelope
	if err := c.do(ctx, http.MethodPost, "/products", p, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

func (c *Client) GetProduct(ctx context.Context, id string) (*Product, error) {
	var env productEnvelope
	if err := c.do(ctx, http.MethodGet, "/products/"+id, nil, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

func (c *Client) UpdateProduct(ctx context.Context, id string, p Product) (*Product, error) {
	var env productEnvelope
	if err := c.do(ctx, http.MethodPatch, "/products/"+id, p, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

// Paddle has no hard-delete for products — archiving (status = "archived")
// is the only supported removal. Terraform Delete calls this.
func (c *Client) ArchiveProduct(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPatch, "/products/"+id, statusPatch{Status: "archived"}, nil)
}

type productListEnvelope struct {
	Data []Product      `json:"data"`
	Meta paginationMeta `json:"meta"`
}

// ListProducts fetches every product, paging through Paddle's cursor-based
// pagination (docs/decisions/0009's sweeper support — nothing else in this
// provider needs a full listing today). Not used by any resource/data
// source, only by acceptance test sweepers.
func (c *Client) ListProducts(ctx context.Context) ([]Product, error) {
	var all []Product
	after := ""
	for {
		var env productListEnvelope
		if err := c.do(ctx, http.MethodGet, listPath("/products", after), nil, &env); err != nil {
			return nil, err
		}
		all = append(all, env.Data...)
		if len(env.Data) == 0 || !env.Meta.Pagination.HasMore {
			return all, nil
		}
		after = env.Data[len(env.Data)-1].ID
	}
}

// ── Prices — https://developer.paddle.com/api-reference/prices ─────────────

type Money struct {
	Amount       string `json:"amount"`
	CurrencyCode string `json:"currency_code"`
}

type BillingCycle struct {
	Interval  string `json:"interval"`
	Frequency int64  `json:"frequency"`
}

type Quantity struct {
	Minimum int64 `json:"minimum,omitempty"`
	Maximum int64 `json:"maximum,omitempty"`
}

type Price struct {
	ID          string `json:"id,omitempty"`
	ProductID   string `json:"product_id"`
	Description string `json:"description"`
	UnitPrice   Money  `json:"unit_price"`
	Type        string `json:"type,omitempty"`
	// Name deliberately lacks `omitempty` — see the comment on
	// Product.Description; a nil pointer must marshal as an explicit null
	// to clear a previously-set value via PATCH.
	Name         *string        `json:"name"`
	BillingCycle *BillingCycle  `json:"billing_cycle,omitempty"`
	Quantity     *Quantity      `json:"quantity,omitempty"`
	TaxMode      string         `json:"tax_mode,omitempty"`
	CustomData   map[string]any `json:"custom_data,omitempty"`
	Status       string         `json:"status,omitempty"`
}

// PriceUpdate is the PATCH body for updating a price. It deliberately
// excludes ProductID: confirmed against the real sandbox API (not just
// inferred), Paddle rejects the update outright with "Additional property
// product_id is not allowed" if it's present at all — this isn't "changes
// to product_id are rejected," it's "the field can't appear in this
// endpoint's payload, changed or not." Reusing Price for update bodies (as
// an earlier version of this client did) breaks every price update.
type PriceUpdate struct {
	Description string `json:"description"`
	UnitPrice   Money  `json:"unit_price"`
	Type        string `json:"type,omitempty"`
	// Name deliberately lacks `omitempty` — see the comment on
	// Product.Description; a nil pointer must marshal as an explicit null
	// to clear a previously-set value via PATCH.
	Name         *string        `json:"name"`
	BillingCycle *BillingCycle  `json:"billing_cycle,omitempty"`
	Quantity     *Quantity      `json:"quantity,omitempty"`
	TaxMode      string         `json:"tax_mode,omitempty"`
	CustomData   map[string]any `json:"custom_data,omitempty"`
	Status       string         `json:"status,omitempty"`
}

type priceEnvelope struct {
	Data Price `json:"data"`
}

func (c *Client) CreatePrice(ctx context.Context, p Price) (*Price, error) {
	var env priceEnvelope
	if err := c.do(ctx, http.MethodPost, "/prices", p, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

func (c *Client) GetPrice(ctx context.Context, id string) (*Price, error) {
	var env priceEnvelope
	if err := c.do(ctx, http.MethodGet, "/prices/"+id, nil, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

func (c *Client) UpdatePrice(ctx context.Context, id string, p PriceUpdate) (*Price, error) {
	var env priceEnvelope
	if err := c.do(ctx, http.MethodPatch, "/prices/"+id, p, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

// Same story as products — archive, not delete.
func (c *Client) ArchivePrice(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPatch, "/prices/"+id, statusPatch{Status: "archived"}, nil)
}

type priceListEnvelope struct {
	Data []Price        `json:"data"`
	Meta paginationMeta `json:"meta"`
}

// ListPrices — see ListProducts' comment; same pagination shape, same
// sweeper-only purpose.
func (c *Client) ListPrices(ctx context.Context) ([]Price, error) {
	var all []Price
	after := ""
	for {
		var env priceListEnvelope
		if err := c.do(ctx, http.MethodGet, listPath("/prices", after), nil, &env); err != nil {
			return nil, err
		}
		all = append(all, env.Data...)
		if len(env.Data) == 0 || !env.Meta.Pagination.HasMore {
			return all, nil
		}
		after = env.Data[len(env.Data)-1].ID
	}
}

// ── Discounts — https://developer.paddle.com/api-reference/discounts ───────
//
// Field list and update-immutability confirmed directly against
// https://developer.paddle.com/api-reference/discounts/create-discount and
// .../update-discount (2026-08-08), not guessed. Unlike Price, Discount's
// update endpoint accepts the same field set as create plus `status` — no
// field is rejected outright the way Price rejects product_id on update —
// so a single struct covers both request bodies; id/times_used/created_at/
// updated_at/import_meta are create-and-update-immutable (Paddle sets them)
// and use `omitempty` so a zero-value Go field never sends them.

type Discount struct {
	ID          string `json:"id,omitempty"`
	Description string `json:"description"`
	// Type: "flat", "flat_per_seat", or "percentage".
	Type string `json:"type"`
	// Amount: "0.01"-"100" for percentage, lowest currency denomination
	// for flat/flat_per_seat.
	Amount string `json:"amount"`
	// Code, CurrencyCode, MaximumRecurringIntervals, UsageLimit, RestrictTo,
	// ExpiresAt, and DiscountGroupID all deliberately lack `omitempty` —
	// same reasoning as Product.Description: Paddle docs mark them
	// nullable, and a nil pointer/nil slice must marshal as explicit null
	// so PATCH can clear a previously-set value rather than silently
	// leaving it unchanged (see docs/decisions/0006-unit-tests-for-pure-logic.md's
	// account of the same class of bug on Product/Price).
	Code                      *string        `json:"code"`
	EnabledForCheckout        *bool          `json:"enabled_for_checkout,omitempty"`
	Mode                      string         `json:"mode,omitempty"`
	CurrencyCode              *string        `json:"currency_code"`
	Recur                     *bool          `json:"recur,omitempty"`
	MaximumRecurringIntervals *int           `json:"maximum_recurring_intervals"`
	UsageLimit                *int           `json:"usage_limit"`
	RestrictTo                []string       `json:"restrict_to"`
	ExpiresAt                 *string        `json:"expires_at"`
	CustomData                map[string]any `json:"custom_data,omitempty"`
	DiscountGroupID           *string        `json:"discount_group_id"`
	Status                    string         `json:"status,omitempty"`
	// Read-only, Paddle-assigned — never sent in a request body regardless
	// of Go zero-value, since all three have omitempty.
	TimesUsed int    `json:"times_used,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type discountEnvelope struct {
	Data Discount `json:"data"`
}

func (c *Client) CreateDiscount(ctx context.Context, d Discount) (*Discount, error) {
	var env discountEnvelope
	if err := c.do(ctx, http.MethodPost, "/discounts", d, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

func (c *Client) GetDiscount(ctx context.Context, id string) (*Discount, error) {
	var env discountEnvelope
	if err := c.do(ctx, http.MethodGet, "/discounts/"+id, nil, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

func (c *Client) UpdateDiscount(ctx context.Context, id string, d Discount) (*Discount, error) {
	var env discountEnvelope
	if err := c.do(ctx, http.MethodPatch, "/discounts/"+id, d, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

// Paddle has no delete operation for discounts at all (not even the
// archive-via-update pattern Product/Price happen to share a name with by
// convention) — the docs are explicit: "There's no delete operation for
// discounts." Archiving is just a normal UpdateDiscount call with
// status: "archived", so this only exists for symmetry with
// ArchiveProduct/ArchivePrice and to keep the archive-body-shape reasoning
// (see statusPatch) in one place — it does not hit a different endpoint.
func (c *Client) ArchiveDiscount(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPatch, "/discounts/"+id, statusPatch{Status: "archived"}, nil)
}

type discountListEnvelope struct {
	Data []Discount     `json:"data"`
	Meta paginationMeta `json:"meta"`
}

// ListDiscounts — see ListProducts' comment; same pagination shape, same
// sweeper-only purpose.
func (c *Client) ListDiscounts(ctx context.Context) ([]Discount, error) {
	var all []Discount
	after := ""
	for {
		var env discountListEnvelope
		if err := c.do(ctx, http.MethodGet, listPath("/discounts", after), nil, &env); err != nil {
			return nil, err
		}
		all = append(all, env.Data...)
		if len(env.Data) == 0 || !env.Meta.Pagination.HasMore {
			return all, nil
		}
		after = env.Data[len(env.Data)-1].ID
	}
}

// ── Discount Groups — https://developer.paddle.com/api-reference/discount-groups ─

type DiscountGroup struct {
	ID     string `json:"id,omitempty"`
	Name   string `json:"name"`
	Status string `json:"status,omitempty"`
}

type discountGroupEnvelope struct {
	Data DiscountGroup `json:"data"`
}

func (c *Client) CreateDiscountGroup(ctx context.Context, g DiscountGroup) (*DiscountGroup, error) {
	var env discountGroupEnvelope
	if err := c.do(ctx, http.MethodPost, "/discount-groups", g, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

func (c *Client) GetDiscountGroup(ctx context.Context, id string) (*DiscountGroup, error) {
	var env discountGroupEnvelope
	if err := c.do(ctx, http.MethodGet, "/discount-groups/"+id, nil, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

func (c *Client) UpdateDiscountGroup(ctx context.Context, id string, g DiscountGroup) (*DiscountGroup, error) {
	var env discountGroupEnvelope
	if err := c.do(ctx, http.MethodPatch, "/discount-groups/"+id, g, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

// Same story as Product/Price/Discount — no separate delete operation,
// archiving via update is the only removal path (confirmed against the
// real API reference, docs/decisions/0007).
func (c *Client) ArchiveDiscountGroup(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPatch, "/discount-groups/"+id, statusPatch{Status: "archived"}, nil)
}

type discountGroupListEnvelope struct {
	Data []DiscountGroup `json:"data"`
	Meta paginationMeta  `json:"meta"`
}

// ListDiscountGroups — see ListProducts' comment; same pagination shape,
// same sweeper-only purpose.
func (c *Client) ListDiscountGroups(ctx context.Context) ([]DiscountGroup, error) {
	var all []DiscountGroup
	after := ""
	for {
		var env discountGroupListEnvelope
		if err := c.do(ctx, http.MethodGet, listPath("/discount-groups", after), nil, &env); err != nil {
			return nil, err
		}
		all = append(all, env.Data...)
		if len(env.Data) == 0 || !env.Meta.Pagination.HasMore {
			return all, nil
		}
		after = env.Data[len(env.Data)-1].ID
	}
}

// ── Notification Settings — https://developer.paddle.com/api-reference/notification-settings ─
//
// Unlike Product/Price/Discount/Discount Group, this entity has a real
// hard DELETE endpoint (confirmed against the real API reference,
// docs/decisions/0007) — no Archive*/statusPatch pattern applies here.
//
// The request and response shapes for subscribed_events genuinely differ
// (confirmed against the real API reference, not assumed): a create/update
// request sends it as a plain array of event type name strings, but every
// response (create, update, and get) returns it as an array of event
// objects ({name, description, group, available_versions}) — the same
// asymmetry Price's product_id has between create and update, just on a
// field's shape instead of a field's presence. NotificationSetting (the
// response/entity shape) and NotificationSettingCreate/
// NotificationSettingUpdate (the request shapes) are three separate types
// because of this, not two — reusing one struct for both directions would
// mean either the request sends objects Paddle rejects, or the response
// fails to unmarshal into a []string field.

type NotificationSettingEvent struct {
	Name              string `json:"name"`
	Description       string `json:"description,omitempty"`
	Group             string `json:"group,omitempty"`
	AvailableVersions []int  `json:"available_versions,omitempty"`
}

type NotificationSetting struct {
	ID                     string                     `json:"id,omitempty"`
	Description            string                     `json:"description"`
	Type                   string                     `json:"type,omitempty"`
	Destination            string                     `json:"destination"`
	Active                 bool                       `json:"active,omitempty"`
	APIVersion             int                        `json:"api_version,omitempty"`
	IncludeSensitiveFields bool                       `json:"include_sensitive_fields,omitempty"`
	TrafficSource          string                     `json:"traffic_source,omitempty"`
	SubscribedEvents       []NotificationSettingEvent `json:"subscribed_events,omitempty"`
	// EndpointSecretKey signs webhook payloads sent to this destination —
	// genuinely sensitive, must never be logged (do() already never logs
	// bodies at all, but this is also why the resource schema marks the
	// matching attribute Sensitive).
	EndpointSecretKey string `json:"endpoint_secret_key,omitempty"`
}

// NotificationSettingCreate is the POST body. No Active field at all —
// confirmed against the real API reference, it's genuinely not accepted
// at create (defaults true), only settable via a later update.
type NotificationSettingCreate struct {
	Description            string   `json:"description"`
	Type                   string   `json:"type"`
	Destination            string   `json:"destination"`
	SubscribedEvents       []string `json:"subscribed_events"`
	APIVersion             *int     `json:"api_version,omitempty"`
	IncludeSensitiveFields *bool    `json:"include_sensitive_fields,omitempty"`
	TrafficSource          string   `json:"traffic_source,omitempty"`
}

// NotificationSettingUpdate is the PATCH body. No Type field — confirmed
// against the real API reference, it's immutable after create (same class
// of fix as Price's product_id, caught before writing this resource rather
// than after a sandbox crash). Active is present here, the only place it's
// settable at all.
type NotificationSettingUpdate struct {
	Description            string   `json:"description"`
	Destination            string   `json:"destination"`
	Active                 *bool    `json:"active,omitempty"`
	SubscribedEvents       []string `json:"subscribed_events"`
	APIVersion             *int     `json:"api_version,omitempty"`
	IncludeSensitiveFields *bool    `json:"include_sensitive_fields,omitempty"`
	TrafficSource          string   `json:"traffic_source,omitempty"`
}

type notificationSettingEnvelope struct {
	Data NotificationSetting `json:"data"`
}

func (c *Client) CreateNotificationSetting(ctx context.Context, ns NotificationSettingCreate) (*NotificationSetting, error) {
	var env notificationSettingEnvelope
	if err := c.do(ctx, http.MethodPost, "/notification-settings", ns, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

func (c *Client) GetNotificationSetting(ctx context.Context, id string) (*NotificationSetting, error) {
	var env notificationSettingEnvelope
	if err := c.do(ctx, http.MethodGet, "/notification-settings/"+id, nil, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

func (c *Client) UpdateNotificationSetting(ctx context.Context, id string, ns NotificationSettingUpdate) (*NotificationSetting, error) {
	var env notificationSettingEnvelope
	if err := c.do(ctx, http.MethodPatch, "/notification-settings/"+id, ns, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

// DeleteNotificationSetting is a real hard DELETE — unlike every other
// entity this provider manages, there is no archive-via-update fallback
// for this one. Whether a second DELETE on an already-deleted destination
// 404s the same way the archive endpoints do (so IsNotFound tolerance
// applies the same way) is confirmed against the real sandbox via this
// resource's acceptance test CheckDestroy, not assumed to transfer over
// from the archive pattern automatically.
func (c *Client) DeleteNotificationSetting(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/notification-settings/"+id, nil, nil)
}

type notificationSettingListEnvelope struct {
	Data []NotificationSetting `json:"data"`
	Meta paginationMeta        `json:"meta"`
}

// ListNotificationSettings — see ListProducts' comment; same pagination
// shape, same sweeper-only purpose.
func (c *Client) ListNotificationSettings(ctx context.Context) ([]NotificationSetting, error) {
	var all []NotificationSetting
	after := ""
	for {
		var env notificationSettingListEnvelope
		if err := c.do(ctx, http.MethodGet, listPath("/notification-settings", after), nil, &env); err != nil {
			return nil, err
		}
		all = append(all, env.Data...)
		if len(env.Data) == 0 || !env.Meta.Pagination.HasMore {
			return all, nil
		}
		after = env.Data[len(env.Data)-1].ID
	}
}

// ── Checkout Domains — https://developer.paddle.com/api-reference/checkout-domains ─
//
// Unlike every other entity this provider handles, there is no create (or
// update) operation for checkout domains at all — confirmed against the
// real API reference, 2026-08-09: "You can't add a checkout domain using
// the API. To submit a new domain for approval, go to Paddle > Checkout >
// Website approval > Domain approval in your dashboard." Only List, Get,
// Delete, and a verify-payment-method action exist. This provider only
// implements Get (a `paddle_checkout_domain` data source — see
// docs/decisions/0007's Step 6 status for why a full read/write resource
// isn't modeled for an entity that can't be created or updated via API at
// all).

type ApplePayVerification struct {
	Status string `json:"status"`
}

type PaymentMethodVerification struct {
	ApplePay ApplePayVerification `json:"apple_pay"`
}

type CheckoutDomain struct {
	ID                        string                    `json:"id,omitempty"`
	Domain                    string                    `json:"domain"`
	Status                    string                    `json:"status,omitempty"`
	PaymentMethodVerification PaymentMethodVerification `json:"payment_method_verification"`
	CreatedAt                 string                    `json:"created_at,omitempty"`
	UpdatedAt                 string                    `json:"updated_at,omitempty"`
}

type checkoutDomainEnvelope struct {
	Data CheckoutDomain `json:"data"`
}

func (c *Client) GetCheckoutDomain(ctx context.Context, id string) (*CheckoutDomain, error) {
	var env checkoutDomainEnvelope
	if err := c.do(ctx, http.MethodGet, "/checkout-domains/"+id, nil, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

type checkoutDomainListEnvelope struct {
	Data []CheckoutDomain `json:"data"`
	Meta paginationMeta   `json:"meta"`
}

// ListCheckoutDomains — see ListProducts' comment for the pagination
// shape. Used by the acceptance test (there's no way to create a fixture
// via API — see this section's own comment — so the test lists whatever
// already exists in the sandbox rather than provisioning its own) rather
// than by any sweeper (nothing to sweep: this provider never creates a
// checkout domain in the first place).
func (c *Client) ListCheckoutDomains(ctx context.Context) ([]CheckoutDomain, error) {
	var all []CheckoutDomain
	after := ""
	for {
		var env checkoutDomainListEnvelope
		if err := c.do(ctx, http.MethodGet, listPath("/checkout-domains", after), nil, &env); err != nil {
			return nil, err
		}
		all = append(all, env.Data...)
		if len(env.Data) == 0 || !env.Meta.Pagination.HasMore {
			return all, nil
		}
		after = env.Data[len(env.Data)-1].ID
	}
}

// ── Adjustments — https://developer.paddle.com/api-reference/adjustments ───
//
// No archive/delete/update operation exists for adjustments at all —
// confirmed against the real API reference (2026-08-10): only create and
// list/get. Once created, an adjustment's `status` (pending_approval,
// approved, rejected, reversed) moves on Paddle's own side, not via any
// call this client makes.

// AdjustmentItem is one line item of a partial adjustment — see
// Adjustment's own comment for why response-only fields (proration,
// totals) aren't modeled.
type AdjustmentItem struct {
	// ItemID is the transaction line item being adjusted (txnitm_...),
	// not the adjustment's own item id — Paddle's response echoes both,
	// this client only needs the request-side one.
	ItemID string `json:"item_id"`
	// Type: "full", "partial", "tax", or "proration".
	Type string `json:"type"`
	// Amount: required when Type is "partial", omitted otherwise —
	// deliberately a pointer with omitempty rather than a bare string, so
	// a full-type item can omit it entirely instead of sending "".
	Amount *string `json:"amount,omitempty"`
}

// Adjustment is Paddle's adjustment object. Modeled with only the fields
// this provider's action layer actually uses (every request field, plus
// ID/Status for search-before-invoke matching and terminal-status
// checking) — not the full response shape (totals, payout_totals,
// tax_rates_used, subscription_id, customer_id, currency_code,
// created_at/updated_at aren't consumed anywhere in this provider, so
// aren't modeled here; add them if a future caller needs them, the same
// "don't model ahead of use" discipline every other entity in this client
// already follows).
type Adjustment struct {
	ID string `json:"id,omitempty"`
	// Action: "credit", "refund", "chargeback", "chargeback_reverse",
	// "chargeback_warning", "chargeback_warning_reverse", or
	// "credit_reverse" — the full enum Paddle's API reference documents,
	// not just refund/credit (an earlier, unverified assumption this
	// client corrects before shipping, not after).
	Action string `json:"action"`
	// Type: "full" or "partial", defaults to "partial" server-side if
	// omitted.
	Type string `json:"type,omitempty"`
	// TaxMode: "internal" or "external", only meaningful for partial
	// adjustments.
	TaxMode       string `json:"tax_mode,omitempty"`
	TransactionID string `json:"transaction_id"`
	Reason        string `json:"reason"`
	// Items is required by Paddle's API when Type is "partial", optional
	// otherwise — not cross-field-validated client-side, same "rely on
	// Paddle's own API error unless it's a two-line change" default this
	// provider already applies to paddle_discount's discount_group_id
	// (docs/plans/paddle-provider-v2.md Step 4 item 6).
	Items []AdjustmentItem `json:"items,omitempty"`
	// Status is Paddle-assigned, read-only — never sent in a request body
	// regardless of Go zero-value, since it has omitempty. One of
	// "pending_approval", "approved", "rejected", "reversed".
	Status string `json:"status,omitempty"`
}

type adjustmentEnvelope struct {
	Data Adjustment `json:"data"`
}

// CreateAdjustment uses doNoRetry, not do() — see
// docs/guardrails/money-moving-actions-no-blanket-retry.md. A blindly
// retried create-adjustment could double-refund or double-credit real
// money; Paddle has no idempotency-key mechanism to protect against that
// server-side, so this client must not risk it by retrying blindly either.
func (c *Client) CreateAdjustment(ctx context.Context, a Adjustment) (*Adjustment, error) {
	var env adjustmentEnvelope
	if err := c.doNoRetry(ctx, http.MethodPost, "/adjustments", a, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

type adjustmentListEnvelope struct {
	Data []Adjustment   `json:"data"`
	Meta paginationMeta `json:"meta"`
}

// ListAdjustments is used for search-before-invoke — find an existing
// adjustment for the same transaction before creating a new one, best
// effort (see docs/guardrails/money-moving-actions-no-blanket-retry.md) —
// not by a sweeper: this provider only creates adjustments from
// paddle_adjustment's own Invoke(), and Paddle has no
// archive/delete operation for adjustments to sweep anyway. Uses the
// regular retry-wrapped do(), not doNoRetry — a read is safe to retry,
// only the create call above isn't. transaction_id is Paddle's
// documented comma-separated-list filter syntax; a single value needs no
// comma.
func (c *Client) ListAdjustments(ctx context.Context, transactionID string) ([]Adjustment, error) {
	var all []Adjustment
	after := ""
	for {
		path := "/adjustments?per_page=200&transaction_id=" + url.QueryEscape(transactionID)
		if after != "" {
			path += "&after=" + url.QueryEscape(after)
		}
		var env adjustmentListEnvelope
		if err := c.do(ctx, http.MethodGet, path, nil, &env); err != nil {
			return nil, err
		}
		all = append(all, env.Data...)
		if len(env.Data) == 0 || !env.Meta.Pagination.HasMore {
			return all, nil
		}
		after = env.Data[len(env.Data)-1].ID
	}
}

// ── Subscriptions — https://developer.paddle.com/api-reference/subscriptions ───
//
// Actions only, not a managed resource — see
// docs/decisions/0010-v3-scope-lifecycle-actions.md for why. Subscriptions
// have a much larger real field list than modeled here; only the fields
// this provider's cancel/pause/resume/charge actions actually read
// (ID/Status) are included, same "don't model ahead of use" discipline
// Adjustment follows.

type Subscription struct {
	ID     string `json:"id,omitempty"`
	Status string `json:"status,omitempty"`
}

type subscriptionEnvelope struct {
	Data Subscription `json:"data"`
}

// GetSubscription is search-before-invoke's status check for
// cancel/pause/resume (docs/guardrails/money-moving-actions-no-blanket-retry.md)
// — uses the regular retry-wrapped do(), not doNoRetry, since a read is
// safe to retry.
func (c *Client) GetSubscription(ctx context.Context, id string) (*Subscription, error) {
	var env subscriptionEnvelope
	if err := c.do(ctx, http.MethodGet, "/subscriptions/"+id, nil, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

type SubscriptionCancelRequest struct {
	// EffectiveFrom: "immediately" or "next_billing_period". No default
	// applied client-side even though Paddle defaults to
	// next_billing_period server-side if omitted — deliberately required
	// in the action schema that builds this request, so "immediate" is
	// never an implicit choice for something this hard to reverse ("You
	// can't reinstate a canceled subscription" — Paddle's own docs).
	EffectiveFrom string `json:"effective_from"`
}

// CancelSubscription uses doNoRetry — see
// docs/guardrails/money-moving-actions-no-blanket-retry.md. A blindly
// retried cancel could hit an already-canceled subscription with a
// confusing error, or worse, mask a real ambiguous outcome; Paddle has no
// idempotency-key mechanism to protect against that.
func (c *Client) CancelSubscription(ctx context.Context, id string, req SubscriptionCancelRequest) (*Subscription, error) {
	var env subscriptionEnvelope
	if err := c.doNoRetry(ctx, http.MethodPost, "/subscriptions/"+id+"/cancel", req, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

type SubscriptionPauseRequest struct {
	// EffectiveFrom: "immediately" or "next_billing_period" — required
	// here, same reasoning as SubscriptionCancelRequest.
	EffectiveFrom string `json:"effective_from"`
	// ResumeAt: RFC 3339 timestamp: when a paused subscription should
	// resume automatically. Omit (nil) for an indefinite pause — a
	// meaningful, deliberate choice, not a default this client should
	// paper over.
	ResumeAt *string `json:"resume_at,omitempty"`
	// OnResume: "continue_existing_billing_period" or
	// "start_new_billing_period". Left to Paddle's own default
	// (start_new_billing_period) if omitted.
	OnResume string `json:"on_resume,omitempty"`
}

// PauseSubscription uses doNoRetry — see CancelSubscription's comment.
func (c *Client) PauseSubscription(ctx context.Context, id string, req SubscriptionPauseRequest) (*Subscription, error) {
	var env subscriptionEnvelope
	if err := c.doNoRetry(ctx, http.MethodPost, "/subscriptions/"+id+"/pause", req, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

type SubscriptionResumeRequest struct {
	// EffectiveFrom: "immediately" or an RFC 3339 timestamp for a
	// scheduled resume. Required — no client-side default, same
	// reasoning as Cancel/Pause.
	EffectiveFrom string `json:"effective_from"`
	OnResume      string `json:"on_resume,omitempty"`
}

// ResumeSubscription uses doNoRetry — see CancelSubscription's comment.
func (c *Client) ResumeSubscription(ctx context.Context, id string, req SubscriptionResumeRequest) (*Subscription, error) {
	var env subscriptionEnvelope
	if err := c.doNoRetry(ctx, http.MethodPost, "/subscriptions/"+id+"/resume", req, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

// SubscriptionChargeItem is deliberately catalog-price-only (price_id +
// quantity) — Paddle's create-subscription-charge accepts two other item
// variants (a non-catalog price against an existing product, or a
// fully inline non-catalog product+price), each with their own nested
// price/product object shapes. Scoped out for now: the catalog-price
// variant is the dominant real-world case and the one this provider's
// existing paddle_price resource already produces IDs for; the other two
// variants are a materially bigger schema-design task on their own,
// deliberately deferred rather than shipped half-modeled. Extend this
// type (or add a oneof-shaped alternative) if/when that's needed.
type SubscriptionChargeItem struct {
	PriceID  string `json:"price_id"`
	Quantity int64  `json:"quantity"`
}

type SubscriptionChargeRequest struct {
	// EffectiveFrom: "immediately" or "next_billing_period" — required,
	// same reasoning as Cancel/Pause/Resume.
	EffectiveFrom string                   `json:"effective_from"`
	Items         []SubscriptionChargeItem `json:"items"`
	// OnPaymentFailure: "prevent_change" or "apply_change". Left to
	// Paddle's own default (prevent_change) if omitted.
	OnPaymentFailure string `json:"on_payment_failure,omitempty"`
	// ReceiptData is only valid when EffectiveFrom is "immediately" —
	// not cross-field-validated client-side, same "rely on Paddle's own
	// API error" default this provider already applies elsewhere.
	ReceiptData *string `json:"receipt_data,omitempty"`
}

// ChargeSubscription uses doNoRetry — see CancelSubscription's comment. A
// blindly retried charge could double-bill a real customer.
func (c *Client) ChargeSubscription(ctx context.Context, id string, req SubscriptionChargeRequest) (*Subscription, error) {
	var env subscriptionEnvelope
	if err := c.doNoRetry(ctx, http.MethodPost, "/subscriptions/"+id+"/charge", req, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

// ── Transactions — read-only, search-before-invoke support only ───────────
//
// Transactions themselves stay excluded as a managed resource
// (docs/decisions/0001-catalog-only-scope-v1.md, unrevisited by
// docs/decisions/0010-v3-scope-lifecycle-actions.md). This narrow,
// read-only slice exists solely so paddle_subscription_charge can search
// for a prior matching charge before creating a new one — not the start
// of broader Transaction support. Only the fields that search actually
// compares are modeled (see SubscriptionChargeItem's comment on why
// items are catalog-price-only here too).

type TransactionItem struct {
	PriceID  string `json:"price_id,omitempty"`
	Quantity int64  `json:"quantity,omitempty"`
}

type Transaction struct {
	ID             string            `json:"id,omitempty"`
	SubscriptionID string            `json:"subscription_id,omitempty"`
	Status         string            `json:"status,omitempty"`
	Origin         string            `json:"origin,omitempty"`
	Items          []TransactionItem `json:"items,omitempty"`
}

type transactionListEnvelope struct {
	Data []Transaction  `json:"data"`
	Meta paginationMeta `json:"meta"`
}

// ListSubscriptionChargeTransactions lists transactions for subscriptionID
// with origin=subscription_charge — the set paddle_subscription_charge's
// search-before-invoke matches against. subscription_id and origin are
// Paddle's documented comma-separated-list filter syntax; a single value
// needs no comma. Uses the regular retry-wrapped do(), not doNoRetry — a
// read is safe to retry.
func (c *Client) ListSubscriptionChargeTransactions(ctx context.Context, subscriptionID string) ([]Transaction, error) {
	var all []Transaction
	after := ""
	for {
		path := "/transactions?per_page=200&origin=subscription_charge&subscription_id=" + url.QueryEscape(subscriptionID)
		if after != "" {
			path += "&after=" + url.QueryEscape(after)
		}
		var env transactionListEnvelope
		if err := c.do(ctx, http.MethodGet, path, nil, &env); err != nil {
			return nil, err
		}
		all = append(all, env.Data...)
		if len(env.Data) == 0 || !env.Meta.Pagination.HasMore {
			return all, nil
		}
		after = env.Data[len(env.Data)-1].ID
	}
}

// NextTransactionItem/GetSubscriptionNextTransaction: a one-time charge
// created with effective_from "next_billing_period" produces no
// queryable Transaction at all until the subscription actually renews
// (confirmed against the real API reference and the real sandbox,
// 2026-08-10 — a genuine gap this client shipped with initially, found by
// running paddle_subscription_charge's acceptance test for real:
// ListSubscriptionChargeTransactions correctly finds nothing for a
// "next_billing_period" charge, but that's not "no duplicate exists", it's
// "this search method can't see pending ones at all"). Paddle's own docs
// name the fix: GET /subscriptions/{id}?include=next_transaction previews
// what's already queued for the next renewal, including one-time charges
// not yet billed — this is what a "next_billing_period" charge's
// search-before-invoke must check instead of ListSubscriptionChargeTransactions.
type NextTransactionItem struct {
	PriceID  string `json:"price_id"`
	Quantity int64  `json:"quantity"`
}

type NextTransactionPreview struct {
	Items []NextTransactionItem `json:"items"`
}

type subscriptionWithNextTransactionEnvelope struct {
	Data struct {
		Subscription
		NextTransaction *NextTransactionPreview `json:"next_transaction"`
	} `json:"data"`
}

// GetSubscriptionNextTransaction returns nil (not an error) if the
// subscription has no upcoming charge preview at all — a subscription
// with nothing queued for its next renewal beyond its normal recurring
// items is a legitimate, common case, not a failure.
func (c *Client) GetSubscriptionNextTransaction(ctx context.Context, id string) (*NextTransactionPreview, error) {
	var env subscriptionWithNextTransactionEnvelope
	if err := c.do(ctx, http.MethodGet, "/subscriptions/"+id+"?include=next_transaction", nil, &env); err != nil {
		return nil, err
	}
	return env.Data.NextTransaction, nil
}

// ── Test fixture support only — Customers, Addresses, Transactions ────────
//
// NOT managed resources and NOT exposed anywhere in
// internal/provider/*_resource.go or *_data_source.go. Customers/Addresses
// stay deferred per docs/decisions/0010-v3-scope-lifecycle-actions.md
// (the PII-in-state-file concern that decision raises applies to a real
// Terraform resource whose values persist in state across applies — it
// does not apply here: these exist only to script a disposable
// customer+address+transaction into the sandbox as a fixture for
// paddle_adjustment's acceptance test, created and left to be swept like
// any other acc-test object, never touching Terraform state at all. Don't
// build a resource or data source on top of these without revisiting that
// decision properly first — this section existing is not evidence the
// reversal was reconsidered.

type Customer struct {
	ID     string `json:"id,omitempty"`
	Email  string `json:"email"`
	Name   string `json:"name,omitempty"`
	Status string `json:"status,omitempty"`
}

type customerEnvelope struct {
	Data Customer `json:"data"`
}

func (c *Client) CreateCustomer(ctx context.Context, cust Customer) (*Customer, error) {
	var env customerEnvelope
	if err := c.do(ctx, http.MethodPost, "/customers", cust, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

type customerListEnvelope struct {
	Data []Customer     `json:"data"`
	Meta paginationMeta `json:"meta"`
}

// ListTestFixtureCustomers exists solely so the sweeper can find and
// archive fixture customers created by CreateCustomer above — same
// "Acc Test" naming-convention sweep every other entity already uses,
// here matched via Email rather than Name (Customer has no name field
// used at creation in this fixture path).
func (c *Client) ListTestFixtureCustomers(ctx context.Context) ([]Customer, error) {
	var all []Customer
	after := ""
	for {
		var env customerListEnvelope
		if err := c.do(ctx, http.MethodGet, listPath("/customers", after), nil, &env); err != nil {
			return nil, err
		}
		all = append(all, env.Data...)
		if len(env.Data) == 0 || !env.Meta.Pagination.HasMore {
			return all, nil
		}
		after = env.Data[len(env.Data)-1].ID
	}
}

// ArchiveTestFixtureCustomer — same archive-not-delete pattern every other
// entity in this client uses; Customers have no separate delete operation
// either.
func (c *Client) ArchiveTestFixtureCustomer(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPatch, "/customers/"+id, statusPatch{Status: "archived"}, nil)
}

type Address struct {
	ID          string `json:"id,omitempty"`
	CountryCode string `json:"country_code"`
	PostalCode  string `json:"postal_code,omitempty"`
	Status      string `json:"status,omitempty"`
}

type addressEnvelope struct {
	Data Address `json:"data"`
}

func (c *Client) CreateAddress(ctx context.Context, customerID string, a Address) (*Address, error) {
	var env addressEnvelope
	if err := c.do(ctx, http.MethodPost, "/customers/"+customerID+"/addresses", a, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

// TransactionCreateItem/TransactionCreate/PaymentTerms model just enough
// of create-transaction's request shape to script a manually-collected,
// pre-billed fixture transaction — not general Transaction support. See
// this section's own top comment.
type TransactionCreateItem struct {
	PriceID  string `json:"price_id"`
	Quantity int64  `json:"quantity"`
}

type PaymentTerms struct {
	Interval  string `json:"interval"`
	Frequency int64  `json:"frequency"`
}

type BillingDetails struct {
	PaymentTerms PaymentTerms `json:"payment_terms"`
}

type TransactionCreate struct {
	Items          []TransactionCreateItem `json:"items"`
	CustomerID     string                  `json:"customer_id,omitempty"`
	AddressID      string                  `json:"address_id,omitempty"`
	CollectionMode string                  `json:"collection_mode,omitempty"`
	// Status: set to "billed" directly at creation for a manually-collected
	// fixture transaction — see create-adjustment's requirement that the
	// target transaction be completed (auto) or billed/past_due (manual).
	Status         string          `json:"status,omitempty"`
	BillingDetails *BillingDetails `json:"billing_details,omitempty"`
}

type transactionCreateResponseEnvelope struct {
	Data Transaction `json:"data"`
}

func (c *Client) CreateTransaction(ctx context.Context, t TransactionCreate) (*Transaction, error) {
	var env transactionCreateResponseEnvelope
	if err := c.do(ctx, http.MethodPost, "/transactions", t, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

// ListSubscriptions is used only by acceptance tests to find (or confirm
// the absence of) an already-existing, manually/checkout-provisioned
// subscription in the sandbox — subscriptions can't be created via pure
// API calls at all (confirmed 2026-08-10: only real checkout + a test
// card produces one), so this provider's subscription action tests can't
// self-provision a fixture the way every other test in this repo does.
// Same skip-cleanly-if-none-found pattern docs/plans/paddle-provider-v2.md
// Step 6 already established for paddle_checkout_domain.
func (c *Client) ListSubscriptions(ctx context.Context) ([]Subscription, error) {
	var all []Subscription
	after := ""
	for {
		var env subscriptionListEnvelope
		if err := c.do(ctx, http.MethodGet, listPath("/subscriptions", after), nil, &env); err != nil {
			return nil, err
		}
		all = append(all, env.Data...)
		if len(env.Data) == 0 || !env.Meta.Pagination.HasMore {
			return all, nil
		}
		after = env.Data[len(env.Data)-1].ID
	}
}

type subscriptionListEnvelope struct {
	Data []Subscription `json:"data"`
	Meta paginationMeta `json:"meta"`
}
