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

// productMockStore is a minimal in-memory stand-in for Paddle's real
// /products endpoint — enough to drive Create/Read/Update/Delete through a
// real resource.Test lifecycle (see mockserver_test.go), not a faithful
// reimplementation of Paddle's actual validation/business logic. This is a
// faster, cheaper signal underneath TestAccPaddleProduct_basic
// (product_resource_acc_test.go, unchanged, still the real verification —
// docs/guardrails/mock-tests-supplement-not-replace-acceptance-tests.md).
type productMockStore struct {
	mu     sync.Mutex
	nextID int
	data   *client.Product
}

func (s *productMockStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/products":
		var p client.Product
		_ = json.NewDecoder(r.Body).Decode(&p)
		s.nextID++
		p.ID = fmt.Sprintf("pro_mock_%d", s.nextID)
		p.Status = "active"
		if p.Type == "" {
			p.Type = "standard"
		}
		s.data = &p
		_ = json.NewEncoder(w).Encode(map[string]any{"data": s.data})

	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/products/"):
		if s.data == nil {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "not_found"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": s.data})

	case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/products/"):
		if s.data == nil {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "not_found"}})
			return
		}
		var patch struct {
			Status *string `json:"status"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &patch)
		if patch.Status != nil {
			// The archive-on-destroy PATCH only ever sends {"status":
			// "archived"} — a real full-field update would also carry
			// name/tax_category/etc, but this test's Update step doesn't
			// need to distinguish the two bodies precisely.
			s.data.Status = *patch.Status
		} else {
			var p client.Product
			_ = json.Unmarshal(body, &p)
			p.ID = s.data.ID
			p.Status = s.data.Status
			s.data = &p
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": s.data})

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestMockPaddleProduct_basicLifecycle(t *testing.T) {
	store := &productMockStore{}
	factories := newMockPaddleServer(t, store)
	resourceName := "paddle_product.test"

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: mockProviderConfig + `
resource "paddle_product" "test" {
  name         = "Mock Widget"
  tax_category = "standard"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", "Mock Widget"),
					resource.TestCheckResourceAttr(resourceName, "status", "active"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
				),
			},
			{
				// Update: name changes in place.
				Config: mockProviderConfig + `
resource "paddle_product" "test" {
  name         = "Mock Widget Renamed"
  tax_category = "standard"
}`,
				Check: resource.TestCheckResourceAttr(resourceName, "name", "Mock Widget Renamed"),
			},
		},
	})

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.data == nil || store.data.Status != "archived" {
		t.Errorf("after destroy, mock product status = %v, want archived", store.data)
	}
}
