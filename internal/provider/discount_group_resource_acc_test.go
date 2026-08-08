package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccPaddleDiscountGroup_basic(t *testing.T) {
	resourceName := "paddle_discount_group.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDiscountGroupArchived(resourceName),
		Steps: []resource.TestStep{
			{
				// Create.
				Config: providerConfig + testAccDiscountGroupConfig("Acc Test Group"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", "Acc Test Group"),
					resource.TestCheckResourceAttr(resourceName, "status", "active"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
				),
			},
			{
				// Update: name changes in place, no replacement.
				Config: providerConfig + testAccDiscountGroupConfig("Acc Test Group Renamed"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", "Acc Test Group Renamed"),
				),
			},
			{
				// A second plan against the same config must be a no-op.
				Config:   providerConfig + testAccDiscountGroupConfig("Acc Test Group Renamed"),
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

func TestAccPaddleDiscountGroupDataSource_basic(t *testing.T) {
	dataSourceName := "data.paddle_discount_group.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDiscountGroupArchived("paddle_discount_group.test"),
		Steps: []resource.TestStep{
			{
				Config: providerConfig + testAccDiscountGroupConfig("Acc Test Group For Lookup") + `
data "paddle_discount_group" "test" {
  id = paddle_discount_group.test.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(dataSourceName, "id", "paddle_discount_group.test", "id"),
					resource.TestCheckResourceAttr(dataSourceName, "name", "Acc Test Group For Lookup"),
					resource.TestCheckResourceAttr(dataSourceName, "status", "active"),
				),
			},
		},
	})
}

func testAccDiscountGroupConfig(name string) string {
	return fmt.Sprintf(`
resource "paddle_discount_group" "test" {
  name = %[1]q
}
`, name)
}

// testAccCheckDiscountGroupArchived is this resource's CheckDestroy —
// same "destroyed means archived" pattern as Product/Price/Discount.
func testAccCheckDiscountGroupArchived(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found in state: %s", resourceName)
		}
		id := rs.Primary.ID

		g, err := newTestAccClient().GetDiscountGroup(context.Background(), id)
		if err != nil {
			return fmt.Errorf("GetDiscountGroup(%s) after destroy: %w", id, err)
		}
		if g.Status != "archived" {
			return fmt.Errorf("discount group %s status = %q after destroy, want archived", id, g.Status)
		}
		return nil
	}
}
