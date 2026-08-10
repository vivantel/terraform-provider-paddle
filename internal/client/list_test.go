package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestListProducts_FollowsHasMoreCursor confirms ListProducts pages through
// Paddle's cursor-based pagination (after=<last id of the previous page>,
// stopping once meta.pagination.has_more is false) rather than assuming a
// single page — sweepers must see every matching object, not just the
// first `per_page` of them.
func TestListProducts_FollowsHasMoreCursor(t *testing.T) {
	var afterValues []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		afterValues = append(afterValues, r.URL.Query().Get("after"))
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("after") == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []Product{{ID: "pro_1", Name: "Acc Test A", TaxCategory: "standard"}},
				"meta": map[string]any{"pagination": map[string]any{"has_more": true}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []Product{{ID: "pro_2", Name: "Acc Test B", TaxCategory: "standard"}},
			"meta": map[string]any{"pagination": map[string]any{"has_more": false}},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	products, err := c.ListProducts(context.Background())
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	if len(products) != 2 {
		t.Fatalf("len(products) = %d, want 2", len(products))
	}
	if products[0].ID != "pro_1" || products[1].ID != "pro_2" {
		t.Errorf("products = %+v, want [pro_1, pro_2] in order", products)
	}
	if len(afterValues) != 2 || afterValues[0] != "" || afterValues[1] != "pro_1" {
		t.Errorf("after cursor values = %v, want [\"\", \"pro_1\"]", afterValues)
	}
}

func TestListPrices_StopsWhenHasMoreFalse(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []Price{{ID: "pri_1", ProductID: "pro_1", Description: "Acc Test Price"}},
			"meta": map[string]any{"pagination": map[string]any{"has_more": false}},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	prices, err := c.ListPrices(context.Background())
	if err != nil {
		t.Fatalf("ListPrices: %v", err)
	}
	if len(prices) != 1 {
		t.Fatalf("len(prices) = %d, want 1", len(prices))
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (has_more=false must stop pagination)", calls)
	}
}

func TestListDiscounts_EmptyPageStopsWithoutInfiniteLoop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// A pathological has_more=true with zero items must not spin forever
		// — the empty page itself is the stop condition, independent of what
		// has_more claims.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []Discount{},
			"meta": map[string]any{"pagination": map[string]any{"has_more": true}},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	discounts, err := c.ListDiscounts(context.Background())
	if err != nil {
		t.Fatalf("ListDiscounts: %v", err)
	}
	if len(discounts) != 0 {
		t.Errorf("len(discounts) = %d, want 0", len(discounts))
	}
}

func TestListAdjustments_FiltersByTransactionIDAndFollowsHasMoreCursor(t *testing.T) {
	var transactionIDValues []string
	var afterValues []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		transactionIDValues = append(transactionIDValues, r.URL.Query().Get("transaction_id"))
		afterValues = append(afterValues, r.URL.Query().Get("after"))
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("after") == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []Adjustment{{ID: "adj_1", Action: "refund", TransactionID: "txn_1", Reason: "r"}},
				"meta": map[string]any{"pagination": map[string]any{"has_more": true}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []Adjustment{{ID: "adj_2", Action: "refund", TransactionID: "txn_1", Reason: "r"}},
			"meta": map[string]any{"pagination": map[string]any{"has_more": false}},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	adjustments, err := c.ListAdjustments(context.Background(), "txn_1")
	if err != nil {
		t.Fatalf("ListAdjustments: %v", err)
	}
	if len(adjustments) != 2 {
		t.Fatalf("len(adjustments) = %d, want 2", len(adjustments))
	}
	for _, v := range transactionIDValues {
		if v != "txn_1" {
			t.Errorf("transaction_id query param = %q, want %q on every page request", v, "txn_1")
		}
	}
	if len(afterValues) != 2 || afterValues[0] != "" || afterValues[1] != "adj_1" {
		t.Errorf("after cursor values = %v, want [\"\", \"adj_1\"]", afterValues)
	}
}

func TestListSubscriptionChargeTransactions_FiltersByOriginAndSubscriptionID(t *testing.T) {
	var originValues, subIDValues []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originValues = append(originValues, r.URL.Query().Get("origin"))
		subIDValues = append(subIDValues, r.URL.Query().Get("subscription_id"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []Transaction{{ID: "txn_1", SubscriptionID: "sub_1", Origin: "subscription_charge"}},
			"meta": map[string]any{"pagination": map[string]any{"has_more": false}},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	txns, err := c.ListSubscriptionChargeTransactions(context.Background(), "sub_1")
	if err != nil {
		t.Fatalf("ListSubscriptionChargeTransactions: %v", err)
	}
	if len(txns) != 1 {
		t.Fatalf("len(txns) = %d, want 1", len(txns))
	}
	if originValues[0] != "subscription_charge" {
		t.Errorf("origin query param = %q, want %q", originValues[0], "subscription_charge")
	}
	if subIDValues[0] != "sub_1" {
		t.Errorf("subscription_id query param = %q, want %q", subIDValues[0], "sub_1")
	}
}
