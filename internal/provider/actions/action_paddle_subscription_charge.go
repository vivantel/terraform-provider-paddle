package actions

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

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
			"this check too (docs/guardrails/money-moving-actions-no-blanket-retry.md). The check itself differs by " +
			"`effective_from`, since Paddle only creates a queryable transaction record for an `immediately` charge — a " +
			"`next_billing_period` charge is checked against the subscription's next-renewal preview instead. **The " +
			"`next_billing_period` path is implemented per Paddle's documented API shape but not yet confirmed against a " +
			"real response** (2026-08-11: the preview didn't reliably reflect a just-queued charge quickly enough for a " +
			"real-sandbox test to verify) — prefer `immediately` if you depend on this action's duplicate-prevention.",
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
	// a single search can't cover both.
	if effectiveFrom == "next_billing_period" {
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
	} else {
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
