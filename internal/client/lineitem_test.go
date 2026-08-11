package client

import "testing"

// Fixtures below mirror the real-shaped JSON bodies already captured in
// client_test.go's TestTransactionJSON_* tests — not invented shapes.

func TestLineItemIDs(t *testing.T) {
	txn := &Transaction{
		ID: "txn_01abc",
		Details: &TransactionDetails{
			LineItems: []TransactionLineItem{
				{ID: "txnitm_1", PriceID: "pri_1", Quantity: 1},
				{ID: "txnitm_2", PriceID: "pri_2", Quantity: 2},
			},
		},
	}
	got := LineItemIDs(txn)
	want := []string{"txnitm_1", "txnitm_2"}
	if len(got) != len(want) {
		t.Fatalf("LineItemIDs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("LineItemIDs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLineItemIDs_NilTransaction(t *testing.T) {
	if got := LineItemIDs(nil); got != nil {
		t.Errorf("LineItemIDs(nil) = %v, want nil", got)
	}
}

func TestLineItemIDs_NilDetails(t *testing.T) {
	txn := &Transaction{ID: "txn_01abc"}
	if got := LineItemIDs(txn); got != nil {
		t.Errorf("LineItemIDs = %v, want nil when Details is nil", got)
	}
}

func TestResolveLineItems(t *testing.T) {
	txn := &Transaction{
		Details: &TransactionDetails{
			LineItems: []TransactionLineItem{
				{ID: "txnitm_1", PriceID: "pri_1", Quantity: 1},
				{ID: "txnitm_2", PriceID: "pri_2", Quantity: 2},
			},
		},
	}
	got := ResolveLineItems(txn)
	want := []ResolvedLineItem{
		{ItemID: "txnitm_1", PriceID: "pri_1", Quantity: 1},
		{ItemID: "txnitm_2", PriceID: "pri_2", Quantity: 2},
	}
	if len(got) != len(want) {
		t.Fatalf("ResolveLineItems = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ResolveLineItems[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestResolveLineItems_NilTransaction(t *testing.T) {
	if got := ResolveLineItems(nil); got != nil {
		t.Errorf("ResolveLineItems(nil) = %v, want nil", got)
	}
}

func TestResolveLineItems_NilDetails(t *testing.T) {
	txn := &Transaction{ID: "txn_01abc"}
	if got := ResolveLineItems(txn); got != nil {
		t.Errorf("ResolveLineItems = %v, want nil when Details is nil", got)
	}
}

func TestResolveLineItemID_Match(t *testing.T) {
	txn := &Transaction{
		Details: &TransactionDetails{
			LineItems: []TransactionLineItem{
				{ID: "txnitm_1", PriceID: "pri_1", Quantity: 1},
				{ID: "txnitm_2", PriceID: "pri_2", Quantity: 2},
			},
		},
	}
	id, ok := ResolveLineItemID(txn, "pri_2")
	if !ok || id != "txnitm_2" {
		t.Errorf("ResolveLineItemID(pri_2) = (%q, %v), want (txnitm_2, true)", id, ok)
	}
}

func TestResolveLineItemID_NoMatch(t *testing.T) {
	txn := &Transaction{
		Details: &TransactionDetails{
			LineItems: []TransactionLineItem{
				{ID: "txnitm_1", PriceID: "pri_1", Quantity: 1},
			},
		},
	}
	if _, ok := ResolveLineItemID(txn, "pri_nonexistent"); ok {
		t.Error("ResolveLineItemID matched a price with no line item, want ok=false")
	}
}

func TestResolveLineItemID_AmbiguousMatch(t *testing.T) {
	// Two line items on the same price — quantity alone can't
	// disambiguate which txnitm_... a caller meant, so this must report
	// "not found" rather than guessing at either one.
	txn := &Transaction{
		Details: &TransactionDetails{
			LineItems: []TransactionLineItem{
				{ID: "txnitm_1", PriceID: "pri_1", Quantity: 1},
				{ID: "txnitm_2", PriceID: "pri_1", Quantity: 1},
			},
		},
	}
	if id, ok := ResolveLineItemID(txn, "pri_1"); ok {
		t.Errorf("ResolveLineItemID matched an ambiguous price, want ok=false, got (%q, true)", id)
	}
}

func TestResolveLineItemID_NilTransaction(t *testing.T) {
	if _, ok := ResolveLineItemID(nil, "pri_1"); ok {
		t.Error("ResolveLineItemID(nil, ...) = ok=true, want false")
	}
}

func TestResolveLineItemID_NilDetails(t *testing.T) {
	txn := &Transaction{ID: "txn_01abc"}
	if _, ok := ResolveLineItemID(txn, "pri_1"); ok {
		t.Error("ResolveLineItemID with nil Details = ok=true, want false")
	}
}
