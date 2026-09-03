package provider

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// productListMockStore is a fixed, pre-populated GET-only stand-in for
// Paddle's list-products endpoint — separate from productMockStore
// (product_resource_mock_test.go), which holds exactly one product and has
// no list endpoint at all; a list resource test needs several.
type productListMockStore struct {
	products []client.Product
}

func (s *productListMockStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodGet && r.URL.Path == "/products" {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": s.products,
			"meta": map[string]any{"pagination": map[string]any{"has_more": false}},
		})
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

// TestMockPaddleProductListResource_query drives a real `terraform query`
// (Query: true — see testStepNewQuery) against paddle_product's list
// resource. Query mode has no state of its own to assert on with the usual
// TestCheckResourceAttr, so this uses querycheck, terraform-plugin-testing's
// own purpose-built package for it — same reasoning as
// notification_setting_secret_ephemeral_mock_test.go using echoprovider for
// ephemeral results.
func TestMockPaddleProductListResource_query(t *testing.T) {
	store := &productListMockStore{
		products: []client.Product{
			{ID: "pro_mock_1", Name: "Mock Product One", TaxCategory: "saas", Status: "active"},
			{ID: "pro_mock_2", Name: "Mock Product Two", TaxCategory: "saas", Status: "active"},
		},
	}
	factories := newMockPaddleServer(t, store)

	resource.Test(t, resource.TestCase{
		IsUnitTest: true,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0), // list resources / terraform query support
		},
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				// A preceding plain Config step, deliberately — a Query
				// step's own Config coexists with, rather than replaces,
				// an earlier step's generated .tf file in the same working
				// directory. Including mockProviderConfig in *both* this
				// step's Config and the Query step's below (as a single-
				// step version of this test once did) produces a real
				// "Duplicate provider configuration" error the moment a
				// second step is added — caught the hard way against the
				// real sandbox (2026-09-03, this PR's own CI run) before
				// being reproduced and fixed here. The Query step below
				// deliberately has no provider block of its own for
				// exactly this reason.
				Config: mockProviderConfig,
			},
			{
				Query: true,
				Config: `
list "paddle_product" "test" {
  provider = paddle

  config {}

  include_resource = true
}
`,
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLength("paddle_product.test", 2),
					querycheck.ExpectIdentity("paddle_product.test", map[string]knownvalue.Check{
						"id": knownvalue.StringExact("pro_mock_1"),
					}),
					querycheck.ExpectResourceKnownValues(
						"paddle_product.test",
						queryfilter.ByResourceIdentity(map[string]knownvalue.Check{
							"id": knownvalue.StringExact("pro_mock_2"),
						}),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact("Mock Product Two")},
						},
					),
				},
			},
		},
	})
}
