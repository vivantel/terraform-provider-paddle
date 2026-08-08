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
)

const (
	SandboxBaseURL    = "https://sandbox-api.paddle.com"
	ProductionBaseURL = "https://api.paddle.com"
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
		HTTPClient: http.DefaultClient,
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
	ID          string         `json:"id,omitempty"`
	Name        string         `json:"name"`
	TaxCategory string         `json:"tax_category"`
	Description *string        `json:"description,omitempty"`
	Type        string         `json:"type,omitempty"`
	ImageURL    *string        `json:"image_url,omitempty"`
	CustomData  map[string]any `json:"custom_data,omitempty"`
	Status      string         `json:"status,omitempty"`
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
	_, err := c.UpdateProduct(ctx, id, Product{Status: "archived"})
	return err
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
	ID           string         `json:"id,omitempty"`
	ProductID    string         `json:"product_id"`
	Description  string         `json:"description"`
	UnitPrice    Money          `json:"unit_price"`
	Type         string         `json:"type,omitempty"`
	Name         *string        `json:"name,omitempty"`
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
	_, err := c.UpdatePrice(ctx, id, Price{Status: "archived"})
	return err
}
