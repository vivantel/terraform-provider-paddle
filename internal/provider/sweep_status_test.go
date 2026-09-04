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
//
// **Correction, 2026-09-04**: the 2026-08-12 correction below this
// comment (widening back to "every status but completed") was itself
// wrong about "past_due" specifically — found via a real sweep run
// (fix/sweep-retry-tuning's own verification run) where 21 of 21 leaked
// past_due transactions each burned this repo's new, much shorter
// sweep-tuned retry budget on a doomed Cancel attempt before falling
// through, which is exactly what starved every other registered sweeper
// out of the same 30-minute job. Confirmed directly against Paddle's own
// spec (third_party/paddle-openapi/v1/openapi.yaml, update-transaction's
// description), not inferred from the timeout symptom alone: "You can
// update transactions that are draft or ready. billed and completed
// transactions are considered records for tax and legal purposes, so
// they can't be changed... Cancel a billed transaction by sending a
// PATCH request to set status to canceled." past_due is none of
// draft/ready/billed — it's Paddle's own auto-set, readOnly terminal-ish
// status for a failed/overdue payment, not a status the update-transaction
// PATCH accepts a transition away from at all. A Cancel attempt against
// one is exactly as guaranteed-to-fail as one against "completed" was —
// the credit/refund fallback is the only real path (adjustments are a
// separate operation, valid against past_due — see
// action_paddle_adjustment.go's transaction_id doc comment).
func TestShouldAttemptCancel(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"draft", true},
		{"ready", true},
		{"billed", true},
		{"past_due", false},
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
