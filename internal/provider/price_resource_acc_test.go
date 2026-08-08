package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccPaddlePrice_basic(t *testing.T) {
	resourceName := "paddle_price.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPriceArchived(resourceName),
		Steps: []resource.TestStep{
			{
				// Create.
				Config: providerConfig + testAccPriceConfig("1000", "USD"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "unit_price.amount", "1000"),
					resource.TestCheckResourceAttr(resourceName, "unit_price.currency_code", "USD"),
					resource.TestCheckResourceAttr(resourceName, "status", "active"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "product_id"),
					// Regression coverage: quantity was left unset in
					// config, so this must not perpetually plan as
					// "known after apply" (price_resource.go's quantity
					// Default/UseStateForUnknown fix).
					resource.TestCheckResourceAttrSet(resourceName, "quantity.minimum"),
					resource.TestCheckResourceAttrSet(resourceName, "quantity.maximum"),
				),
			},
			{
				// Update: amount changes in place, no replacement.
				Config: providerConfig + testAccPriceConfig("2000", "USD"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "unit_price.amount", "2000"),
				),
			},
			{
				// A second plan against the same config as the previous
				// step must be a no-op — this is exactly what the
				// quantity UseStateForUnknown bug would have broken.
				Config:   providerConfig + testAccPriceConfig("2000", "USD"),
				PlanOnly: true,
			},
			{
				// Import.
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccPaddlePriceDataSource_basic(t *testing.T) {
	dataSourceName := "data.paddle_price.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPriceArchived("paddle_price.test"),
		Steps: []resource.TestStep{
			{
				Config: providerConfig + testAccPriceConfig("1500", "USD") + `
data "paddle_price" "test" {
  id = paddle_price.test.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(dataSourceName, "id", "paddle_price.test", "id"),
					resource.TestCheckResourceAttrPair(dataSourceName, "product_id", "paddle_price.test", "product_id"),
					resource.TestCheckResourceAttr(dataSourceName, "unit_price.amount", "1500"),
					resource.TestCheckResourceAttr(dataSourceName, "unit_price.currency_code", "USD"),
					resource.TestCheckResourceAttr(dataSourceName, "status", "active"),
					resource.TestCheckResourceAttrSet(dataSourceName, "quantity.minimum"),
					resource.TestCheckResourceAttrSet(dataSourceName, "quantity.maximum"),
				),
			},
		},
	})
}

func testAccPriceConfig(amount, currency string) string {
	return fmt.Sprintf(`
resource "paddle_product" "test" {
  name         = "Acc Test Price Parent"
  tax_category = "standard"
}

resource "paddle_price" "test" {
  product_id  = paddle_product.test.id
  description = "acc test price"
  unit_price = {
    amount        = %[1]q
    currency_code = %[2]q
  }
}
`, amount, currency)
}

// testAccCheckPriceArchived is this resource's CheckDestroy. Same story as
// products — Paddle archives, doesn't hard-delete, prices.
func testAccCheckPriceArchived(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found in state: %s", resourceName)
		}
		id := rs.Primary.ID

		p, err := newTestAccClient().GetPrice(context.Background(), id)
		if err != nil {
			return fmt.Errorf("GetPrice(%s) after destroy: %w", id, err)
		}
		if p.Status != "archived" {
			return fmt.Errorf("price %s status = %q after destroy, want archived", id, p.Status)
		}
		return nil
	}
}
