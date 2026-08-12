package provider

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccExampleLookupThenAct_appliesCleanly applies the real, published
// examples/lookup-then-act/main.tf — read from disk byte for byte, not a
// re-typed copy — against the sandbox, proving the example itself is
// real working HCL, not just eyeballed for syntax
// (docs/plans/paddle-provider-v5.md Step 6's own Definition of Done).
//
// Substitutes the placeholder ctm_.../txn_... IDs with real fixture IDs:
//   - the pinned PADDLE_TEST_CANCELED_SUBSCRIPTION_ID fixture (already
//     canceled), not the active pinned fixture other tests depend on
//     staying active — paddle_subscription_cancel's own
//     already-canceled short-circuit
//     (action_paddle_subscription_cancel.go's checkAlreadyInTargetState
//     call) means applying this example against it exercises the real
//     lookup+action wiring without actually canceling anything or
//     touching the shared active fixture.
//   - a fresh, disposable transaction fixture
//     (createAdjustmentFixtureTransaction,
//     action_paddle_adjustment_acc_test.go), the same one
//     TestAccPaddleTransactionDataSource_feedsAdjustment already uses,
//     cleaned up the same way via t.Cleanup.
//
// Strips the example's own `terraform { required_providers {...} } `
// block before use — that block is for a real user's own config
// resolving a released version from the registry; this test runs
// against the in-process dev provider via
// testAccProtoV6ProviderFactories instead, the same as every other
// acceptance test in this package.
func TestAccExampleLookupThenAct_appliesCleanly(t *testing.T) {
	testAccPreCheck(t)
	c := newTestAccClient()

	// findCanceledTestSubscription, not findTestSubscription(t, c,
	// "canceled") — findTestSubscription always checks
	// PADDLE_TEST_SUBSCRIPTION_ID (the pinned *active* fixture other
	// tests depend on staying active) first, regardless of the status
	// argument passed in, and skips if it doesn't match; it would never
	// find the canceled fixture this test actually needs.
	// findCanceledTestSubscription is the dedicated helper for exactly
	// this case — see its own comment in
	// action_paddle_subscription_acc_test.go for why the two aren't the
	// same function.
	canceledSub := findCanceledTestSubscription(t, c)
	if canceledSub == nil {
		t.Skip("no canceled subscription found in the sandbox account (see README.md's PADDLE_TEST_CANCELED_SUBSCRIPTION_ID section) — skipping")
	}
	txn := createAdjustmentFixtureTransaction(t, c)

	raw, err := os.ReadFile("../../examples/lookup-then-act/main.tf")
	if err != nil {
		t.Fatalf("reading examples/lookup-then-act/main.tf: %v", err)
	}
	config := string(raw)

	// Drop everything before the provider block — the leading comment
	// header and the terraform{required_providers{}} block are for a
	// real user's own config, not this in-process test harness.
	if i := strings.Index(config, `provider "paddle" {`); i >= 0 {
		config = config[i:]
	} else {
		t.Fatal(`examples/lookup-then-act/main.tf no longer contains provider "paddle" { — update this test's slicing`)
	}

	replacements := []struct{ old, new string }{
		{`customer_id = "ctm_..." # replace with a real customer ID`, fmt.Sprintf("customer_id = %q", canceledSub.CustomerID)},
		{`status      = "active"`, `status      = "canceled"`},
		{`id = "txn_..." # replace with a real transaction ID`, fmt.Sprintf("id = %q", txn.ID)},
	}
	for _, r := range replacements {
		if !strings.Contains(config, r.old) {
			t.Fatalf("examples/lookup-then-act/main.tf no longer contains %q — update this test's replacements to match the current example", r.old)
		}
		config = strings.Replace(config, r.old, r.new, 1)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   tfVersionChecksActions,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.paddle_subscription.customer_sub", "id", canceledSub.ID),
					resource.TestCheckResourceAttr("data.paddle_transaction.refund_target", "id", txn.ID),
					resource.TestCheckOutput("canceled_subscription_id", canceledSub.ID),
					resource.TestCheckOutput("refunded_transaction_id", txn.ID),
				),
			},
		},
	})
}
