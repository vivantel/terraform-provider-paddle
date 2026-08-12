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

// notificationSettingMockStore — see productMockStore's comment in
// product_resource_mock_test.go for what this is and isn't. Unlike the
// other four resources, this entity has a real hard DELETE (no archive
// pattern) and its create/update request shape for subscribed_events
// (plain string array) genuinely differs from its response shape (event
// objects) — see client.go's comment on NotificationSetting for why.
type notificationSettingMockStore struct {
	mu      sync.Mutex
	nextID  int
	data    *client.NotificationSetting
	deleted bool
}

func toEvents(names []string) []client.NotificationSettingEvent {
	events := make([]client.NotificationSettingEvent, len(names))
	for i, n := range names {
		events[i] = client.NotificationSettingEvent{Name: n}
	}
	return events
}

func (s *notificationSettingMockStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/notification-settings":
		var create client.NotificationSettingCreate
		_ = json.NewDecoder(r.Body).Decode(&create)
		s.nextID++
		s.data = &client.NotificationSetting{
			ID:                fmt.Sprintf("ntfset_mock_%d", s.nextID),
			Description:       create.Description,
			Type:              create.Type,
			Destination:       create.Destination,
			Active:            true, // Create never accepts active — always true, matching the real API.
			SubscribedEvents:  toEvents(create.SubscribedEvents),
			EndpointSecretKey: "mock-secret-key",
		}
		s.deleted = false
		_ = json.NewEncoder(w).Encode(map[string]any{"data": s.data})

	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/notification-settings/"):
		if s.data == nil || s.deleted {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "not_found"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": s.data})

	case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/notification-settings/"):
		if s.data == nil || s.deleted {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "not_found"}})
			return
		}
		var upd client.NotificationSettingUpdate
		_ = json.NewDecoder(r.Body).Decode(&upd)
		s.data.Description = upd.Description
		s.data.Destination = upd.Destination
		s.data.SubscribedEvents = toEvents(upd.SubscribedEvents)
		if upd.Active != nil {
			s.data.Active = *upd.Active
		}
		if upd.APIVersion != nil {
			s.data.APIVersion = *upd.APIVersion
		}
		if upd.IncludeSensitiveFields != nil {
			s.data.IncludeSensitiveFields = *upd.IncludeSensitiveFields
		}
		if upd.TrafficSource != "" {
			s.data.TrafficSource = upd.TrafficSource
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": s.data})

	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/notification-settings/"):
		if s.data == nil || s.deleted {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "not_found"}})
			return
		}
		s.deleted = true
		w.WriteHeader(http.StatusNoContent)

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestMockPaddleNotificationSetting_basicLifecycle(t *testing.T) {
	store := &notificationSettingMockStore{}
	factories := newMockPaddleServer(t, store)
	resourceName := "paddle_notification_setting.test"

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: mockProviderConfig + `
resource "paddle_notification_setting" "test" {
  description        = "mock notification setting"
  type                = "url"
  destination         = "https://example.com/webhook"
  subscribed_events   = ["transaction.billed"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "description", "mock notification setting"),
					resource.TestCheckResourceAttr(resourceName, "active", "true"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "endpoint_secret_key"),
				),
			},
			{
				// Update: description changes in place, active flips false
				// (exercises the update-body active-carrying path, not the
				// Create-time follow-up-update path a false-at-create would).
				Config: mockProviderConfig + `
resource "paddle_notification_setting" "test" {
  description        = "mock notification setting renamed"
  type                = "url"
  destination         = "https://example.com/webhook"
  subscribed_events   = ["transaction.billed"]
  active              = false
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "description", "mock notification setting renamed"),
					resource.TestCheckResourceAttr(resourceName, "active", "false"),
				),
			},
		},
	})

	store.mu.Lock()
	defer store.mu.Unlock()
	if !store.deleted {
		t.Error("after destroy, mock notification setting was not deleted — this entity has a real hard DELETE, no archive fallback")
	}
}
