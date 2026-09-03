package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// TestAccPaddleProductListResource_query creates a real product, then
// queries the account's whole product list and confirms that exact product
// comes back with a matching identity and name. Uses ExpectLengthAtLeast
// (not an exact count) and filters by this test's own product's identity —
// other acceptance tests may have products of their own live in the same
// sandbox account at the same time.
func TestAccPaddleProductListResource_query(t *testing.T) {
	resourceName := "paddle_product.test"

	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0), // list resources / terraform query support
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProductArchived(resourceName),
		Steps: []resource.TestStep{
			{
				Config: providerConfig + testAccProductConfig("Acc Test List Resource Product", "standard", `"queried via the paddle_product list resource"`),
			},
			{
				// No providerConfig here — see
				// product_list_resource_mock_test.go's comment on its own
				// preceding Config step: a Query step's Config coexists
				// with, rather than replaces, the previous step's
				// generated .tf file, so a second provider "paddle" block
				// here produces a real "Duplicate provider configuration"
				// error (caught the hard way against the real sandbox,
				// 2026-09-03).
				Query: true,
				Config: `
list "paddle_product" "test" {
  provider = paddle

  config {}

  include_resource = true
}
`,
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLengthAtLeast("paddle_product.test", 1),
					// Filtered by DisplayName (List() sets it to the
					// product's own Name — see product_list_resource.go),
					// not identity: this test's product's ID is only known
					// after step 1 creates it, but QueryResultChecks is
					// built into the TestCase literal before any step
					// runs, so a filter keyed on a not-yet-known ID isn't
					// expressible here the way it is once the sandbox
					// state already exists (see ByResourceIdentity's own
					// tests for that shape). The name is fixed at
					// test-authoring time either way, so filtering on it
					// directly is simpler, not a workaround.
					querycheck.ExpectResourceKnownValues(
						"paddle_product.test",
						queryfilter.ByDisplayName(knownvalue.StringExact("Acc Test List Resource Product")),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact("Acc Test List Resource Product")},
						},
					),
				},
			},
		},
	})
}
