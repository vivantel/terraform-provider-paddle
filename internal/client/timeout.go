package client

import (
	"context"
	"errors"
)

// IsTimeout reports whether err is (or wraps) a context deadline
// exceeded — meaning do()/doNoRetry's retryOverallBudget was exhausted
// without ever getting an inspectable HTTP response, as opposed to a
// real, fast *APIError. net/http wraps a mid-flight context deadline in
// a *url.Error, which correctly unwraps to context.DeadlineExceeded via
// errors.Is; doOnce's own "do request: %w" wrapping (and any caller's
// further wrapping) unwraps the same way.
//
// Callers use this to avoid a second, equally doomed call after a first
// one times out — see cancelOrCreditTransaction's use in
// internal/provider/sweep_test.go, found necessary after a real sweep
// run showed a timed-out CancelTransaction still followed by an
// unconditional (and also timed-out) GetTransaction fallback, doubling
// real-world time spent on a transaction that was never going to
// succeed either way within that run.
func IsTimeout(err error) bool {
	return errors.Is(err, context.DeadlineExceeded)
}
