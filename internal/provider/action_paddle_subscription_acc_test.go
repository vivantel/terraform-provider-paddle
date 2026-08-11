package provider

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

var tfVersionChecksActions = []tfversion.TerraformVersionCheck{
	tfversion.SkipBelow(version.Must(version.NewVersion("1.14.0"))),
}

// findTestSubscription is shared by every subscription action test below
// — subscriptions can't be created via pure API calls at all (confirmed
// 2026-08-10: only a real checkout + a test card produces one), so none
// of these tests can self-provision a fixture the way every other test in
// this repo does. Same pattern docs/plans/paddle-provider-v2.md Step 6
// established for paddle_checkout_domain (see
// TestAccPaddleCheckoutDomainDataSource_basic): the lookup and skip
// happen *before* resource.Test is called, in the test function body,
// not inside PreCheck — Config strings are plain Go strings built once
// when the resource.TestCase struct literal is evaluated, before PreCheck
// ever runs, so a value only known after a dynamic lookup can't be
// threaded through PreCheck the way it can here.
//
// Two modes, in priority order:
//
//  1. PADDLE_TEST_SUBSCRIPTION_ID set: fetch that exact subscription and
//     use it only if its current status matches — a deliberately pinned,
//     recognizable fixture (a specific test customer/subscription created
//     once via a real sandbox checkout) rather than "whatever this
//     account happens to have lying around". Skips cleanly, with the
//     subscription's actual current status in the message, if it isn't
//     currently in the wanted state (e.g. a previous pause/resume run
//     left it somewhere unexpected) — never silently falls back to
//     searching the whole account instead once a specific ID is pinned,
//     since that would defeat the point of pinning one.
//  2. Unset: list every subscription in the account and use the first
//     one matching the wanted status — the original, less predictable
//     behavior, kept as a fallback for environments that haven't pinned
//     one yet.
func findTestSubscription(t *testing.T, c *client.Client, status string) *client.Subscription {
	t.Helper()
	ctx := context.Background()

	if id := os.Getenv("PADDLE_TEST_SUBSCRIPTION_ID"); id != "" {
		sub, err := c.GetSubscription(ctx, id)
		if err != nil {
			t.Fatalf("GetSubscription(%q) (from PADDLE_TEST_SUBSCRIPTION_ID): %v", id, err)
		}
		if sub.Status != status {
			t.Skipf("PADDLE_TEST_SUBSCRIPTION_ID=%s is currently %q, want %q for this test — leave it to settle back (pause/resume tests always return it to active) or wait for another test run, rather than pointing this test at a different subscription instead", id, sub.Status, status)
		}
		return sub
	}

	subs, err := c.ListSubscriptions(ctx)
	if err != nil {
		t.Fatalf("ListSubscriptions: %v", err)
	}
	for i := range subs {
		if subs[i].Status == status {
			return &subs[i]
		}
	}
	return nil
}

// TestAccPaddleSubscriptionCancel_invalidSubscriptionID always runs (needs
// no fixture subscription) — confirms this action's real HTTP round trip
// against the sandbox (auth, URL construction, error surfacing) works end
// to end, even though the destructive success path can't run
// automatically in this test suite: this action's schema description
// warns cancellation "can't be undone", so deliberately never invoking it
// against a real active subscription just to prove a positive case is the
// right call here, not a coverage gap to fill in later — see
// TestAccPaddleSubscriptionCancel_alreadyCanceledShortCircuits for the
// safe half of this action's real-sandbox coverage instead.
func TestAccPaddleSubscriptionCancel_invalidSubscriptionID(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   tfVersionChecksActions,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
action "paddle_subscription_cancel" "test" {
  config {
    subscription_id = "sub_does_not_exist_00000000000000000"
    effective_from  = "immediately"
  }
}

resource "terraform_data" "trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.paddle_subscription_cancel.test]
    }
  }
}
`,
				ExpectError: regexp.MustCompile(`(?s)Error reading Paddle subscription`),
			},
		},
	})
}

// TestAccPaddleSubscriptionCancel_alreadyCanceledShortCircuits is the safe
// half of this action's real-sandbox coverage: rather than destructively
// canceling a real active subscription (irreversible), this looks for a
// subscription that's *already* canceled and confirms invoking cancel
// against it applies cleanly (search-before-invoke's short-circuit,
// docs/guardrails/money-moving-actions-no-blanket-retry.md) instead of
// erroring. Skips cleanly if no already-canceled subscription exists.
func TestAccPaddleSubscriptionCancel_alreadyCanceledShortCircuits(t *testing.T) {
	testAccPreCheck(t)
	c := newTestAccClient()
	sub := findTestSubscription(t, c, "canceled")
	if sub == nil {
		t.Skip("no already-canceled subscription found in the sandbox account — skipping (see findTestSubscription's comment on why this test can't self-provision one)")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   tfVersionChecksActions,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
action "paddle_subscription_cancel" "test" {
  config {
    subscription_id = %q
    effective_from  = "immediately"
  }
}

resource "terraform_data" "trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.paddle_subscription_cancel.test]
    }
  }
}
`, sub.ID),
				// No PostApplyFunc assertion beyond "apply succeeded
				// without error" — a successful apply here already
				// proves the short-circuit path ran (an unconditional
				// re-cancel of an already-canceled subscription would
				// otherwise surface Paddle's own rejection).
			},
		},
	})
}

// TestAccPaddleSubscriptionPauseResume_roundTrip is this action pair's
// real-sandbox success-path coverage: pause then resume is reversible
// (unlike cancel), so this exercises the actual mutating calls against a
// real subscription rather than only a short-circuit path — pauses an
// active subscription, confirms status becomes paused, resumes it,
// confirms status returns to active. Skips cleanly if no active
// subscription exists in the sandbox account.
func TestAccPaddleSubscriptionPauseResume_roundTrip(t *testing.T) {
	testAccPreCheck(t)
	c := newTestAccClient()
	sub := findTestSubscription(t, c, "active")
	if sub == nil {
		t.Skip("no active subscription found in the sandbox account — skipping (see findTestSubscription's comment on why this test can't self-provision one)")
	}

	pauseConfig := providerConfig + fmt.Sprintf(`
action "paddle_subscription_pause" "test" {
  config {
    subscription_id = %[1]q
    effective_from  = "immediately"
  }
}

resource "terraform_data" "trigger" {
  input = "pause"
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.paddle_subscription_pause.test]
    }
  }
}
`, sub.ID)

	resumeConfig := providerConfig + fmt.Sprintf(`
action "paddle_subscription_resume" "test" {
  config {
    subscription_id = %[1]q
    effective_from  = "immediately"
  }
}

resource "terraform_data" "trigger" {
  input = "resume"
  lifecycle {
    action_trigger {
      events  = [after_create, after_update]
      actions = [action.paddle_subscription_resume.test]
    }
  }
}
`, sub.ID)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   tfVersionChecksActions,
		Steps: []resource.TestStep{
			{
				Config: pauseConfig,
				PostApplyFunc: func() {
					got, err := c.GetSubscription(context.Background(), sub.ID)
					if err != nil {
						t.Fatalf("GetSubscription: %v", err)
					}
					if got.Status != "paused" {
						t.Errorf("subscription %s status = %q after pause, want %q", sub.ID, got.Status, "paused")
					}
				},
			},
			{
				Config: resumeConfig,
				PostApplyFunc: func() {
					got, err := c.GetSubscription(context.Background(), sub.ID)
					if err != nil {
						t.Fatalf("GetSubscription: %v", err)
					}
					if got.Status != "active" {
						t.Errorf("subscription %s status = %q after resume, want %q", sub.ID, got.Status, "active")
					}
				},
			},
		},
	})
}

// TestAccPaddleSubscriptionCharge_roundTrip exercises paddle_subscription_charge
// against a real active subscription, invoked twice to prove
// search-before-invoke prevents a duplicate charge
// (docs/guardrails/money-moving-actions-no-blanket-retry.md) — the same
// standard TestAccPaddleAdjustment_basic holds paddle_adjustment to.
// effective_from is "next_billing_period" deliberately, not "immediately"
// — avoids triggering an immediate real invoice/receipt send even in
// sandbox. Requires a paddle_price fixture with a real catalog price;
// creates its own via the client directly (same fixture-outside-Terraform
// pattern as action_paddle_adjustment_acc_test.go) since this test has no
// way to know in advance which price(s), if any, the found subscription
// already bills — a *new* one-time catalog price avoids any ambiguity
// about what "matching items" means against pre-existing subscription
// items. Skips cleanly if no active subscription exists.
func TestAccPaddleSubscriptionCharge_roundTrip(t *testing.T) {
	testAccPreCheck(t)
	c := newTestAccClient()
	sub := findTestSubscription(t, c, "active")
	if sub == nil {
		t.Skip("no active subscription found in the sandbox account — skipping (see findTestSubscription's comment on why this test can't self-provision one)")
	}

	suffix := randAccTestSuffix()
	ctx := context.Background()
	prod, err := c.CreateProduct(ctx, client.Product{Name: "Acc Test Charge Fixture " + suffix, TaxCategory: "standard"})
	if err != nil {
		t.Fatalf("fixture CreateProduct: %v", err)
	}
	price, err := c.CreatePrice(ctx, client.Price{
		ProductID:   prod.ID,
		Description: "Acc Test Charge Fixture " + suffix,
		UnitPrice:   client.Money{Amount: "500", CurrencyCode: "USD"},
	})
	if err != nil {
		t.Fatalf("fixture CreatePrice: %v", err)
	}

	config := func(triggerInput string) string {
		return providerConfig + fmt.Sprintf(`
action "paddle_subscription_charge" "test" {
  config {
    subscription_id = %[1]q
    effective_from  = "next_billing_period"
    items = [
      {
        price_id = %[2]q
        quantity = 1
      }
    ]
  }
}

resource "terraform_data" "trigger" {
  input = %[3]q
  lifecycle {
    action_trigger {
      events  = [after_create, after_update]
      actions = [action.paddle_subscription_charge.test]
    }
  }
}
`, sub.ID, price.ID, triggerInput)
	}

	// Verifies against Paddle's next_transaction renewal preview, not
	// ListSubscriptionChargeTransactions — this test's config uses
	// effective_from="next_billing_period", and Paddle creates no
	// queryable transaction at all for that until the subscription
	// actually renews (found the hard way, 2026-08-11: the transaction
	// search reported 0 both before and after invoking, since it was
	// checking the wrong thing entirely — same root cause the action's
	// own search-before-invoke needed fixing for, see
	// docs/guardrails/money-moving-actions-no-blanket-retry.md). Counts
	// occurrences within the preview's own item list rather than just
	// checking presence, so a real dedup failure (the action somehow
	// queuing the item twice) would still be caught as a count of 2, not
	// silently pass as "present".
	countCharges := func() int {
		t.Helper()
		preview, err := c.GetSubscriptionNextTransaction(context.Background(), sub.ID)
		if err != nil {
			t.Fatalf("GetSubscriptionNextTransaction: %v", err)
		}
		if preview == nil {
			return 0
		}
		count := 0
		for _, item := range preview.Items {
			if item.PriceID == price.ID && item.Quantity == 1 {
				count++
			}
		}
		return count
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   tfVersionChecksActions,
		Steps: []resource.TestStep{
			{
				Config: config("v1"),
				PostApplyFunc: func() {
					if got := countCharges(); got != 1 {
						t.Errorf("charges matching price %s on subscription %s = %d, want exactly 1 after the first invoke", price.ID, sub.ID, got)
					}
				},
			},
			{
				Config: config("v2"),
				PostApplyFunc: func() {
					if got := countCharges(); got != 1 {
						t.Errorf("charges matching price %s on subscription %s = %d, want exactly 1 after a second invoke (search-before-invoke should have prevented a duplicate)", price.ID, sub.ID, got)
					}
				},
			},
		},
	})
}
