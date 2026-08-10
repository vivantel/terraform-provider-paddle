package actions

import (
	"context"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// checkAlreadyInTargetState fetches subscriptionID and reports whether its
// current status already equals targetStatus, so cancel/pause/resume's
// Invoke() can skip calling Paddle again — search-before-invoke's
// state-transition check
// (docs/guardrails/money-moving-actions-no-blanket-retry.md). Deliberately
// narrow: an exact-match comparison only, never "anything but the source
// state" — that guardrail explicitly calls out resume's check as the
// concrete failure mode ("not paused" would wrongly treat an already
// -canceled subscription as already-resumed). Any status that's neither
// the source nor target state returns alreadyDone=false and falls through
// to the caller's normal mutating call, so Paddle's own response decides.
func checkAlreadyInTargetState(ctx context.Context, c *client.Client, subscriptionID, targetStatus string) (alreadyDone bool, currentStatus string, err error) {
	sub, err := c.GetSubscription(ctx, subscriptionID)
	if err != nil {
		return false, "", err
	}
	return sub.Status == targetStatus, sub.Status, nil
}
