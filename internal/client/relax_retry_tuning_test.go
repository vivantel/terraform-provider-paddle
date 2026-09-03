package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestRelaxRetryTuningForAcceptanceTests_RidesOutTheFullOverallBudget is a
// regression test for a real bug found empirically, 2026-08-13, in this
// same function's first version: it widened retryOverallBudget and
// retryMaxRetryAfter, but left retryMaxAttempts/retryMaxBackoff at
// values sized for a 60s budget. Against a 429 response carrying no
// Retry-After header (real against the sandbox, not just hypothetical),
// do() falls back to backoffDelay's exponential computation, capped by
// retryMaxBackoff — so a handful of attempts at the old 10s cap
// exhausted retryMaxAttempts (and returned the error) in under 90s
// regardless of how large the overall budget was. CI kept failing at
// ~60s despite the widened budget until this was found and fixed
// (retryMaxAttempts and retryMaxBackoff both widened too).
//
// Restores every mutated package var afterward — these are shared,
// package-level tuning knobs, and this test must not leak a relaxed
// state into any test that runs after it in the same binary.
func TestRelaxRetryTuningForAcceptanceTests_RidesOutTheFullOverallBudget(t *testing.T) {
	origMaxAttempts, origBaseBackoff, origMaxBackoff := retryMaxAttempts, retryBaseBackoff, retryMaxBackoff
	origMaxRetryAfter, origOverallBudget := retryMaxRetryAfter, retryOverallBudget
	t.Cleanup(func() {
		retryMaxAttempts, retryBaseBackoff, retryMaxBackoff = origMaxAttempts, origBaseBackoff, origMaxBackoff
		retryMaxRetryAfter, retryOverallBudget = origMaxRetryAfter, origOverallBudget
	})

	RelaxRetryTuningForAcceptanceTests()
	// Shrink just the overall budget after relaxing — proves the loop
	// now actually rides out whatever budget is configured (limited by
	// attempt count/backoff cap being wide enough), without this test
	// itself needing to wait out the real 10-minute production-relaxed
	// value to prove it.
	retryOverallBudget = 3 * time.Second

	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		// Deliberately no Retry-After header — the exact condition that
		// exposed the bug; do() must fall back to backoffDelay here.
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	start := time.Now()
	err := c.do(context.Background(), http.MethodPost, "/customers", nil, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("do() = nil error, want a timeout/429 error from a server that always returns 429")
	}
	// The loop must run close to the full 3s budget, not stop early
	// (the old bug: attempt==retryMaxAttempts cut it off around 60-90s
	// even against a 10-minute budget — proportionally, well under this
	// 3s budget too).
	if elapsed < 2*time.Second {
		t.Errorf("elapsed = %v, want close to the %v overall budget — retryMaxAttempts/retryMaxBackoff must still be too small, cutting the loop off before the time budget is used", elapsed, retryOverallBudget)
	}
	if calls < 2 {
		t.Errorf("calls = %d, want at least 2 — a single attempt wouldn't distinguish this from doNoRetry", calls)
	}
}

// TestRelaxRetryTuningForSweepRun_RidesOutItsOwnOverallBudget is
// RelaxRetryTuningForSweepRun's counterpart to the acceptance-tuning test
// above — same "does the loop actually use the full budget, not cut off
// early on attempt count/backoff cap" regression shape, proven the same
// way (shrink the budget for the test rather than waiting out the real
// value).
func TestRelaxRetryTuningForSweepRun_RidesOutItsOwnOverallBudget(t *testing.T) {
	origMaxAttempts, origBaseBackoff, origMaxBackoff := retryMaxAttempts, retryBaseBackoff, retryMaxBackoff
	origMaxRetryAfter, origOverallBudget := retryMaxRetryAfter, retryOverallBudget
	t.Cleanup(func() {
		retryMaxAttempts, retryBaseBackoff, retryMaxBackoff = origMaxAttempts, origBaseBackoff, origMaxBackoff
		retryMaxRetryAfter, retryOverallBudget = origMaxRetryAfter, origOverallBudget
	})

	RelaxRetryTuningForSweepRun()
	retryOverallBudget = 3 * time.Second

	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	start := time.Now()
	err := c.do(context.Background(), http.MethodPost, "/customers", nil, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("do() = nil error, want a timeout/429 error from a server that always returns 429")
	}
	if elapsed < 2*time.Second {
		t.Errorf("elapsed = %v, want close to the %v overall budget", elapsed, retryOverallBudget)
	}
	if calls < 2 {
		t.Errorf("calls = %d, want at least 2 — a single attempt wouldn't distinguish this from doNoRetry", calls)
	}
}

// TestRelaxRetryTuningForSweepRun_IsMuchShorterThanAcceptanceTests is the
// actual regression test for the 2026-09-03 sweep timeout bug: a sweep run
// can issue far more sequential calls than an acceptance test run (a real
// backlog across several sweepers, each already tolerant of an individual
// call failing — see RelaxRetryTuningForSweepRun's own comment), so
// giving every one of those calls the same 10-minute-scale budget
// acceptance tests get is precisely what let a handful of stubborn calls
// consume a sweep run's entire time budget before its own
// skip-and-continue logic ever got to run on the rest. If a future change
// makes these two functions converge again (e.g. by copying one's values
// into the other), this test catches it.
func TestRelaxRetryTuningForSweepRun_IsMuchShorterThanAcceptanceTests(t *testing.T) {
	origMaxAttempts, origBaseBackoff, origMaxBackoff := retryMaxAttempts, retryBaseBackoff, retryMaxBackoff
	origMaxRetryAfter, origOverallBudget := retryMaxRetryAfter, retryOverallBudget
	t.Cleanup(func() {
		retryMaxAttempts, retryBaseBackoff, retryMaxBackoff = origMaxAttempts, origBaseBackoff, origMaxBackoff
		retryMaxRetryAfter, retryOverallBudget = origMaxRetryAfter, origOverallBudget
	})

	RelaxRetryTuningForAcceptanceTests()
	acceptanceBudget, acceptanceRetryAfter := retryOverallBudget, retryMaxRetryAfter

	RelaxRetryTuningForSweepRun()
	sweepBudget, sweepRetryAfter := retryOverallBudget, retryMaxRetryAfter

	// "Much shorter" per RelaxRetryTuningForSweepRun's own comment — a
	// fraction of the acceptance-test tuning, not just nominally smaller.
	// A generous 1/3 threshold catches an accidental near-copy without
	// being brittle against reasonable future retuning of either value.
	if sweepBudget > acceptanceBudget/3 {
		t.Errorf("sweep retryOverallBudget = %v, acceptance = %v — sweep tuning must stay a small fraction of acceptance tuning, not converge with it (that's the exact bug this fix addresses)", sweepBudget, acceptanceBudget)
	}
	if sweepRetryAfter > acceptanceRetryAfter/3 {
		t.Errorf("sweep retryMaxRetryAfter = %v, acceptance = %v — same reasoning as retryOverallBudget above", sweepRetryAfter, acceptanceRetryAfter)
	}
}
