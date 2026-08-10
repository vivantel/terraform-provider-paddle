package actions

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

func TestSameChargeItems_OrderIndependentExactMatch(t *testing.T) {
	want := []client.SubscriptionChargeItem{
		{PriceID: "pri_1", Quantity: 1},
		{PriceID: "pri_2", Quantity: 3},
	}
	got := []client.TransactionItem{
		{PriceID: "pri_2", Quantity: 3},
		{PriceID: "pri_1", Quantity: 1},
	}
	if !sameChargeItems(want, got) {
		t.Error("sameChargeItems = false, want true (same items, different order)")
	}
}

func TestSameChargeItems_DifferentQuantityDoesNotMatch(t *testing.T) {
	want := []client.SubscriptionChargeItem{{PriceID: "pri_1", Quantity: 1}}
	got := []client.TransactionItem{{PriceID: "pri_1", Quantity: 2}}
	if sameChargeItems(want, got) {
		t.Error("sameChargeItems = true, want false — quantities differ")
	}
}

func TestSameChargeItems_DifferentLengthDoesNotMatch(t *testing.T) {
	want := []client.SubscriptionChargeItem{{PriceID: "pri_1", Quantity: 1}, {PriceID: "pri_2", Quantity: 1}}
	got := []client.TransactionItem{{PriceID: "pri_1", Quantity: 1}}
	if sameChargeItems(want, got) {
		t.Error("sameChargeItems = true, want false — different item counts")
	}
}

func TestSameChargeItems_DuplicatePriceIDsMatchOneToOne(t *testing.T) {
	// Two line items with the same price_id but bought as separate items
	// (e.g. two different quantities) must match one-to-one, not let one
	// existing item satisfy both wanted items.
	want := []client.SubscriptionChargeItem{
		{PriceID: "pri_1", Quantity: 1},
		{PriceID: "pri_1", Quantity: 2},
	}
	gotOneOnly := []client.TransactionItem{{PriceID: "pri_1", Quantity: 1}}
	if sameChargeItems(want, gotOneOnly) {
		t.Error("sameChargeItems = true, want false — only one of the two wanted items is present")
	}
	gotBoth := []client.TransactionItem{{PriceID: "pri_1", Quantity: 1}, {PriceID: "pri_1", Quantity: 2}}
	if !sameChargeItems(want, gotBoth) {
		t.Error("sameChargeItems = false, want true — both wanted items present")
	}
}

func TestFindMatchingCharge_MatchesOnExactItemSet(t *testing.T) {
	existing := []client.Transaction{
		{ID: "txn_1", Items: []client.TransactionItem{{PriceID: "pri_1", Quantity: 1}}},
		{ID: "txn_2", Items: []client.TransactionItem{{PriceID: "pri_2", Quantity: 5}}},
	}
	want := []client.SubscriptionChargeItem{{PriceID: "pri_2", Quantity: 5}}
	got := findMatchingCharge(existing, want)
	if got == nil || got.ID != "txn_2" {
		t.Errorf("findMatchingCharge = %v, want txn_2", got)
	}
}

func TestFindMatchingCharge_NoMatchReturnsNil(t *testing.T) {
	existing := []client.Transaction{
		{ID: "txn_1", Items: []client.TransactionItem{{PriceID: "pri_1", Quantity: 1}}},
	}
	want := []client.SubscriptionChargeItem{{PriceID: "pri_9", Quantity: 1}}
	if got := findMatchingCharge(existing, want); got != nil {
		t.Errorf("findMatchingCharge = %v, want nil", got)
	}
}

func TestToAPISubscriptionChargeItems_ConvertsAllFields(t *testing.T) {
	items := []subscriptionChargeItemModel{
		{PriceID: types.StringValue("pri_1"), Quantity: types.Int64Value(3)},
	}
	got := toAPISubscriptionChargeItems(items)
	if len(got) != 1 || got[0].PriceID != "pri_1" || got[0].Quantity != 3 {
		t.Errorf("toAPISubscriptionChargeItems() = %+v, want [{pri_1 3}]", got)
	}
}
