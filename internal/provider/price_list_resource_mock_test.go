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

// priceListMockStore — see productListMockStore's comment in
// product_list_resource_mock_test.go for what this is and isn't: a fixed,
// pre-populated GET-only stand-in for Paddle's list-prices endpoint,
// separate from priceMockStore (price_resource_mock_test.go), which holds
// exactly one price and has no list endpoint at all.
type priceListMockStore struct {
	prices []client.Price
}

func (s *priceListMockStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodGet && r.URL.Path == "/prices" {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": s.prices,
			"meta": map[string]any{"pagination": map[string]any{"has_more": false}},
		})
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

// TestMockPaddlePriceListResource_query — see
// TestMockPaddleProductListResource_query's comment for why this uses
// querycheck and a leading plain Config step.
func TestMockPaddlePriceListResource_query(t *testing.T) {
	store := &priceListMockStore{
		prices: []client.Price{
			{ID: "pri_mock_1", ProductID: "pro_mock_1", Description: "Mock Price One", Status: "active",
				UnitPrice: client.Money{Amount: "1000", CurrencyCode: "USD"}},
			{ID: "pri_mock_2", ProductID: "pro_mock_1", Description: "Mock Price Two", Status: "active",
				UnitPrice: client.Money{Amount: "2000", CurrencyCode: "USD"}},
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
				Config: mockProviderConfig,
			},
			{
				Query: true,
				Config: `
list "paddle_price" "test" {
  provider = paddle

  config {}

  include_resource = true
}
`,
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLength("paddle_price.test", 2),
					querycheck.ExpectIdentity("paddle_price.test", map[string]knownvalue.Check{
						"id": knownvalue.StringExact("pri_mock_1"),
					}),
					querycheck.ExpectResourceKnownValues(
						"paddle_price.test",
						queryfilter.ByResourceIdentity(map[string]knownvalue.Check{
							"id": knownvalue.StringExact("pri_mock_2"),
						}),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("description"), KnownValue: knownvalue.StringExact("Mock Price Two")},
							{Path: tfjsonpath.New("unit_price").AtMapKey("amount"), KnownValue: knownvalue.StringExact("2000")},
						},
					),
				},
			},
		},
	})
}
