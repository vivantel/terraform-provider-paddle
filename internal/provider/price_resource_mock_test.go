package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// priceMockStore — see productMockStore's comment in
// product_resource_mock_test.go for what this is and isn't.
type priceMockStore struct {
	mu     sync.Mutex
	nextID int
	data   *client.Price
}

func (s *priceMockStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/prices":
		var p client.Price
		_ = json.NewDecoder(r.Body).Decode(&p)
		s.nextID++
		p.ID = fmt.Sprintf("pri_mock_%d", s.nextID)
		p.Status = "active"
		if p.Quantity == nil {
			p.Quantity = &client.Quantity{Minimum: 1, Maximum: 100}
		}
		s.data = &p
		_ = json.NewEncoder(w).Encode(map[string]any{"data": s.data})

	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/prices/"):
		if s.data == nil {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "not_found"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": s.data})

	case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/prices/"):
		if s.data == nil {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "not_found"}})
			return
		}
		body, _ := io.ReadAll(r.Body)
		var patch struct {
			Status *string `json:"status"`
		}
		_ = json.Unmarshal(body, &patch)
		if patch.Status != nil {
			s.data.Status = *patch.Status
		} else {
			var upd client.PriceUpdate
			_ = json.Unmarshal(body, &upd)
			s.data.Description = upd.Description
			s.data.UnitPrice = upd.UnitPrice
			s.data.Name = upd.Name
			s.data.BillingCycle = upd.BillingCycle
			s.data.Quantity = upd.Quantity
			s.data.TaxMode = upd.TaxMode
			s.data.CustomData = upd.CustomData
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": s.data})

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestMockPaddlePrice_basicLifecycle(t *testing.T) {
	store := &priceMockStore{}
	factories := newMockPaddleServer(t, store)
	resourceName := "paddle_price.test"

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: mockProviderConfig + `
resource "paddle_price" "test" {
  product_id  = "pro_mock_1"
  description = "mock price"
  unit_price = {
    amount        = "1000"
    currency_code = "USD"
  }
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "description", "mock price"),
					resource.TestCheckResourceAttr(resourceName, "unit_price.amount", "1000"),
					resource.TestCheckResourceAttr(resourceName, "status", "active"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
				),
			},
			{
				// Update: description changes in place.
				Config: mockProviderConfig + `
resource "paddle_price" "test" {
  product_id  = "pro_mock_1"
  description = "mock price renamed"
  unit_price = {
    amount        = "1000"
    currency_code = "USD"
  }
}`,
				Check: resource.TestCheckResourceAttr(resourceName, "description", "mock price renamed"),
			},
		},
	})

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.data == nil || store.data.Status != "archived" {
		t.Errorf("after destroy, mock price status = %v, want archived", store.data)
	}
}
