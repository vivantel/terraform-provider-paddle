package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// TestAccPaddlePriceListResource_query — see
// TestAccPaddleProductListResource_query's comment for the overall shape
// (ExpectLengthAtLeast, filter-by-known-field since this test's own price's
// ID isn't known until step 1 creates it). Filters by Description (List()
// sets DisplayName to it — see price_list_resource.go's comment on why,
// unlike product, price has no top-level Name reliably set) rather than
// identity, same reasoning as the product test — but deliberately does NOT
// reuse testAccPriceConfig's shared "acc test price" description: that
// exact string is also what TestAccPaddlePrice_basic creates via the same
// helper, and two concurrently-running acceptance tests both matching the
// same DisplayName filter would make this test's KnownValueCheck fail
// against whichever one isn't this test's own price. A unique description
// keeps the filter unambiguous regardless of what else is running.
func TestAccPaddlePriceListResource_query(t *testing.T) {
	resourceName := "paddle_price.test"

	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0), // list resources / terraform query support
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPriceArchived(resourceName),
		Steps: []resource.TestStep{
			{
				Config: providerConfig + testAccPriceConfigForList("1234", "USD"),
			},
			{
				// No providerConfig here — see
				// product_list_resource_acc_test.go's comment on the same
				// shape: a Query step's Config coexists with, rather than
				// replaces, the previous step's generated .tf file.
				Query: true,
				Config: `
list "paddle_price" "test" {
  provider = paddle

  config {}

  include_resource = true
}
`,
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLengthAtLeast("paddle_price.test", 1),
					// Filtered by unit_price.amount (fixed at
					// test-authoring time, distinctive enough to not
					// collide with other acceptance tests' own prices
					// running concurrently in the same sandbox account),
					// same reasoning as the product test's DisplayName
					// filter — this price's real ID is only known after
					// step 1 creates it.
					querycheck.ExpectResourceKnownValues(
						"paddle_price.test",
						queryfilter.ByDisplayName(knownvalue.StringExact("acc test list resource price")),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("unit_price").AtMapKey("amount"), KnownValue: knownvalue.StringExact("1234")},
						},
					),
				},
			},
		},
	})
}

// testAccPriceConfigForList is testAccPriceConfig with a description unique
// to this file's own test — see TestAccPaddlePriceListResource_query's
// comment on why reusing the shared helper's fixed description would be a
// real (if rare) source of test flakiness.
func testAccPriceConfigForList(amount, currency string) string {
	return fmt.Sprintf(`
resource "paddle_product" "test" {
  name         = "Acc Test List Resource Price Parent"
  tax_category = "standard"
}

resource "paddle_price" "test" {
  product_id  = paddle_product.test.id
  description = "acc test list resource price"
  unit_price = {
    amount        = %[1]q
    currency_code = %[2]q
  }
}
`, amount, currency)
}
