package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccPaddleSubscriptionsDataSource_byFilter reuses findTestSubscription
// (action_paddle_subscription_acc_test.go) — the same pinned fixture
// subscription_data_source_acc_test.go's singular tests already depend on
// — filtered by customer_id/status, and confirms the pinned subscription
// is somewhere in the matching list, not necessarily the only entry (the
// sandbox account may have other subscriptions from other test runs).
func TestAccPaddleSubscriptionsDataSource_byFilter(t *testing.T) {
	testAccPreCheck(t)
	c := newTestAccClient()
	sub := findTestSubscription(t, c, "active")
	if sub == nil || sub.CustomerID == "" {
		t.Skip("no active subscription with a customer_id found in the sandbox account — skipping")
	}
	dataSourceName := "data.paddle_subscriptions.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   tfVersionChecksActions,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
data "paddle_subscriptions" "test" {
  customer_id = %[1]q
  status      = %[2]q
}
`, sub.CustomerID, sub.Status),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(dataSourceName, "subscriptions.#"),
					checkListContainsID(dataSourceName, "subscriptions", sub.ID),
				),
			},
		},
	})
}
