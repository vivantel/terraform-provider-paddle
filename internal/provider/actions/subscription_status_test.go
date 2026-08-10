package actions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

func newTestSubscriptionServer(t *testing.T, status string) *client.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"id": "sub_1", "status": status},
		})
	}))
	t.Cleanup(srv.Close)
	return client.New(srv.URL, "test-key")
}

func TestCheckAlreadyInTargetState_MatchReturnsTrue(t *testing.T) {
	c := newTestSubscriptionServer(t, "canceled")
	alreadyDone, status, err := checkAlreadyInTargetState(context.Background(), c, "sub_1", "canceled")
	if err != nil {
		t.Fatalf("checkAlreadyInTargetState: %v", err)
	}
	if !alreadyDone {
		t.Error("alreadyDone = false, want true")
	}
	if status != "canceled" {
		t.Errorf("status = %q, want %q", status, "canceled")
	}
}

func TestCheckAlreadyInTargetState_MismatchReturnsFalse(t *testing.T) {
	// The exact scenario docs/guardrails/money-moving-actions-no-blanket-retry.md
	// calls out: a canceled subscription is not "paused", but resume's
	// target state is "active" — canceled must not be conflated with
	// either the source state or the target state.
	c := newTestSubscriptionServer(t, "canceled")
	alreadyDone, status, err := checkAlreadyInTargetState(context.Background(), c, "sub_1", "active")
	if err != nil {
		t.Fatalf("checkAlreadyInTargetState: %v", err)
	}
	if alreadyDone {
		t.Error("alreadyDone = true, want false — a canceled subscription is not already active")
	}
	if status != "canceled" {
		t.Errorf("status = %q, want %q", status, "canceled")
	}
}
