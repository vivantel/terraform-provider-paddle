package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccPaddleProduct_basic(t *testing.T) {
	resourceName := "paddle_product.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProductArchived(resourceName),
		Steps: []resource.TestStep{
			{
				// Create.
				Config: providerConfig + testAccProductConfig("Acc Test Widget", "standard", `"an acc test product"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", "Acc Test Widget"),
					resource.TestCheckResourceAttr(resourceName, "tax_category", "standard"),
					resource.TestCheckResourceAttr(resourceName, "description", "an acc test product"),
					resource.TestCheckResourceAttr(resourceName, "status", "active"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
				),
			},
			{
				// Update: name and description change in place, no
				// replacement — also exercises the description-clearing
				// fix (docs/decisions client.go Product.Description
				// omitempty removal) by setting a new value here and
				// clearing it in the next step.
				Config: providerConfig + testAccProductConfig("Acc Test Widget Renamed", "standard", `"a renamed acc test product"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", "Acc Test Widget Renamed"),
					resource.TestCheckResourceAttr(resourceName, "description", "a renamed acc test product"),
				),
			},
			{
				// Clear description entirely — regression coverage for
				// the omitempty bug the code review found: this must
				// actually clear server-side, not just locally in state.
				Config: providerConfig + testAccProductConfigNoDescription("Acc Test Widget Renamed", "standard"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(resourceName, "description"),
				),
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

func TestAccPaddleProductDataSource_basic(t *testing.T) {
	dataSourceName := "data.paddle_product.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProductArchived("paddle_product.test"),
		Steps: []resource.TestStep{
			{
				Config: providerConfig + testAccProductConfig("Acc Test Widget For Lookup", "standard", `"looked up via data source"`) + `
data "paddle_product" "test" {
  id = paddle_product.test.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(dataSourceName, "id", "paddle_product.test", "id"),
					resource.TestCheckResourceAttr(dataSourceName, "name", "Acc Test Widget For Lookup"),
					resource.TestCheckResourceAttr(dataSourceName, "tax_category", "standard"),
					resource.TestCheckResourceAttr(dataSourceName, "description", "looked up via data source"),
					resource.TestCheckResourceAttr(dataSourceName, "status", "active"),
				),
			},
		},
	})
}

func testAccProductConfig(name, taxCategory, description string) string {
	return fmt.Sprintf(`
resource "paddle_product" "test" {
  name         = %[1]q
  tax_category = %[2]q
  description  = %[3]s
}
`, name, taxCategory, description)
}

func testAccProductConfigNoDescription(name, taxCategory string) string {
	return fmt.Sprintf(`
resource "paddle_product" "test" {
  name         = %[1]q
  tax_category = %[2]q
}
`, name, taxCategory)
}

// testAccCheckProductArchived is this resource's CheckDestroy. Paddle has
// no hard delete for products — terraform destroy archives instead — so
// "destroyed" here means "status is archived", not "404s".
func testAccCheckProductArchived(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found in state: %s", resourceName)
		}
		id := rs.Primary.ID

		p, err := newTestAccClient().GetProduct(context.Background(), id)
		if err != nil {
			return fmt.Errorf("GetProduct(%s) after destroy: %w", id, err)
		}
		if p.Status != "archived" {
			return fmt.Errorf("product %s status = %q after destroy, want archived", id, p.Status)
		}
		return nil
	}
}
