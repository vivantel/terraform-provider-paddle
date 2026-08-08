// Package client is a minimal HTTP client for the Paddle Billing API
// (https://developer.paddle.com/api-reference/overview). It only implements
// the Products and Prices endpoints this provider currently needs.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
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

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("unmarshal response body: %w", err)
		}
	}
	return nil
}

// ── Products — https://developer.paddle.com/api-reference/products ─────────

type Product struct {
	ID          string  `json:"id,omitempty"`
	Name        string  `json:"name"`
	TaxCategory string  `json:"tax_category"`
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

func (c *Client) UpdatePrice(ctx context.Context, id string, p Price) (*Price, error) {
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
