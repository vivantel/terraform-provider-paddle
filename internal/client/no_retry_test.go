package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// doNoRetry has no retry policy of its own to shrink for speed (unlike
// do()'s withFastRetries helper) — every test here expects exactly one
// HTTP attempt, so there's no backoff to wait out.

func TestDoNoRetry_MakesExactlyOneAttemptOn5xx(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"type":"internal","code":"boom","detail":"transient failure"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	err := c.doNoRetry(context.Background(), http.MethodPost, "/adjustments", nil, nil)

	if err == nil {
		t.Fatal("doNoRetry: got nil error, want a *NonRetryableError")
	}
	var nonRetryable *NonRetryableError
	if !errors.As(err, &nonRetryable) {
		t.Fatalf("error type = %T, want *NonRetryableError (a 5xx is ambiguous — Paddle may have processed the request before failing to respond cleanly)", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want exactly 1 — doNoRetry must never retry, unlike do()", got)
	}
}

func TestDoNoRetry_MakesExactlyOneAttemptOn429(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	err := c.doNoRetry(context.Background(), http.MethodPost, "/adjustments", nil, nil)

	var nonRetryable *NonRetryableError
	if !errors.As(err, &nonRetryable) {
		t.Fatalf("error type = %T, want *NonRetryableError", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want exactly 1 — do()'s Retry-After handling must not apply here", got)
	}
}

func TestDoNoRetry_TransportFailureIsNonRetryable(t *testing.T) {
	// A closed connection (server never responds) is the most ambiguous
	// case of all: the request may or may not have reached a real backend
	// before the failure. Simulated here by pointing at a URL nothing is
	// listening on.
	c := New("http://127.0.0.1:1", "test-key")
	err := c.doNoRetry(context.Background(), http.MethodPost, "/adjustments", nil, nil)

	var nonRetryable *NonRetryableError
	if !errors.As(err, &nonRetryable) {
		t.Fatalf("error type = %T, want *NonRetryableError for a transport-level failure", err)
	}
}

func TestDoNoRetry_CleanClientErrorReturnsPlainAPIError(t *testing.T) {
	// A definitive 4xx rejection (not 429) is not ambiguous — Paddle
	// processed the request and rejected it outright, nothing happened,
	// safe to fix and reapply. Must NOT be wrapped in *NonRetryableError,
	// which would wrongly suggest the outcome is unknown.
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"type":"request_error","code":"invalid_field","detail":"reason is required"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	err := c.doNoRetry(context.Background(), http.MethodPost, "/adjustments", nil, nil)

	if err == nil {
		t.Fatal("doNoRetry: got nil error, want *APIError")
	}
	var nonRetryable *NonRetryableError
	if errors.As(err, &nonRetryable) {
		t.Fatalf("error type = %T, want a plain *APIError — a clean 4xx is not ambiguous and must not be wrapped as retryable-unknown", err)
	}
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("error = %v, want *APIError{StatusCode: 400}", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want exactly 1", got)
	}
}

func TestDoNoRetry_SucceedsAndDecodesOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(productEnvelope{Data: Product{ID: "pro_1", Name: "Widget", TaxCategory: "standard"}})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	var env productEnvelope
	if err := c.doNoRetry(context.Background(), http.MethodGet, "/products/pro_1", nil, &env); err != nil {
		t.Fatalf("doNoRetry: %v", err)
	}
	if env.Data.ID != "pro_1" {
		t.Errorf("Data.ID = %q, want pro_1", env.Data.ID)
	}
}

func TestNonRetryableError_UnwrapsToUnderlyingError(t *testing.T) {
	inner := &APIError{StatusCode: http.StatusServiceUnavailable, Body: "boom"}
	err := &NonRetryableError{Err: inner}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatal("errors.As(*NonRetryableError, *APIError) = false, want true — callers checking for the underlying APIError shape must still work through the wrapper")
	}
	if apiErr.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusServiceUnavailable)
	}
}
