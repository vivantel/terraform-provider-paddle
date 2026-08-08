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

func TestDo_OverallBudgetBoundsSlowPersistentFailures(t *testing.T) {
	// Regression test for /code-review high finding: the 30s per-attempt
	// HTTP client timeout isn't wrapped by any overall budget across the
	// 5-attempt retry loop, so a persistently *slow* (not fast-failing)
	// backend can block a single call for minutes — up to 5 * 30s of
	// request time plus jittered backoff, sequentially. retryOverallBudget
	// wraps the whole do() call in a context.WithTimeout, which — since
	// context.WithTimeout always takes the *earlier* of two deadlines —
	// only ever tightens an already-bounded caller context, never loosens
	// an unbounded one beyond this budget.
	origBudget := retryOverallBudget
	retryOverallBudget = 30 * time.Millisecond
	t.Cleanup(func() { retryOverallBudget = origBudget })

	withFastRetries(t) // so any actual retry attempts stay quick too

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Each individual response takes longer than the whole budget —
		// simulates a hanging/slow backend, not a fast-failing one.
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(productEnvelope{Data: Product{ID: "pro_1"}})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")

	start := time.Now()
	_, err := c.GetProduct(context.Background(), "pro_1")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("GetProduct: got nil error against a backend slower than retryOverallBudget, want a timeout error")
	}
	if elapsed > 150*time.Millisecond {
		t.Errorf("GetProduct took %v, want bounded near retryOverallBudget (30ms) — a slow backend must not be allowed to block for the full per-attempt timeout on every retry", elapsed)
	}
}

func TestWaitBeforeRetry_RespectsContextCancellation(t *testing.T) {
	// Tests waitBeforeRetry directly rather than through a full do() retry
	// loop over real HTTP round trips. An earlier version of this test
	// (TestDo_RespectsContextCancellationDuringBackoff) drove this through
	// GetProduct against an httptest server with a 20ms context timeout
	// against a 200ms *jittered* backoff — backoffDelay's full jitter
	// picks a random delay in [0, 200ms), so on a slower CI runner it
	// could legitimately land under 20ms and let a second attempt through
	// before the deadline fired, flaking the "exactly 1 call" assertion
	// (seen in CI run 31273500236: calls=2, not 1). Fixed by testing the
	// wait itself with a backoff far longer than the deadline, and an
	// already-cancelled context, so the outcome doesn't depend on timing
	// at all.
	origBase, origMax := retryBaseBackoff, retryMaxBackoff
	retryBaseBackoff = time.Second
	retryMaxBackoff = time.Second
	t.Cleanup(func() { retryBaseBackoff, retryMaxBackoff = origBase, origMax })

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before waitBeforeRetry is even called

	start := time.Now()
	err := waitBeforeRetry(ctx, 1, "")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("waitBeforeRetry: got nil error, want the context's cancellation error")
	}
	// The backoff itself would sleep up to 1s; an already-cancelled
	// context must short-circuit that near-instantly, not sleep through it.
	if elapsed > 100*time.Millisecond {
		t.Errorf("waitBeforeRetry took %v with an already-cancelled context, want near-instant return", elapsed)
	}
}

func TestWaitBeforeRetry_ZeroDelayStillRespectsCancellation(t *testing.T) {
	// Regression test for /code-review high finding: when the computed
	// delay is <= 0, waitBeforeRetry returned nil immediately without ever
	// checking ctx.Done() — the select below that check exists specifically
	// to perform is skipped entirely. A Retry-After: 0 header is a clean,
	// deterministic way to force d == 0 without going through
	// backoffDelay's jitter (rand.Int63n(0) would panic, and jitter is
	// nondeterministic besides) — parseRetryAfter("0") returns (0, true).
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	err := waitBeforeRetry(ctx, 1, "0")

	if err == nil {
		t.Fatal("waitBeforeRetry: got nil error with an already-cancelled context and a zero delay, want the context's cancellation error")
	}
}
