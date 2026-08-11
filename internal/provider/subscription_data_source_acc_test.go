package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccPaddleSubscriptionDataSource_byID reuses findTestSubscription
// (action_paddle_subscription_acc_test.go) rather than provisioning a
// third fixture — both PADDLE_TEST_SUBSCRIPTION_ID/
// PADDLE_TEST_CANCELED_SUBSCRIPTION_ID-pinned subscriptions are already
// exactly what this data source needs to look up, per
// docs/plans/paddle-provider-v4.md Step 2.
func TestAccPaddleSubscriptionDataSource_byID(t *testing.T) {
	testAccPreCheck(t)
	c := newTestAccClient()
	sub := findTestSubscription(t, c, "active")
	if sub == nil {
		t.Skip("no active subscription found in the sandbox account — skipping (see findTestSubscription's comment on why this test can't self-provision one)")
	}
	dataSourceName := "data.paddle_subscription.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   tfVersionChecksActions,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
data "paddle_subscription" "test" {
  id = %[1]q
}
`, sub.ID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dataSourceName, "id", sub.ID),
					resource.TestCheckResourceAttr(dataSourceName, "status", sub.Status),
					resource.TestCheckResourceAttr(dataSourceName, "customer_id", sub.CustomerID),
					resource.TestCheckResourceAttrSet(dataSourceName, "created_at"),
				),
			},
		},
	})
}

// TestAccPaddleSubscriptionDataSource_byFilter confirms the filter-based
// lookup path (customer_id + status, no id) resolves to the same
// subscription the direct-id lookup above does — the actual usability
// gap this data source was built to close, since a real user looking up
// "the active subscription for this customer" won't already have the
// sub_... ID.
func TestAccPaddleSubscriptionDataSource_byFilter(t *testing.T) {
	testAccPreCheck(t)
	c := newTestAccClient()
	sub := findTestSubscription(t, c, "active")
	if sub == nil || sub.CustomerID == "" {
		t.Skip("no active subscription with a customer_id found in the sandbox account — skipping")
	}
	dataSourceName := "data.paddle_subscription.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   tfVersionChecksActions,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
data "paddle_subscription" "test" {
  customer_id = %[1]q
  status      = %[2]q
}
`, sub.CustomerID, sub.Status),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dataSourceName, "id", sub.ID),
					resource.TestCheckResourceAttr(dataSourceName, "customer_id", sub.CustomerID),
				),
			},
		},
	})
}
