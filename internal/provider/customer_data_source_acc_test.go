package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// TestAccPaddleCustomerDataSource_byIDAndEmail creates a fixture customer
// (same CreateCustomer call createAdjustmentFixtureTransaction already
// uses, per docs/plans/paddle-provider-v4.md Step 4), looks it up both by
// id and by email, confirms the fields match, then archives it. The email
// contains "acctest" — sweepTestFixtureCustomers
// (internal/provider/sweep_test.go) already sweeps any customer matching
// that substring, so this fixture is covered by the existing sweeper
// without needing its own extension; archived explicitly at the end of
// this test too, rather than only relying on the sweeper, so a normal
// passing run doesn't leave anything for the sweeper to find at all.
func TestAccPaddleCustomerDataSource_byIDAndEmail(t *testing.T) {
	testAccPreCheck(t)
	c := newTestAccClient()
	ctx := context.Background()
	suffix := randAccTestSuffix()
	email := fmt.Sprintf("acctest-customer-ds-%s@example.invalid", suffix)
	name := "Acc Test Customer DS Fixture " + suffix

	cust, err := c.CreateCustomer(ctx, client.Customer{Email: email, Name: name})
	if err != nil {
		t.Fatalf("fixture CreateCustomer: %v", err)
	}
	t.Cleanup(func() {
		if err := c.ArchiveTestFixtureCustomer(context.Background(), cust.ID); err != nil && !client.IsNotFound(err) {
			t.Logf("cleanup: ArchiveTestFixtureCustomer(%s): %v", cust.ID, err)
		}
	})

	dataSourceName := "data.paddle_customer.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
data "paddle_customer" "test" {
  id = %[1]q
}
`, cust.ID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dataSourceName, "id", cust.ID),
					resource.TestCheckResourceAttr(dataSourceName, "email", email),
					resource.TestCheckResourceAttr(dataSourceName, "name", name),
					resource.TestCheckResourceAttr(dataSourceName, "status", "active"),
				),
			},
			{
				Config: providerConfig + fmt.Sprintf(`
data "paddle_customer" "test" {
  email = %[1]q
}
`, email),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dataSourceName, "id", cust.ID),
					resource.TestCheckResourceAttr(dataSourceName, "email", email),
				),
			},
		},
	})
}
