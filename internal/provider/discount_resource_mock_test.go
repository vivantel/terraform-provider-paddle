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

// discountMockStore — see productMockStore's comment in
// product_resource_mock_test.go for what this is and isn't.
type discountMockStore struct {
	mu     sync.Mutex
	nextID int
	data   *client.Discount
}

func (s *discountMockStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/discounts":
		var d client.Discount
		_ = json.NewDecoder(r.Body).Decode(&d)
		s.nextID++
		d.ID = fmt.Sprintf("dsc_mock_%d", s.nextID)
		d.Status = "active"
		if d.Mode == "" {
			d.Mode = "standard"
		}
		if d.Code == nil {
			v := "MOCKCODE"
			d.Code = &v
		}
		s.data = &d
		_ = json.NewEncoder(w).Encode(map[string]any{"data": s.data})

	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/discounts/"):
		if s.data == nil {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "not_found"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": s.data})

	case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/discounts/"):
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
		if patch.Status != nil && len(body) < 40 {
			// A pure archive-on-destroy PATCH is exactly {"status":
			// "archived"} — short enough to distinguish from a real full
			// update body (which also happens to carry status via
			// fromAPIDiscount, but along with every other field too).
			s.data.Status = *patch.Status
		} else {
			var d client.Discount
			_ = json.Unmarshal(body, &d)
			// A real update request body never carries status/times_used/
			// created_at/updated_at (toAPIDiscount doesn't send them) —
			// preserve what the server already has for those, the same way
			// Paddle's real API would, rather than losing them to zero
			// values from an unmarshal of a request body that never set
			// them.
			d.ID = s.data.ID
			d.Status = s.data.Status
			d.TimesUsed = s.data.TimesUsed
			d.CreatedAt = s.data.CreatedAt
			d.UpdatedAt = s.data.UpdatedAt
			s.data = &d
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": s.data})

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestMockPaddleDiscount_basicLifecycle(t *testing.T) {
	store := &discountMockStore{}
	factories := newMockPaddleServer(t, store)
	resourceName := "paddle_discount.test"

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: mockProviderConfig + `
resource "paddle_discount" "test" {
  description = "mock discount"
  type        = "percentage"
  amount      = "10"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "description", "mock discount"),
					resource.TestCheckResourceAttr(resourceName, "status", "active"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
				),
			},
			{
				// Update: description changes in place.
				Config: mockProviderConfig + `
resource "paddle_discount" "test" {
  description = "mock discount renamed"
  type        = "percentage"
  amount      = "10"
}`,
				Check: resource.TestCheckResourceAttr(resourceName, "description", "mock discount renamed"),
			},
		},
	})

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.data == nil || store.data.Status != "archived" {
		t.Errorf("after destroy, mock discount status = %v, want archived", store.data)
	}
}
