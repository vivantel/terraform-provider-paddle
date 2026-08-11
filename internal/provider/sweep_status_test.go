package provider

import "testing"

// shouldAttemptCancel's own unit tests — pure logic, no real API needed.
// Found via a real sweep run against today's backlog, 2026-08-11:
// cancelOrCreditTransaction always tried CancelTransaction first, even
// for a transaction its own comment already knows is "completed"
// (subscription-charge transactions are auto-collected, so always
// "completed" by the time this sweeper runs — a Cancel attempt against
// one is guaranteed to be rejected, not a transient failure). Each
// guaranteed-to-fail attempt still went through the client's full
// retry-with-backoff path before falling through, doubling this
// sweeper's real-world cleanup time under Paddle's rate limiting
// (observed: exactly ~120s per leaked transaction — a full ~60s retry
// budget burned on Cancel, then another ~60s on the GetTransaction
// fallback).

func TestShouldAttemptCancel(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"draft", true},
		{"ready", true},
		{"billed", true},
		{"past_due", true},
		{"completed", false},
		{"canceled", true}, // already canceled — Cancel is a safe no-op-ish attempt (IsNotFound/already-canceled tolerance lives in cancelOrCreditTransaction itself), not this function's job to special-case.
		{"", true},         // unknown/empty status — fail open to the old behavior (attempt Cancel) rather than guess.
	}
	for _, c := range cases {
		t.Run(c.status, func(t *testing.T) {
			if got := shouldAttemptCancel(c.status); got != c.want {
				t.Errorf("shouldAttemptCancel(%q) = %v, want %v", c.status, got, c.want)
			}
		})
	}
}
