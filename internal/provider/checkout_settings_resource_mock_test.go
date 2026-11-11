package provider

import (
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// checkoutSettingsMockStore — see productMockStore's comment in
// product_resource_mock_test.go for what this is and isn't. The singleton
// has no ID and no create/delete: both are the same PATCH.
type checkoutSettingsMockStore struct {
	mu      sync.Mutex
	url     *string
	patches []string // raw PATCH bodies, in order
}

func (s *checkoutSettingsMockStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")

	if r.URL.Path != "/settings/checkout-url" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
	case http.MethodPatch:
		var raw map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&raw)
		b, _ := json.Marshal(raw)
		s.patches = append(s.patches, string(b))
		var u *string
		_ = json.Unmarshal(raw["default_url"], &u)
		s.url = u
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"data": client.CheckoutURLSettings{DefaultURL: s.url}})
}

func checkoutSettingsConfig(url string) string {
	return mockProviderConfig + `
resource "paddle_checkout_settings" "test" {
  default_url = "` + url + `"
}
`
}

func TestMockCheckoutSettings_lifecycle(t *testing.T) {
	store := &checkoutSettingsMockStore{}
	factories := newMockPaddleServer(t, store)
	name := "paddle_checkout_settings.test"

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: factories,
		CheckDestroy: func(_ *terraform.State) error {
			store.mu.Lock()
			defer store.mu.Unlock()
			if store.url != nil {
				t.Errorf("default_url = %q after destroy, want null", *store.url)
			}
			if n := len(store.patches); n == 0 || store.patches[n-1] != `{"default_url":null}` {
				t.Errorf("last PATCH = %v, want an explicit null", store.patches)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: checkoutSettingsConfig("https://example.com/buy"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "id", "checkout-url"),
					resource.TestCheckResourceAttr(name, "default_url", "https://example.com/buy"),
				),
			},
			{
				Config: checkoutSettingsConfig("https://example.com/checkout"),
				Check:  resource.TestCheckResourceAttr(name, "default_url", "https://example.com/checkout"),
			},
			{
				Config:   checkoutSettingsConfig("https://example.com/checkout"),
				PlanOnly: true,
			},
			{
				ResourceName:            name,
				ImportState:             true,
				ImportStateId:           "checkout-url",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// Cleared outside Terraform (dashboard): the next plan must propose
// re-creating it rather than silently accepting null.
func TestMockCheckoutSettings_clearedOutsideTerraformIsRecreated(t *testing.T) {
	store := &checkoutSettingsMockStore{}
	factories := newMockPaddleServer(t, store)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: checkoutSettingsConfig("https://example.com/buy")},
			{
				PreConfig: func() {
					store.mu.Lock()
					store.url = nil
					store.mu.Unlock()
				},
				Config: checkoutSettingsConfig("https://example.com/buy"),
				Check: func(_ *terraform.State) error {
					store.mu.Lock()
					defer store.mu.Unlock()
					if store.url == nil {
						t.Error("setting was not re-applied after being cleared outside Terraform")
					}
					return nil
				},
			},
		},
	})
}
