package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestReplayNotification_PostsToReplayEndpointAndDecodesNewNotification
// confirms the request/response shape docs/facts/0007-replay-endpoint-and-timeouts-module-confirmed.md
// documents: a plain POST to /notifications/{id}/replay with no body,
// decoding the response's data into a *new* Notification (a different ID
// than the one replayed — the endpoint doesn't mutate or echo back the
// original record).
func TestReplayNotification_PostsToReplayEndpointAndDecodesNewNotification(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"ntf_new","notification_setting_id":"ntfset_1","status":"not_attempted","type":"product.created","occurred_at":"2026-08-12T00:00:00Z"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	replayed, err := c.ReplayNotification(context.Background(), "ntf_original")
	if err != nil {
		t.Fatalf("ReplayNotification: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/notifications/ntf_original/replay" {
		t.Errorf("path = %q, want /notifications/ntf_original/replay", gotPath)
	}
	if replayed.ID != "ntf_new" {
		t.Errorf("replayed.ID = %q, want ntf_new (a new notification entity, not the original ntf_original)", replayed.ID)
	}
	if replayed.NotificationSettingID != "ntfset_1" {
		t.Errorf("replayed.NotificationSettingID = %q, want ntfset_1", replayed.NotificationSettingID)
	}
}
