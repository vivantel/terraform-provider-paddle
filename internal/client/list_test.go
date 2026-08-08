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
