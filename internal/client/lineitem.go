package client

// Transaction-adjacent API responses model "an item" three different,
// non-interchangeable ways — found the hard way across two of
// v0.4.0-beta.1's three real bugs (see TransactionItem's,
// TransactionLineItem's, and NextTransactionPreview's own doc comments
// in client.go for each shape's full history):
//
//  1. Transaction.Items[].Price.ID — nested under price.id, has Quantity,
//     no item_id of its own.
//  2. Transaction.Details.LineItems[].ID — a separate, fully-calculated
//     breakdown; this is the only shape carrying the txnitm_... ID
//     create-adjustment's item_id parameter actually needs.
//  3. NextTransactionPreview.Items[].PriceID — flat, no ID field at all
//     (a preview has nothing billed yet, so no txnitm_... exists to
//     return) — deliberately not given a resolver function below; there
//     is no item_id to resolve for this shape, only price/quantity
//     matching (see findMatchingScheduledCharge in
//     internal/provider/actions/action_paddle_subscription_charge.go for
//     that subset-containment check).
//
// This file is the one place that knows all three shapes, so every
// future caller needing an item_id has a single call site instead of a
// fourth copy-pasted Details.LineItems traversal.

// LineItemIDs returns every item_id (txnitm_...) from txn's
// Details.LineItems — the set create-adjustment's Items must list
// explicitly even for a full adjustment (Paddle rejects "type: full"
// with no items array at all — see AdjustmentItem's comment). Nil-safe:
// returns nil if txn or txn.Details is nil, rather than requiring every
// caller to repeat that check.
func LineItemIDs(txn *Transaction) []string {
	if txn == nil || txn.Details == nil {
		return nil
	}
	ids := make([]string, 0, len(txn.Details.LineItems))
	for _, li := range txn.Details.LineItems {
		ids = append(ids, li.ID)
	}
	return ids
}

// ResolvedLineItem is the one clean shape a caller needing item_id,
// price_id, and quantity together should use — as opposed to picking
// between Transaction.Items (price.id + quantity, no item_id) and
// Transaction.Details.LineItems (id + price_id + quantity) itself.
// paddle_transaction_data_source.go (docs/plans/paddle-provider-v4.md
// Step 3) is this type's first consumer.
type ResolvedLineItem struct {
	ItemID   string
	PriceID  string
	Quantity int64
}

// ResolveLineItems returns every line item on txn.Details.LineItems as
// ResolvedLineItem — nil-safe, same as LineItemIDs above.
func ResolveLineItems(txn *Transaction) []ResolvedLineItem {
	if txn == nil || txn.Details == nil {
		return nil
	}
	out := make([]ResolvedLineItem, 0, len(txn.Details.LineItems))
	for _, li := range txn.Details.LineItems {
		out = append(out, ResolvedLineItem{ItemID: li.ID, PriceID: li.PriceID, Quantity: li.Quantity})
	}
	return out
}

// ResolveLineItemID finds the item_id (txnitm_...) on txn.Details.LineItems
// for the line item matching priceID — reconciling the price-keyed shape
// (Transaction.Items, or a price ID a caller already has from elsewhere,
// e.g. paddle_price's own resource ID) with the id-keyed shape
// (Transaction.Details.LineItems) create-adjustment's item_id actually
// needs. Returns ok=false if zero or more than one line item matches
// priceID — quantity isn't enough to disambiguate two line items on the
// same price, and Paddle's API doesn't return anything else that would;
// an ambiguous match is reported as "not found" rather than guessing.
func ResolveLineItemID(txn *Transaction, priceID string) (itemID string, ok bool) {
	if txn == nil || txn.Details == nil {
		return "", false
	}
	var match string
	count := 0
	for _, li := range txn.Details.LineItems {
		if li.PriceID == priceID {
			match = li.ID
			count++
		}
	}
	if count != 1 {
		return "", false
	}
	return match, true
}
