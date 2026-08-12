package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// discountGroupMockStore — see productMockStore's comment in
// product_resource_mock_test.go for what this is and isn't.
type discountGroupMockStore struct {
	mu     sync.Mutex
	nextID int
	data   *client.DiscountGroup
}

func (s *discountGroupMockStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/discount-groups":
		var g client.DiscountGroup
		_ = json.NewDecoder(r.Body).Decode(&g)
		s.nextID++
		g.ID = fmt.Sprintf("dsg_mock_%d", s.nextID)
		g.Status = "active"
		s.data = &g
		_ = json.NewEncoder(w).Encode(map[string]any{"data": s.data})

	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/discount-groups/"):
		if s.data == nil {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "not_found"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": s.data})

	case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/discount-groups/"):
		if s.data == nil {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "not_found"}})
			return
		}
		var g client.DiscountGroup
		_ = json.NewDecoder(r.Body).Decode(&g)
		if g.Status != "" {
			// Archive-on-destroy PATCH: {"status": "archived"}, name
			// omitted (zero value) — don't let it clobber the real name.
			s.data.Status = g.Status
		} else {
			g.ID = s.data.ID
			g.Status = s.data.Status
			s.data = &g
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": s.data})

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestMockPaddleDiscountGroup_basicLifecycle(t *testing.T) {
	store := &discountGroupMockStore{}
	factories := newMockPaddleServer(t, store)
	resourceName := "paddle_discount_group.test"

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: mockProviderConfig + `
resource "paddle_discount_group" "test" {
  name = "Mock VIP Customers"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", "Mock VIP Customers"),
					resource.TestCheckResourceAttr(resourceName, "status", "active"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
				),
			},
			{
				// Update: name changes in place.
				Config: mockProviderConfig + `
resource "paddle_discount_group" "test" {
  name = "Mock VIP Customers Renamed"
}`,
				Check: resource.TestCheckResourceAttr(resourceName, "name", "Mock VIP Customers Renamed"),
			},
		},
	})

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.data == nil || store.data.Status != "archived" {
		t.Errorf("after destroy, mock discount group status = %v, want archived", store.data)
	}
}
