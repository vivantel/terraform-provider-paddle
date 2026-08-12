package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccPaddleTransactionsDataSource_byFilter reuses
// createAdjustmentFixtureTransaction (action_paddle_adjustment_acc_test.go)
// rather than provisioning a fourth fixture transaction — the same
// self-provisioning fixture transaction_data_source_acc_test.go's own test
// already depends on.
func TestAccPaddleTransactionsDataSource_byFilter(t *testing.T) {
	testAccPreCheck(t)
	c := newTestAccClient()
	txn := createAdjustmentFixtureTransaction(t, c)
	dataSourceName := "data.paddle_transactions.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
data "paddle_transactions" "test" {
  customer_id = %[1]q
}
`, txn.CustomerID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(dataSourceName, "transactions.#"),
					checkListContainsID(dataSourceName, "transactions", txn.ID),
				),
			},
		},
	})
}
