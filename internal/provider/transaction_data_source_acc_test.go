package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// TestAccPaddleTransactionDataSource_feedsAdjustment is this data
// source's most valuable proof, per docs/plans/paddle-provider-v4.md Step
// 3: look up a transaction via paddle_transaction, then feed its
// line_items[0].item_id straight into a real paddle_adjustment action
// invocation, entirely via Terraform references (not a value this test
// computed in Go and interpolated in) — confirming this data source
// actually closes paddle_adjustment's item_id discovery gap, not just
// that it returns correctly-shaped data in isolation.
func TestAccPaddleTransactionDataSource_feedsAdjustment(t *testing.T) {
	testAccPreCheck(t)
	c := newTestAccClient()
	reason := "Acc Test transaction data source " + randAccTestSuffix()

	// Same closure-evaluation-order reasoning as
	// TestAccPaddleAdjustment_basic — must be created before
	// resource.Test() is called, not inside PreCheck.
	transaction := createAdjustmentFixtureTransaction(t, c)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(version.Must(version.NewVersion("1.14.0"))),
		},
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
data "paddle_transaction" "test" {
  id = %[1]q
}

action "paddle_adjustment" "test" {
  config {
    action         = "credit"
    type           = "full"
    transaction_id = data.paddle_transaction.test.id
    reason         = %[2]q
    items = [
      {
        item_id = data.paddle_transaction.test.line_items[0].item_id
        type    = "full"
      }
    ]
  }
}

resource "terraform_data" "trigger" {
  input = "v1"
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.paddle_adjustment.test]
    }
  }
}
`, transaction.ID, reason),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.paddle_transaction.test", "id", transaction.ID),
					resource.TestCheckResourceAttrSet("data.paddle_transaction.test", "line_items.0.item_id"),
				),
				PostApplyFunc: func() {
					if got := countMatchingAdjustments(t, c, transaction.ID, reason); got != 1 {
						t.Errorf("adjustments matching reason %q for transaction %s = %d, want exactly 1 — data.paddle_transaction.test.line_items[0].item_id must not have resolved to a working item_id", reason, transaction.ID, got)
					}
				},
			},
		},
	})
}

// TestAccPaddleTransactionDataSource_byFilter exercises the
// subscription_id/customer_id/status filter-lookup path — only the
// id-lookup path had sandbox coverage before this (found via code
// review; paddle_subscription already has both a _byID and _byFilter
// test, so this closes an inconsistency within this same diff). Confirms
// the filter branch's "exactly one match, re-fetch by ID for line_items"
// logic resolves to the same transaction the id-based lookup would.
func TestAccPaddleTransactionDataSource_byFilter(t *testing.T) {
	testAccPreCheck(t)
	c := newTestAccClient()
	transaction := createAdjustmentFixtureTransaction(t, c)
	if transaction.CustomerID == "" {
		t.Fatal("fixture transaction has no customer_id — can't exercise the filter-lookup path")
	}
	dataSourceName := "data.paddle_transaction.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
data "paddle_transaction" "test" {
  customer_id = %[1]q
  status      = %[2]q
}
`, transaction.CustomerID, transaction.Status),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dataSourceName, "id", transaction.ID),
					resource.TestCheckResourceAttr(dataSourceName, "customer_id", transaction.CustomerID),
					resource.TestCheckResourceAttrSet(dataSourceName, "line_items.0.item_id"),
				),
			},
		},
	})
}
