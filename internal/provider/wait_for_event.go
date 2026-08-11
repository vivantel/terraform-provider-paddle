package provider

import (
	"context"
	"strings"
	"time"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// eventsLister abstracts client.Client.ListEvents so
// waitForEventContaining's retry loop is testable without a real Paddle
// API call.
type eventsLister func(ctx context.Context, eventTypes []string) ([]client.Event, error)

// waitForEventContaining polls lister for an event of eventType whose
// Data payload contains substr, retrying up to attempts times with delay
// between each. Same read-after-write-lag mitigation this repo already
// applies elsewhere (chargeSearchRetryAttempts/chargeSearchRetryDelay in
// internal/provider/actions/action_paddle_subscription_charge.go) —
// found necessary for the same underlying reason, 2026-08-11, running
// TestAccPaddleEventsDataSource_productCreated against the real sandbox:
// a product just created via CreateProduct wasn't yet visible in
// GET /events on the very next query — Paddle gives no read-after-write
// consistency guarantee here either, so a single immediate query-then-
// give-up isn't enough. Returns (false, nil) if every attempt comes up
// empty, not an error — "not found yet" and "will never be found" are
// indistinguishable from here, same as the charge-search precedent.
func waitForEventContaining(ctx context.Context, lister eventsLister, eventType, substr string, attempts int, delay time.Duration) (bool, error) {
	for attempt := 1; attempt <= attempts; attempt++ {
		events, err := lister(ctx, []string{eventType})
		if err != nil {
			return false, err
		}
		for _, e := range events {
			if strings.Contains(string(e.Data), substr) {
				return true, nil
			}
		}
		if attempt < attempts {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return false, ctx.Err()
			}
		}
	}
	return false, nil
}
