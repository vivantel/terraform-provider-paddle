package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// createAdjustmentFixtureTransaction provisions everything create-adjustment
// needs, entirely outside Terraform: a Customer, an Address, a Product +
// Price (these two ARE managed resources elsewhere in this provider, but
// are created here via direct client calls too, for a single consistent
// fixture-setup path), and a manually-collected, pre-billed Transaction —
// create-adjustment requires the target transaction to be completed
// (auto-collected) or billed/past_due (manually-collected); billed is
// reachable via a direct API call, no real checkout/payment needed
// (confirmed against the real API reference 2026-08-10). None of
// Customer/Address/Transaction are managed resources in this provider —
// see internal/client/client.go's "Test fixture support only" section —
// so this can't be expressed as HCL the way every other fixture in this
// repo is; it's the same class of constraint
// docs/plans/paddle-provider-v2.md's Step 6 hit for paddle_checkout_domain,
// just solvable here since (unlike a checkout domain) a fixture
// transaction can be created by direct API call at all.
func createAdjustmentFixtureTransaction(t *testing.T, c *client.Client) string {
	t.Helper()
	ctx := context.Background()
	suffix := randAccTestSuffix()

	// Name is required, not just Email — found against the real sandbox,
	// 2026-08-11: Paddle rejects a manual-collection transaction with
	// transaction_customer_not_suitable_for_collection_mode if the
	// attached customer has no name (manual collection produces an
	// invoice, and invoices need a name to generate).
	cust, err := c.CreateCustomer(ctx, client.Customer{
		Email: fmt.Sprintf("acctest-adjustment-%s@example.invalid", suffix),
		Name:  "Acc Test Fixture Customer " + suffix,
	})
	if err != nil {
		t.Fatalf("fixture CreateCustomer: %v", err)
	}
	// All five fields set — CountryCode alone (or +PostalCode) isn't
	// enough for a manual-collection transaction, found against the real
	// sandbox 2026-08-11: transaction_address_not_suitable_for_collection_mode.
	addr, err := c.CreateAddress(ctx, cust.ID, client.Address{
		CountryCode: "US",
		FirstLine:   "123 Test St",
		City:        "San Francisco",
		Region:      "CA",
		PostalCode:  "94103",
	})
	if err != nil {
		t.Fatalf("fixture CreateAddress: %v", err)
	}
	prod, err := c.CreateProduct(ctx, client.Product{Name: "Acc Test Adjustment Fixture " + suffix, TaxCategory: "standard"})
	if err != nil {
		t.Fatalf("fixture CreateProduct: %v", err)
	}
	price, err := c.CreatePrice(ctx, client.Price{
		ProductID:   prod.ID,
		Description: "Acc Test Adjustment Fixture " + suffix,
		UnitPrice:   client.Money{Amount: "1000", CurrencyCode: "USD"},
	})
	if err != nil {
		t.Fatalf("fixture CreatePrice: %v", err)
	}
	txn, err := c.CreateTransaction(ctx, client.TransactionCreate{
		Items:          []client.TransactionCreateItem{{PriceID: price.ID, Quantity: 1}},
		CustomerID:     cust.ID,
		AddressID:      addr.ID,
		CollectionMode: "manual",
		Status:         "billed",
		BillingDetails: &client.BillingDetails{PaymentTerms: client.PaymentTerms{Interval: "day", Frequency: 14}},
	})
	if err != nil {
		// A real, found-in-CI environmental precondition, not a code bug:
		// this sandbox account has no default payment link configured —
		// Paddle requires one before *any* transaction can be created via
		// the API, even a fully manual, non-checkout one. Same class of
		// one-time manual dashboard step as paddle_checkout_domain's
		// domain-approval precondition (docs/plans/paddle-provider-v2.md
		// Step 6) — skip cleanly rather than fail, so CI accurately
		// reflects "blocked on account setup" instead of "code is
		// broken", same distinction that precedent already established.
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && strings.Contains(apiErr.Body, "transaction_default_checkout_url_not_set") {
			t.Skip("this sandbox account has no default payment link set (Paddle dashboard → Checkout → your default payment link/pay link) — required before any transaction can be created via the API at all, even a manual one. Set it once, then this test's real success path will run. See README.md's Actions section.")
		}
		t.Fatalf("fixture CreateTransaction: %v", err)
	}
	return txn.ID
}

// testAccAdjustmentConfig's terraform_data trigger fires the action on
// both after_create and after_update, so the two-step
// invoke-twice-confirm-once test below can force a second invocation by
// changing triggerInput between steps (an in-place update, not a
// replacement) rather than needing two separate resources.
func testAccAdjustmentConfig(transactionID, reason, triggerInput string) string {
	return providerConfig + fmt.Sprintf(`
action "paddle_adjustment" "test" {
  config {
    action         = "credit"
    transaction_id = %q
    reason         = %q
  }
}

resource "terraform_data" "trigger" {
  input = %q
  lifecycle {
    action_trigger {
      events  = [after_create, after_update]
      actions = [action.paddle_adjustment.test]
    }
  }
}
`, transactionID, reason, triggerInput)
}

// countMatchingAdjustments is shared by both steps below so a mismatch in
// what's being compared can't drift between them.
func countMatchingAdjustments(t *testing.T, c *client.Client, transactionID, reason string) int {
	t.Helper()
	adjustments, err := c.ListAdjustments(context.Background(), transactionID)
	if err != nil {
		t.Fatalf("ListAdjustments: %v", err)
	}
	count := 0
	for _, a := range adjustments {
		if a.Reason == reason && a.Action == "credit" {
			count++
		}
	}
	return count
}

// TestAccPaddleAdjustment_basic is this action's real-sandbox
// invoke-twice-confirm-once proof, required by
// docs/plans/paddle-provider-v3.md Step 1 item 5 and
// docs/guardrails/money-moving-actions-no-blanket-retry.md: the whole
// point of this action's search-before-invoke check is that a second
// invocation against the same transaction+reason must not create a
// second adjustment. Uses "credit" rather than "refund" — a credit
// doesn't require an already-paid transaction, only completed/billed/
// past_due, which the manually-collected "billed" fixture above already
// satisfies without needing a real captured payment.
func TestAccPaddleAdjustment_basic(t *testing.T) {
	testAccPreCheck(t)
	c := newTestAccClient()
	reason := "Acc Test adjustment " + randAccTestSuffix()

	// Deliberately created *before* resource.Test() is called, not inside
	// PreCheck — this was a real bug, found running against the real
	// sandbox (2026-08-11): resource.TestCase's Steps slice literal
	// (built below) is evaluated immediately, before PreCheck ever runs.
	// Building `Config: testAccAdjustmentConfig(transactionID, ...)` while
	// transactionID was still only set inside PreCheck meant every
	// Config baked in an empty transaction_id, always — the fixture
	// itself was created successfully, just never actually used, and
	// every apply searched/created against "" instead. Same class of bug
	// action_paddle_subscription_acc_test.go's findTestSubscription
	// comment already documents avoiding, for the same underlying reason
	// (Go closures over a struct literal's fields don't defer
	// evaluation the way a function call's arguments don't either) —
	// missed here because this test creates its own fixture instead of
	// just looking one up, and that distinction didn't register at the
	// time as the same hazard.
	transactionID := createAdjustmentFixtureTransaction(t, c)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(version.Must(version.NewVersion("1.14.0"))),
		},
		Steps: []resource.TestStep{
			{
				Config: testAccAdjustmentConfig(transactionID, reason, "v1"),
				PostApplyFunc: func() {
					if got := countMatchingAdjustments(t, c, transactionID, reason); got != 1 {
						t.Errorf("adjustments matching reason %q for transaction %s = %d, want exactly 1 after the first invoke", reason, transactionID, got)
					}
				},
			},
			{
				// Same transaction_id + reason, forced re-invocation via
				// after_update — search-before-invoke must recognize the
				// adjustment already created in the previous step and
				// skip, not create a second one.
				Config: testAccAdjustmentConfig(transactionID, reason, "v2"),
				PostApplyFunc: func() {
					if got := countMatchingAdjustments(t, c, transactionID, reason); got != 1 {
						t.Errorf("adjustments matching reason %q for transaction %s = %d, want exactly 1 after a second invoke (search-before-invoke should have prevented a duplicate)", reason, transactionID, got)
					}
				},
			},
		},
	})
}
