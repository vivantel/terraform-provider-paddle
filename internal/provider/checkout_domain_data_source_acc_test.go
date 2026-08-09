package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccPaddleCheckoutDomainDataSource_basic can't provision its own
// fixture the way every other acceptance test in this provider does —
// there's no create operation for checkout domains at all (see
// client.go's Checkout Domains section comment), so this test lists
// whatever already exists in the sandbox account and skips cleanly if
// there isn't one, rather than failing on an environment that happens not
// to have a dashboard-approved domain yet. If one does exist, this
// confirms the data source actually round-trips a real domain's fields,
// including the nested payment_method_verification.apple_pay.status —
// not just the fabricated unit test values in
// checkout_domain_data_source_test.go.
func TestAccPaddleCheckoutDomainDataSource_basic(t *testing.T) {
	testAccPreCheck(t)
	c := newTestAccClient()
	domains, err := c.ListCheckoutDomains(context.Background())
	if err != nil {
		t.Fatalf("ListCheckoutDomains: %v", err)
	}
	if len(domains) == 0 {
		t.Skip("no checkout domains exist in this sandbox account — nothing to look up (there's no API to create one for this test)")
	}
	want := domains[0]
	dataSourceName := "data.paddle_checkout_domain.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
data "paddle_checkout_domain" "test" {
  id = %[1]q
}
`, want.ID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dataSourceName, "id", want.ID),
					resource.TestCheckResourceAttr(dataSourceName, "domain", want.Domain),
					resource.TestCheckResourceAttr(dataSourceName, "status", want.Status),
					resource.TestCheckResourceAttrSet(dataSourceName, "payment_method_verification.apple_pay.status"),
				),
			},
		},
	})
}
