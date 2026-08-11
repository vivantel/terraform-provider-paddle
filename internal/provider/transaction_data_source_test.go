package provider

import (
	"testing"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

func TestFromAPITransaction(t *testing.T) {
	txn := client.Transaction{
		ID:             "txn_01abc",
		SubscriptionID: "sub_01abc",
		CustomerID:     "ctm_01abc",
		Status:         "billed",
		Origin:         "subscription_charge",
		Details: &client.TransactionDetails{
			LineItems: []client.TransactionLineItem{
				{ID: "txnitm_1", PriceID: "pri_1", Quantity: 2},
			},
		},
	}
	var m TransactionDataSourceModel
	fromAPITransaction(txn, &m)

	if m.ID.ValueString() != "txn_01abc" {
		t.Errorf("ID = %q, want txn_01abc", m.ID.ValueString())
	}
	if m.SubscriptionID.ValueString() != "sub_01abc" {
		t.Errorf("SubscriptionID = %q, want sub_01abc", m.SubscriptionID.ValueString())
	}
	if m.CustomerID.ValueString() != "ctm_01abc" {
		t.Errorf("CustomerID = %q, want ctm_01abc", m.CustomerID.ValueString())
	}
	if m.Status.ValueString() != "billed" {
		t.Errorf("Status = %q, want billed", m.Status.ValueString())
	}
	if m.Origin.ValueString() != "subscription_charge" {
		t.Errorf("Origin = %q, want subscription_charge", m.Origin.ValueString())
	}
	if len(m.LineItems) != 1 {
		t.Fatalf("len(LineItems) = %d, want 1", len(m.LineItems))
	}
	li := m.LineItems[0]
	if li.ItemID.ValueString() != "txnitm_1" || li.PriceID.ValueString() != "pri_1" || li.Quantity.ValueInt64() != 2 {
		t.Errorf("LineItems[0] = %+v, want {ItemID:txnitm_1 PriceID:pri_1 Quantity:2}", li)
	}
}

func TestFromAPITransaction_NoDetailsIsEmptyLineItems(t *testing.T) {
	txn := client.Transaction{ID: "txn_01abc", Status: "draft"}
	var m TransactionDataSourceModel
	fromAPITransaction(txn, &m)
	if len(m.LineItems) != 0 {
		t.Errorf("len(LineItems) = %d, want 0 when Details is absent", len(m.LineItems))
	}
}
