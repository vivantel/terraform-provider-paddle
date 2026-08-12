package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// TestAccPaddleTransactionsDataSource_byFilter filters by the pinned
// subscription fixture's subscription_id (findTestSubscription,
// action_paddle_subscription_acc_test.go) — its recurring billing history
// already has transactions, so this needs no new fixture provisioning at
// all, unlike an earlier version of this test that called
// createAdjustmentFixtureTransaction (which creates its own Customer +
// Address + Transaction via direct API calls). That version hit real
// sandbox rate limits in CI (429 too_many_requests / context deadline
// exceeded on CreateCustomer) once combined with every other acceptance
// test's own customer-creating fixtures in the same run — found the hard
// way via a real CI failure, not assumed. Reusing an existing fixture
// here, per the plan's own instruction, isn't just "closer to the
// letter of the instruction" — it measurably reduces load on a shared,
// rate-limited sandbox account.
func TestAccPaddleTransactionsDataSource_byFilter(t *testing.T) {
	testAccPreCheck(t)
	c := newTestAccClient()
	sub := findTestSubscription(t, c, "active")
	if sub == nil {
		t.Skip("no active subscription found in the sandbox account — skipping")
	}
	txns, err := c.ListTransactionsFiltered(context.Background(), client.TransactionListFilter{SubscriptionID: sub.ID, Limit: 1})
	if err != nil {
		t.Fatalf("ListTransactionsFiltered: %v", err)
	}
	if len(txns) == 0 {
		t.Skip("the pinned test subscription has no transactions yet — skipping")
	}
	want := txns[0]
	dataSourceName := "data.paddle_transactions.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   tfVersionChecksActions,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
data "paddle_transactions" "test" {
  subscription_id = %[1]q
}
`, sub.ID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(dataSourceName, "transactions.#"),
					checkListContainsID(dataSourceName, "transactions", want.ID),
				),
			},
		},
	})
}
