package provider

import (
	"context"
	"testing"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

func TestFromAPINotification(t *testing.T) {
	deliveredAt := "2026-08-01T00:00:05Z"
	n := client.Notification{
		ID:                    "ntf_01abc",
		Type:                  "product.created",
		Status:                "delivered",
		NotificationSettingID: "ntfset_01abc",
		OccurredAt:            "2026-08-01T00:00:00Z",
		DeliveredAt:           &deliveredAt,
		TimesAttempted:        1,
	}
	logs := []client.NotificationLog{
		{ID: "ntflog_01abc", ResponseCode: 200, ResponseContentType: "text/plain", ResponseBody: "", AttemptedAt: "2026-08-01T00:00:05Z"},
	}
	var m NotificationDataSourceModel
	fromAPINotification(context.Background(), n, logs, &m)

	if m.ID.ValueString() != "ntf_01abc" {
		t.Errorf("ID = %q, want ntf_01abc", m.ID.ValueString())
	}
	if m.Status.ValueString() != "delivered" {
		t.Errorf("Status = %q, want delivered", m.Status.ValueString())
	}
	if m.NotificationSettingID.ValueString() != "ntfset_01abc" {
		t.Errorf("NotificationSettingID = %q, want ntfset_01abc", m.NotificationSettingID.ValueString())
	}
	if m.DeliveredAt.ValueString() != "2026-08-01T00:00:05Z" {
		t.Errorf("DeliveredAt = %q, want 2026-08-01T00:00:05Z", m.DeliveredAt.ValueString())
	}
	if m.TimesAttempted.ValueInt64() != 1 {
		t.Errorf("TimesAttempted = %d, want 1", m.TimesAttempted.ValueInt64())
	}
	if len(m.Logs) != 1 {
		t.Fatalf("len(Logs) = %d, want 1", len(m.Logs))
	}
	if m.Logs[0].ResponseCode.ValueInt64() != 200 {
		t.Errorf("Logs[0].ResponseCode = %d, want 200", m.Logs[0].ResponseCode.ValueInt64())
	}
}

// TestFromAPINotification_NoDeliveredAtIsNull confirms a not-yet-delivered
// notification produces a null delivered_at, not an empty string — same
// convention fix as TestFromAPISubscription_NoNextBilledAtIsNull, found
// via code review.
func TestFromAPINotification_NoDeliveredAtIsNull(t *testing.T) {
	n := client.Notification{ID: "ntf_01abc", Status: "not_attempted"}
	var m NotificationDataSourceModel
	fromAPINotification(context.Background(), n, nil, &m)
	if !m.DeliveredAt.IsNull() {
		t.Errorf("DeliveredAt = %v, want null when Paddle hasn't delivered yet", m.DeliveredAt)
	}
}
