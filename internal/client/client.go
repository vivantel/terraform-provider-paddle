// Package client is a minimal HTTP client for the Paddle Billing API
// (https://developer.paddle.com/api-reference/overview). It only implements
// the Products, Prices, Discounts, Discount Groups, Notification Settings,
// and Checkout Domains endpoints this provider currently needs.
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

	var bodyBytes []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyBytes = b
	}

	// No upper bound on the loop header itself — every branch inside
	// already returns explicitly (including the attempt == retryMaxAttempts
	// case below), so an infinite `for` here is exactly as bounded in
	// practice as `for attempt := 1; attempt <= retryMaxAttempts; attempt++`
	// was, but without needing a vestigial `return` after the loop just to
	// satisfy the compiler's control-flow analysis of a bound it can't
	// prove is never exceeded.
	for attempt := 1; ; attempt++ {
		var reqBody io.Reader
		if bodyBytes != nil {
			// A fresh Reader per attempt: bytes.Reader is consumed after
			// one send, and body must survive being sent again on retry.
			reqBody = bytes.NewReader(bodyBytes)
		}

		tflog.Debug(ctx, "paddle: sending request", map[string]any{
			"method":  method,
			"path":    path,
			"attempt": attempt,
		})

		req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reqBody)
		if err != nil {
			return fmt.Errorf("build request: %w", err)
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
			return fmt.Errorf("do request: %w", err)
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("read response body: %w", err)
		}

		tflog.Debug(ctx, "paddle: received response", map[string]any{
			"method": method, "path": path, "attempt": attempt, "status": resp.StatusCode,
		})

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			apiErr := &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
			if !isRetryableStatus(resp.StatusCode) || attempt == retryMaxAttempts {
				return apiErr
			}
			if err := waitBeforeRetry(ctx, attempt, resp.Header.Get("Retry-After")); err != nil {
				return err
			}
			continue
		}

		if out != nil {
			if err := json.Unmarshal(respBody, out); err != nil {
				return fmt.Errorf("unmarshal response body: %w", err)
			}
		}
		return nil
	}
}

func isRetryableStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode >= 500
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
