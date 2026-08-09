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
	// Paddle enforces global uniqueness on discount group name (confirmed
	// via a real sandbox 409 discount_group_name_conflict — the push and
	// pull_request CI jobs for the same commit ran this test concurrently
	// against the same sandbox account using what was originally a fixed
	// name). A random suffix per test run avoids that collision.
	suffix := randAccTestSuffix()
	name := "Acc Test Group " + suffix
	renamed := "Acc Test Group Renamed " + suffix

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDiscountGroupArchived(resourceName),
		Steps: []resource.TestStep{
			{
				// Create.
				Config: providerConfig + testAccDiscountGroupConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "status", "active"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
				),
			},
			{
				// Update: name changes in place, no replacement.
				Config: providerConfig + testAccDiscountGroupConfig(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", renamed),
				),
			},
			{
				// A second plan against the same config must be a no-op.
				Config:   providerConfig + testAccDiscountGroupConfig(renamed),
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
	// Same uniqueness reasoning as TestAccPaddleDiscountGroup_basic above.
	name := "Acc Test Group For Lookup " + randAccTestSuffix()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDiscountGroupArchived("paddle_discount_group.test"),
		Steps: []resource.TestStep{
			{
				Config: providerConfig + testAccDiscountGroupConfig(name) + `
data "paddle_discount_group" "test" {
  id = paddle_discount_group.test.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(dataSourceName, "id", "paddle_discount_group.test", "id"),
					resource.TestCheckResourceAttr(dataSourceName, "name", name),
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
