package client

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"
)

// IsTimeout distinguishes "the overall retryOverallBudget was exhausted
// without ever getting an inspectable HTTP response" (do()/doOnce wrap
// this via net/http's own *url.Error, which unwraps to
// context.DeadlineExceeded) from a real, fast HTTP error response
// (*APIError) — found necessary the hard way, 2026-08-11/12: a real sweep
// run showed CancelTransaction consistently timing out (not failing fast)
// against a specific leaked transaction, and cancelOrCreditTransaction
// still unconditionally tried the GetTransaction fallback afterward,
// which timed out too — doubling real-world time spent on a transaction
// that was never going to succeed either way within one sweep run.

func TestIsTimeout(t *testing.T) {
	// *url.Error is what net/http actually returns when a request's
	// context deadline is exceeded mid-flight — matches doOnce's real
	// wrapping ("do request: %w") of http.Client.Do's own error, not a
	// synthetic approximation.
	timeoutErr := &url.Error{Op: "Get", URL: "https://sandbox-api.paddle.com/x", Err: context.DeadlineExceeded}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"direct context.DeadlineExceeded", context.DeadlineExceeded, true},
		{"wrapped url.Error", fmt.Errorf("do request: %w", timeoutErr), true},
		{"doubly wrapped", fmt.Errorf("fetching full transaction detail for item IDs: %w", fmt.Errorf("do request: %w", timeoutErr)), true},
		{"real APIError, not a timeout", &APIError{StatusCode: 429}, false},
		{"wrapped APIError, not a timeout", fmt.Errorf("do: %w", &APIError{StatusCode: 500}), false},
		{"unrelated error", errors.New("boom"), false},
		{"nil", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsTimeout(tc.err); got != tc.want {
				t.Errorf("IsTimeout(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
