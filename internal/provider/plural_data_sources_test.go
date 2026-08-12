package provider

import (
	"testing"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// TestFromAPITransactionSummary and its NotificationSummary sibling below
// cover the two new list-conversion functions Step 4's plural data
// sources add — the singular fromAPISubscription/fromAPICustomer are
// already unit-tested (subscription_data_source_test.go,
// customer_data_source_test.go) and reused verbatim by
// paddle_subscriptions/paddle_customers, so no new test is needed there.
func TestFromAPITransactionSummary(t *testing.T) {
	txn := client.Transaction{
		ID:             "txn_1",
		SubscriptionID: "sub_1",
		CustomerID:     "ctm_1",
		Status:         "billed",
		Origin:         "web",
	}
	var m TransactionSummaryModel
	fromAPITransactionSummary(txn, &m)

	if m.ID.ValueString() != "txn_1" {
		t.Errorf("ID = %q, want txn_1", m.ID.ValueString())
	}
	if m.SubscriptionID.ValueString() != "sub_1" {
		t.Errorf("SubscriptionID = %q, want sub_1", m.SubscriptionID.ValueString())
	}
	if m.CustomerID.ValueString() != "ctm_1" {
		t.Errorf("CustomerID = %q, want ctm_1", m.CustomerID.ValueString())
	}
	if m.Status.ValueString() != "billed" {
		t.Errorf("Status = %q, want billed", m.Status.ValueString())
	}
	if m.Origin.ValueString() != "web" {
		t.Errorf("Origin = %q, want web", m.Origin.ValueString())
	}
}

func TestFromAPINotificationSummary(t *testing.T) {
	t.Run("delivered_at present", func(t *testing.T) {
		deliveredAt := "2026-08-12T00:00:00Z"
		n := client.Notification{
			ID:                    "ntf_1",
			NotificationSettingID: "ntfset_1",
			Status:                "delivered",
			Type:                  "product.created",
			OccurredAt:            "2026-08-11T23:59:00Z",
			DeliveredAt:           &deliveredAt,
			TimesAttempted:        1,
		}
		var m NotificationSummaryModel
		fromAPINotificationSummary(n, &m)

		if m.ID.ValueString() != "ntf_1" {
			t.Errorf("ID = %q, want ntf_1", m.ID.ValueString())
		}
		if m.DeliveredAt.IsNull() || m.DeliveredAt.ValueString() != deliveredAt {
			t.Errorf("DeliveredAt = %v, want %q", m.DeliveredAt, deliveredAt)
		}
		if m.TimesAttempted.ValueInt64() != 1 {
			t.Errorf("TimesAttempted = %d, want 1", m.TimesAttempted.ValueInt64())
		}
	})

	t.Run("no delivered_at is null", func(t *testing.T) {
		n := client.Notification{ID: "ntf_2", Status: "not_attempted"}
		var m NotificationSummaryModel
		fromAPINotificationSummary(n, &m)

		if !m.DeliveredAt.IsNull() {
			t.Errorf("DeliveredAt = %v, want null", m.DeliveredAt)
		}
	})
}
