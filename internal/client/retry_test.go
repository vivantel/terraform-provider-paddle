package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// withFastRetries shrinks the package-level retry tuning vars for the
// duration of a test, restoring the production defaults on cleanup — real
// backoff timing (up to ~10s per attempt) would make this suite painfully
// slow and nondeterministic otherwise. Retry-After tests below still use
// real (short) durations, since honoring that header's actual value is
// exactly what they're checking.
func withFastRetries(t *testing.T) {
	t.Helper()
	origAttempts, origBase, origMax, origRetryAfter := retryMaxAttempts, retryBaseBackoff, retryMaxBackoff, retryMaxRetryAfter
	retryMaxAttempts = 3
	retryBaseBackoff = time.Millisecond
	retryMaxBackoff = 5 * time.Millisecond
	retryMaxRetryAfter = 2 * time.Second
	t.Cleanup(func() {
		retryMaxAttempts, retryBaseBackoff, retryMaxBackoff, retryMaxRetryAfter = origAttempts, origBase, origMax, origRetryAfter
	})
}

func TestDo_RetriesOn429ThenSucceeds(t *testing.T) {
	withFastRetries(t)

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(productEnvelope{Data: Product{ID: "pro_1", Name: "Widget", TaxCategory: "standard"}})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	p, err := c.GetProduct(context.Background(), "pro_1")
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if p.ID != "pro_1" {
		t.Errorf("ID = %q, want pro_1", p.ID)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("calls = %d, want 2 (one 429, one success)", got)
	}
}

func TestDo_RetriesOn5xxThenSucceeds(t *testing.T) {
	withFastRetries(t)

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(productEnvelope{Data: Product{ID: "pro_1", Name: "Widget", TaxCategory: "standard"}})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	if _, err := c.GetProduct(context.Background(), "pro_1"); err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("calls = %d, want 2 (one 503, one success)", got)
	}
}

func TestDo_RetryAfterHeaderRespected(t *testing.T) {
	withFastRetries(t)

	const retryAfterSeconds = 1
	var calls int32
	var firstCallAt, secondCallAt time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			firstCallAt = time.Now()
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		secondCallAt = time.Now()
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(productEnvelope{Data: Product{ID: "pro_1", Name: "Widget", TaxCategory: "standard"}})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	if _, err := c.GetProduct(context.Background(), "pro_1"); err != nil {
		t.Fatalf("GetProduct: %v", err)
	}

	elapsed := secondCallAt.Sub(firstCallAt)
	if elapsed < retryAfterSeconds*time.Second {
		t.Errorf("retried after %v, want at least %ds (Retry-After header) — computed backoff was used instead", elapsed, retryAfterSeconds)
	}
}

func TestDo_GivesUpAfterMaxAttemptsReturnsAPIError(t *testing.T) {
	withFastRetries(t) // retryMaxAttempts = 3 for this test

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"type":"internal","code":"boom","detail":"persistent failure"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	_, err := c.GetProduct(context.Background(), "pro_1")
	if err == nil {
		t.Fatal("GetProduct: got nil error, want *APIError after exhausting retries")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusInternalServerError)
	}
	if got := atomic.LoadInt32(&calls); got != int32(retryMaxAttempts) {
		t.Errorf("calls = %d, want %d (retryMaxAttempts)", got, retryMaxAttempts)
	}
}

func TestDo_DoesNotRetryOtherClientErrors(t *testing.T) {
	withFastRetries(t)

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"type":"request_error","code":"not_found","detail":"no such product"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	_, err := c.GetProduct(context.Background(), "pro_missing")
	if err == nil {
		t.Fatal("GetProduct: got nil error, want *APIError")
	}
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("error = %v, want *APIError{StatusCode: 404}", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 — a plain 404 must not be retried", got)
	}
}

func TestDo_RespectsContextCancellationDuringBackoff(t *testing.T) {
	// Deliberately not using withFastRetries: needs a real (if short)
	// backoff window to cancel partway through.
	origBase, origMax := retryBaseBackoff, retryMaxBackoff
	retryBaseBackoff = 200 * time.Millisecond
	retryMaxBackoff = 200 * time.Millisecond
	t.Cleanup(func() { retryBaseBackoff, retryMaxBackoff = origBase, origMax })

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	c := New(srv.URL, "test-key")
	_, err := c.GetProduct(ctx, "pro_1")
	if err == nil {
		t.Fatal("GetProduct: got nil error, want context deadline error")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 — context should have been cancelled during the first backoff wait, before a second attempt", got)
	}
}
