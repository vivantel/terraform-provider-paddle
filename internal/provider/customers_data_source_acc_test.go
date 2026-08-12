package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// TestAccPaddleCustomersDataSource_byEmail — same fixture-provisioning
// pattern customer_data_source_acc_test.go's singular test already uses.
func TestAccPaddleCustomersDataSource_byEmail(t *testing.T) {
	testAccPreCheck(t)
	c := newTestAccClient()
	ctx := context.Background()
	suffix := randAccTestSuffix()
	email := fmt.Sprintf("acctest-customers-ds-%s@example.invalid", suffix)
	name := "Acc Test Customers DS Fixture " + suffix

	cust, err := c.CreateCustomer(ctx, client.Customer{Email: email, Name: name})
	if err != nil {
		t.Fatalf("fixture CreateCustomer: %v", err)
	}
	t.Cleanup(func() {
		if err := c.ArchiveTestFixtureCustomer(context.Background(), cust.ID); err != nil && !client.IsNotFound(err) {
			t.Logf("cleanup: ArchiveTestFixtureCustomer(%s): %v", cust.ID, err)
		}
	})

	dataSourceName := "data.paddle_customers.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
data "paddle_customers" "test" {
  email = %[1]q
}
`, email),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dataSourceName, "customers.#", "1"),
					resource.TestCheckResourceAttr(dataSourceName, "customers.0.id", cust.ID),
					resource.TestCheckResourceAttr(dataSourceName, "customers.0.email", email),
					resource.TestCheckResourceAttr(dataSourceName, "customers.0.name", name),
				),
			},
		},
	})
}
