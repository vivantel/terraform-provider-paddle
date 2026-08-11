package provider

import (
	"testing"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

func TestFromAPISubscription(t *testing.T) {
	nextBilledAt := "2026-09-01T00:00:00Z"
	sub := client.Subscription{
		ID:           "sub_01abc",
		Status:       "active",
		CustomerID:   "ctm_01abc",
		CurrencyCode: "USD",
		NextBilledAt: &nextBilledAt,
		CreatedAt:    "2026-08-01T00:00:00Z",
		UpdatedAt:    "2026-08-05T00:00:00Z",
	}
	var m SubscriptionDataSourceModel
	fromAPISubscription(sub, &m)

	if m.ID.ValueString() != "sub_01abc" {
		t.Errorf("ID = %q, want sub_01abc", m.ID.ValueString())
	}
	if m.Status.ValueString() != "active" {
		t.Errorf("Status = %q, want active", m.Status.ValueString())
	}
	if m.CustomerID.ValueString() != "ctm_01abc" {
		t.Errorf("CustomerID = %q, want ctm_01abc", m.CustomerID.ValueString())
	}
	if m.CurrencyCode.ValueString() != "USD" {
		t.Errorf("CurrencyCode = %q, want USD", m.CurrencyCode.ValueString())
	}
	if m.NextBilledAt.ValueString() != "2026-09-01T00:00:00Z" {
		t.Errorf("NextBilledAt = %q, want 2026-09-01T00:00:00Z", m.NextBilledAt.ValueString())
	}
	if m.CreatedAt.ValueString() != "2026-08-01T00:00:00Z" {
		t.Errorf("CreatedAt = %q, want 2026-08-01T00:00:00Z", m.CreatedAt.ValueString())
	}
	if m.UpdatedAt.ValueString() != "2026-08-05T00:00:00Z" {
		t.Errorf("UpdatedAt = %q, want 2026-08-05T00:00:00Z", m.UpdatedAt.ValueString())
	}
}

// TestFromAPISubscription_NoNextBilledAtIsNull confirms a canceled/paused
// subscription with nothing scheduled produces a null next_billed_at, not
// an empty string indistinguishable from "" being a real value — found
// via code review: this field must follow this codebase's own established
// convention for genuinely-optional API fields (see e.g. discount_resource.go's
// Code/CurrencyCode/ExpiresAt/DiscountGroupID, all *string -> StringNull()
// when absent), which this field didn't originally follow.
func TestFromAPISubscription_NoNextBilledAtIsNull(t *testing.T) {
	sub := client.Subscription{ID: "sub_01abc", Status: "canceled"}
	var m SubscriptionDataSourceModel
	fromAPISubscription(sub, &m)
	if !m.NextBilledAt.IsNull() {
		t.Errorf("NextBilledAt = %v, want null when Paddle omits next_billed_at", m.NextBilledAt)
	}
}
