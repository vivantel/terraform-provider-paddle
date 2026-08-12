package client

import (
	"context"
	"testing"
	"time"
)

// TestWithDefaultTimeout covers the precedence change
// docs/decisions/0013-configurable-timeouts-architecture.md calls out as
// the one piece of v0.6.0 that changes existing client behavior: a
// caller-supplied deadline (derived from a resource's timeouts{} block)
// must be honored as-is, never tightened to retryOverallBudget — but a
// caller with no deadline of its own still gets exactly the previous
// unconditional-60s behavior.
func TestWithDefaultTimeout(t *testing.T) {
	t.Run("no deadline gets retryOverallBudget applied", func(t *testing.T) {
		ctx, cancel := withDefaultTimeout(context.Background())
		defer cancel()

		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("expected a deadline to be applied, got none")
		}
		remaining := time.Until(deadline)
		// Allow generous slack for test execution overhead — this only
		// needs to confirm retryOverallBudget was applied, not measure it
		// precisely.
		if remaining <= 0 || remaining > retryOverallBudget {
			t.Errorf("deadline %v from now, want (0, %v]", remaining, retryOverallBudget)
		}
	})

	t.Run("pre-set deadline shorter than retryOverallBudget passes through unchanged", func(t *testing.T) {
		want := time.Now().Add(5 * time.Second)
		parent, cancelParent := context.WithDeadline(context.Background(), want)
		defer cancelParent()

		ctx, cancel := withDefaultTimeout(parent)
		defer cancel()

		got, ok := ctx.Deadline()
		if !ok {
			t.Fatal("expected the caller's deadline to survive, got none")
		}
		if !got.Equal(want) {
			t.Errorf("deadline = %v, want %v (caller's own deadline, untouched)", got, want)
		}
	})

	t.Run("pre-set deadline longer than retryOverallBudget still passes through unchanged", func(t *testing.T) {
		// The whole point of the precedence fix: a longer
		// timeouts{}-derived deadline must not be intersected down to
		// retryOverallBudget the way context.WithTimeout would have done
		// before v0.6.0.
		want := time.Now().Add(retryOverallBudget + time.Hour)
		parent, cancelParent := context.WithDeadline(context.Background(), want)
		defer cancelParent()

		ctx, cancel := withDefaultTimeout(parent)
		defer cancel()

		got, ok := ctx.Deadline()
		if !ok {
			t.Fatal("expected the caller's deadline to survive, got none")
		}
		if !got.Equal(want) {
			t.Errorf("deadline = %v, want %v (caller's longer deadline must not be tightened)", got, want)
		}
	})
}
