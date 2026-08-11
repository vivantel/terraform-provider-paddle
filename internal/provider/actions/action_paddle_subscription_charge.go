package actions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// chargeSearchRetryAttempts/chargeSearchRetryDelay: read-after-write lag
// mitigation. Found running this action for real, 2026-08-11: neither
// search this action's duplicate-prevention relies on
// (ListSubscriptionChargeTransactions, GetSubscriptionNextTransaction) is
// guaranteed instant-consistent with the ChargeSubscription write that
// precedes it — confirmed the hard way with a real duplicate charge
// created seconds after the first, on the exact back-to-back-invocation
// case this check exists for. A single immediate search-then-give-up
// isn't enough; retry a few times with a short wait before concluding "no
// match, proceed" to shrink (not eliminate — Paddle gives no consistency
// guarantee here, so this remains best-effort) the race window.
const (
	chargeSearchRetryAttempts = 4
	chargeSearchRetryDelay    = 3 * time.Second
)

// waitOrDone waits for d or returns false early if ctx is canceled/expires
// first — never sleeps through a caller giving up.
func waitOrDone(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

var _ action.Action = &SubscriptionChargeAction{}
var _ action.ActionWithConfigure = &SubscriptionChargeAction{}

func NewSubscriptionChargeAction() action.Action {
	return &SubscriptionChargeAction{}
}

type SubscriptionChargeAction struct {
	client *client.Client
}

type subscriptionChargeItemModel struct {
	PriceID  types.String `tfsdk:"price_id"`
	Quantity types.Int64  `tfsdk:"quantity"`
}

type subscriptionChargeActionModel struct {
	SubscriptionID   types.String                  `tfsdk:"subscription_id"`
	EffectiveFrom    types.String                  `tfsdk:"effective_from"`
	Items            []subscriptionChargeItemModel `tfsdk:"items"`
	OnPaymentFailure types.String                  `tfsdk:"on_payment_failure"`
	ReceiptData      types.String                  `tfsdk:"receipt_data"`
}

func (a *SubscriptionChargeAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscription_charge"
}

func (a *SubscriptionChargeAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Creates a one-time charge against a Paddle subscription — see https://developer.paddle.com/api-reference/subscriptions/create-subscription-charge. " +
			"**Catalog prices only** (`price_id` + `quantity`) — Paddle's API also accepts two inline non-catalog item shapes " +
			"(an ad hoc price against an existing product, or a fully inline product+price); those aren't supported by this " +
			"action yet, deliberately scoped out rather than half-modeled (docs/plans/paddle-provider-v3.md Step 2). Before " +
			"charging, checks whether an equivalent charge already exists and treats a match as already-done — best-effort, " +
			"**not a guarantee**: two deliberate, genuinely separate charges for the identical items would look identical to " +
			"this check too, and neither of Paddle's own search mechanisms (list-transactions, the next-renewal preview) is " +
			"guaranteed instant-consistent with a charge just created — confirmed the hard way, 2026-08-11, with a real " +
			"duplicate charge created seconds after the first during testing. This check now retries a few times with a " +
			"short wait before concluding \"no match, proceed\", which meaningfully shrinks but does not eliminate that " +
			"race window (docs/guardrails/money-moving-actions-no-blanket-retry.md). The check itself differs by " +
			"`effective_from`: an `immediately` charge is checked by searching transactions; a `next_billing_period` charge " +
			"is checked against the subscription's next-renewal preview instead, since Paddle creates no queryable " +
			"transaction for it until the subscription actually renews.",
		Attributes: map[string]actionschema.Attribute{
			"subscription_id": actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The subscription to charge (`sub_...`).",
			},
			"effective_from": actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "`immediately` or `next_billing_period`. Required, matching Paddle's own API (this field isn't optional there either).",
				Validators:          []validator.String{stringvalidator.OneOf("immediately", "next_billing_period")},
			},
			"items": actionschema.ListNestedAttribute{
				Required:            true,
				MarkdownDescription: "1-100 catalog price line items to charge.",
				NestedObject: actionschema.NestedAttributeObject{
					Attributes: map[string]actionschema.Attribute{
						"price_id": actionschema.StringAttribute{
							Required:            true,
							MarkdownDescription: "An existing catalog price (`pri_...`) — see `paddle_price`.",
						},
						"quantity": actionschema.Int64Attribute{
							Required:            true,
							MarkdownDescription: "Must be at least 1.",
							Validators:          []validator.Int64{int64validator.AtLeast(1)},
						},
					},
				},
			},
			"on_payment_failure": actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "`prevent_change` or `apply_change`. Left to Paddle's own default (`prevent_change`) if omitted.",
				Validators:          []validator.String{stringvalidator.OneOf("prevent_change", "apply_change")},
			},
			"receipt_data": actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Notes shown to the customer on the receipt/invoice. Only valid when `effective_from` is `immediately` — not cross-field-validated here, Paddle's own API error is authoritative. Max 1500 characters.",
			},
		},
	}
}

func (a *SubscriptionChargeAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "action")
	resp.Diagnostics.Append(diags...)
	a.client = c
}

func toAPISubscriptionChargeItems(items []subscriptionChargeItemModel) []client.SubscriptionChargeItem {
	out := make([]client.SubscriptionChargeItem, 0, len(items))
	for _, item := range items {
		out = append(out, client.SubscriptionChargeItem{
			PriceID:  item.PriceID.ValueString(),
			Quantity: item.Quantity.ValueInt64(),
		})
	}
	return out
}

// sameChargeItems reports whether want and got contain exactly the same
// (price_id, quantity) pairs, order-independent — the matching rule
// findMatchingCharge uses. A pure function over already-converted slices
// so it's unit-testable without an HTTP round trip or a tfsdk model.
func sameChargeItems(want []client.SubscriptionChargeItem, got []client.TransactionItem) bool {
	if len(want) != len(got) {
		return false
	}
	remaining := make([]client.TransactionItem, len(got))
	copy(remaining, got)
	for _, w := range want {
		found := -1
		for i, g := range remaining {
			if g.PriceID == w.PriceID && g.Quantity == w.Quantity {
				found = i
				break
			}
		}
		if found == -1 {
			return false
		}
		remaining = append(remaining[:found], remaining[found+1:]...)
	}
	return true
}

// findMatchingCharge implements this action's search-before-invoke check
// for effective_from="immediately"
// (docs/guardrails/money-moving-actions-no-blanket-retry.md): a pure
// function over already-fetched transactions so it's unit-testable
// without an HTTP round trip. See this action's Schema description for
// this check's known limitation (can't distinguish a retry from a
// deliberate second identical charge).
func findMatchingCharge(existing []client.Transaction, wantItems []client.SubscriptionChargeItem) *client.Transaction {
	for i := range existing {
		if sameChargeItems(wantItems, existing[i].Items) {
			return &existing[i]
		}
	}
	return nil
}

// findMatchingScheduledCharge is findMatchingCharge's counterpart for
// effective_from="next_billing_period": a charge scheduled for the next
// renewal produces no queryable Transaction at all until the subscription
// actually renews (confirmed against the real API and the real sandbox,
// 2026-08-10 — found by running this action's acceptance test for real,
// not assumed at design time), so ListSubscriptionChargeTransactions
// can't see it — searching it for a "next_billing_period" charge would
// always report no match, defeating search-before-invoke silently.
// Paddle's own next_transaction preview (GetSubscriptionNextTransaction)
// is the only way to see what's already queued.
func findMatchingScheduledCharge(preview *client.NextTransactionPreview, wantItems []client.SubscriptionChargeItem) bool {
	if preview == nil {
		return false
	}
	got := make([]client.TransactionItem, 0, len(preview.Items))
	for _, item := range preview.Items {
		got = append(got, client.TransactionItem(item))
	}
	return sameChargeItems(wantItems, got)
}

func (a *SubscriptionChargeAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config subscriptionChargeActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	subID := config.SubscriptionID.ValueString()
	effectiveFrom := config.EffectiveFrom.ValueString()
	wantItems := toAPISubscriptionChargeItems(config.Items)

	// Two different, non-interchangeable searches depending on
	// effective_from — see findMatchingScheduledCharge's comment for why
	// a single search can't cover both. Both retry
	// (chargeSearchRetryAttempts/chargeSearchRetryDelay) to mitigate
	// read-after-write lag on Paddle's side — see that const block's
	// comment.
	if effectiveFrom == "next_billing_period" {
		for attempt := 1; attempt <= chargeSearchRetryAttempts; attempt++ {
			preview, err := a.client.GetSubscriptionNextTransaction(ctx, subID)
			if err != nil {
				resp.Diagnostics.AddError("Error checking Paddle subscription's next transaction preview", client.FriendlyErrorMessage(err))
				return
			}
			if findMatchingScheduledCharge(preview, wantItems) {
				if resp.SendProgress != nil {
					resp.SendProgress(action.InvokeProgressEvent{
						Message: fmt.Sprintf("A one-time charge with these exact items is already queued for subscription %s's next renewal — not creating a duplicate.", subID),
					})
				}
				return
			}
			if attempt < chargeSearchRetryAttempts {
				if resp.SendProgress != nil {
					resp.SendProgress(action.InvokeProgressEvent{
						Message: fmt.Sprintf("No matching queued charge found on attempt %d/%d — waiting before retrying, in case Paddle's renewal preview hasn't caught up with a very recent charge yet.", attempt, chargeSearchRetryAttempts),
					})
				}
				if !waitOrDone(ctx, chargeSearchRetryDelay) {
					resp.Diagnostics.AddError("Timed out waiting to check Paddle subscription's next transaction preview", "The context was canceled or expired while retrying the duplicate-charge check. Not proceeding to charge — verify manually before retrying this action.")
					return
				}
			}
		}
	} else {
		for attempt := 1; attempt <= chargeSearchRetryAttempts; attempt++ {
			existing, err := a.client.ListSubscriptionChargeTransactions(ctx, subID)
			if err != nil {
				resp.Diagnostics.AddError("Error searching existing Paddle subscription charges", client.FriendlyErrorMessage(err))
				return
			}
			if match := findMatchingCharge(existing, wantItems); match != nil {
				if resp.SendProgress != nil {
					resp.SendProgress(action.InvokeProgressEvent{
						Message: fmt.Sprintf("A one-time charge with these exact items already exists for subscription %s (%s, status=%s) — not creating a duplicate.", subID, match.ID, match.Status),
					})
				}
				return
			}
			if attempt < chargeSearchRetryAttempts {
				if resp.SendProgress != nil {
					resp.SendProgress(action.InvokeProgressEvent{
						Message: fmt.Sprintf("No matching existing charge found on attempt %d/%d — waiting before retrying, in case Paddle's transaction search hasn't caught up with a very recent charge yet.", attempt, chargeSearchRetryAttempts),
					})
				}
				if !waitOrDone(ctx, chargeSearchRetryDelay) {
					resp.Diagnostics.AddError("Timed out waiting to search existing Paddle subscription charges", "The context was canceled or expired while retrying the duplicate-charge check. Not proceeding to charge — verify manually before retrying this action.")
					return
				}
			}
		}
	}

	chargeReq := client.SubscriptionChargeRequest{
		EffectiveFrom: effectiveFrom,
		Items:         wantItems,
	}
	if !config.OnPaymentFailure.IsNull() && !config.OnPaymentFailure.IsUnknown() {
		chargeReq.OnPaymentFailure = config.OnPaymentFailure.ValueString()
	}
	if !config.ReceiptData.IsNull() && !config.ReceiptData.IsUnknown() {
		v := config.ReceiptData.ValueString()
		chargeReq.ReceiptData = &v
	}

	updated, err := a.client.ChargeSubscription(ctx, subID, chargeReq)
	if err != nil {
		var nonRetryable *client.NonRetryableError
		if errors.As(err, &nonRetryable) {
			resp.Diagnostics.AddError(
				"Error charging Paddle subscription — outcome unknown",
				fmt.Sprintf("The request may or may not have been processed by Paddle before this failure. "+
					"Check subscription %s's transactions in the Paddle dashboard or API before retrying this action, "+
					"rather than re-running it blindly — Paddle has no idempotency-key protection against a duplicate charge. %s",
					subID, client.FriendlyErrorMessage(err)),
			)
			return
		}
		resp.Diagnostics.AddError("Error charging Paddle subscription", client.FriendlyErrorMessage(err))
		return
	}

	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{
			Message: fmt.Sprintf("Charge requested for subscription %s (status now %s).", subID, updated.Status),
		})
	}
}
