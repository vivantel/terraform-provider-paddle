package actions

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

func TestFindMatchingAdjustment_MatchesOnActionAndReason(t *testing.T) {
	existing := []client.Adjustment{
		{ID: "adj_1", Action: "credit", Reason: "goodwill", Type: "full"},
		{ID: "adj_2", Action: "refund", Reason: "customer requested refund", Type: "partial"},
	}
	got := findMatchingAdjustment(existing, "refund", "customer requested refund", "")
	if got == nil {
		t.Fatal("findMatchingAdjustment: got nil, want a match on adj_2")
	}
	if got.ID != "adj_2" {
		t.Errorf("ID = %q, want adj_2", got.ID)
	}
}

func TestFindMatchingAdjustment_NoMatchReturnsNil(t *testing.T) {
	existing := []client.Adjustment{
		{ID: "adj_1", Action: "credit", Reason: "goodwill"},
	}
	if got := findMatchingAdjustment(existing, "refund", "customer requested refund", ""); got != nil {
		t.Errorf("findMatchingAdjustment: got %+v, want nil", got)
	}
}

func TestFindMatchingAdjustment_ActionAloneIsNotEnough(t *testing.T) {
	// Same action, different reason — must not match. A refund for "item
	// defective" and a refund for "duplicate charge" against the same
	// transaction are two different real-world refunds, not a retry of
	// one another.
	existing := []client.Adjustment{
		{ID: "adj_1", Action: "refund", Reason: "item defective"},
	}
	if got := findMatchingAdjustment(existing, "refund", "duplicate charge", ""); got != nil {
		t.Errorf("findMatchingAdjustment: got %+v, want nil (different reason)", got)
	}
}

func TestFindMatchingAdjustment_TypeNarrowsWhenSpecified(t *testing.T) {
	existing := []client.Adjustment{
		{ID: "adj_1", Action: "refund", Reason: "duplicate charge", Type: "full"},
	}
	if got := findMatchingAdjustment(existing, "refund", "duplicate charge", "partial"); got != nil {
		t.Errorf("findMatchingAdjustment: got %+v, want nil — existing is type=full, wanted type=partial", got)
	}
	if got := findMatchingAdjustment(existing, "refund", "duplicate charge", "full"); got == nil {
		t.Error("findMatchingAdjustment: got nil, want a match when type matches too")
	}
}

func TestFindMatchingAdjustment_EmptyWantTypeMatchesAnyExistingType(t *testing.T) {
	// wantType == "" means the action's config left `type` unset (Paddle
	// defaults it server-side) — don't require an exact type match in
	// that case, or every un-typed invocation would fail to recognize its
	// own prior attempt.
	existing := []client.Adjustment{
		{ID: "adj_1", Action: "refund", Reason: "duplicate charge", Type: "partial"},
	}
	if got := findMatchingAdjustment(existing, "refund", "duplicate charge", ""); got == nil {
		t.Error("findMatchingAdjustment: got nil, want a match regardless of the existing adjustment's type")
	}
}

func TestToAPIAdjustment_OmitsUnsetOptionalFields(t *testing.T) {
	m := adjustmentActionModel{
		Action:        types.StringValue("refund"),
		TransactionID: types.StringValue("txn_1"),
		Reason:        types.StringValue("duplicate charge"),
		Type:          types.StringNull(),
		TaxMode:       types.StringNull(),
	}
	got := toAPIAdjustment(m)
	if got.Type != "" || got.TaxMode != "" {
		t.Errorf("toAPIAdjustment() = %+v, want Type and TaxMode both empty when unset in config", got)
	}
	if len(got.Items) != 0 {
		t.Errorf("Items = %v, want empty when config has none", got.Items)
	}
}

func TestToAPIAdjustment_ItemsRoundTripAmount(t *testing.T) {
	m := adjustmentActionModel{
		Action:        types.StringValue("refund"),
		TransactionID: types.StringValue("txn_1"),
		Reason:        types.StringValue("duplicate charge"),
		Type:          types.StringValue("partial"),
		Items: []adjustmentItemModel{
			{ItemID: types.StringValue("txnitm_1"), Type: types.StringValue("partial"), Amount: types.StringValue("500")},
			{ItemID: types.StringValue("txnitm_2"), Type: types.StringValue("full"), Amount: types.StringNull()},
		},
	}
	got := toAPIAdjustment(m)
	if len(got.Items) != 2 {
		t.Fatalf("len(Items) = %d, want 2", len(got.Items))
	}
	if got.Items[0].Amount == nil || *got.Items[0].Amount != "500" {
		t.Errorf("Items[0].Amount = %v, want \"500\"", got.Items[0].Amount)
	}
	if got.Items[1].Amount != nil {
		t.Errorf("Items[1].Amount = %v, want nil (unset in config)", *got.Items[1].Amount)
	}
}
