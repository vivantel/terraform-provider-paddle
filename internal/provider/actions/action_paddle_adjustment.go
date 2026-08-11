package actions

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

var _ action.Action = &AdjustmentAction{}
var _ action.ActionWithConfigure = &AdjustmentAction{}

func NewAdjustmentAction() action.Action {
	return &AdjustmentAction{}
}

type AdjustmentAction struct {
	client *client.Client
}

type adjustmentItemModel struct {
	ItemID types.String `tfsdk:"item_id"`
	Type   types.String `tfsdk:"type"`
	Amount types.String `tfsdk:"amount"`
}

type adjustmentActionModel struct {
	Action        types.String          `tfsdk:"action"`
	Type          types.String          `tfsdk:"type"`
	TaxMode       types.String          `tfsdk:"tax_mode"`
	TransactionID types.String          `tfsdk:"transaction_id"`
	Reason        types.String          `tfsdk:"reason"`
	Items         []adjustmentItemModel `tfsdk:"items"`
}

func (a *AdjustmentAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_adjustment"
}

func (a *AdjustmentAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Creates a Paddle adjustment (refund, credit, or a chargeback-related record) against a transaction — " +
			"see https://developer.paddle.com/api-reference/adjustments/create-adjustment. Paddle has no idempotency-key support, " +
			"and adjustments have no update/delete operation once created, so this action lists existing adjustments for the same " +
			"`transaction_id` and treats a match on `action`+`reason` (and `type`, if set) as already-done rather than creating a " +
			"second one — best-effort correlation, not a guarantee (docs/guardrails/money-moving-actions-no-blanket-retry.md). " +
			"**This moves real money or changes a real customer's balance in a live (non-sandbox) environment.** See this provider's " +
			"README for operational guidance (a separate, tightly-scoped API key; not running this under `-auto-approve` without " +
			"review) before using it in an automated pipeline.",
		Attributes: map[string]actionschema.Attribute{
			"action": actionschema.StringAttribute{
				Required: true,
				MarkdownDescription: "One of `credit`, `refund`, `chargeback`, `chargeback_reverse`, `chargeback_warning`, " +
					"`chargeback_warning_reverse`, `credit_reverse` — the full enum Paddle's API accepts.",
				Validators: []validator.String{
					stringvalidator.OneOf("credit", "refund", "chargeback", "chargeback_reverse", "chargeback_warning", "chargeback_warning_reverse", "credit_reverse"),
				},
			},
			"type": actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "`full` or `partial`. Paddle defaults to `partial` server-side if omitted — left unset here rather than defaulted client-side, since actions have no state to reconcile a client-side default against.",
				Validators:          []validator.String{stringvalidator.OneOf("full", "partial")},
			},
			"tax_mode": actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "`internal` or `external`. Only meaningful for partial adjustments.",
				Validators:          []validator.String{stringvalidator.OneOf("internal", "external")},
			},
			"transaction_id": actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The transaction being adjusted (`txn_...`). Must be completed (auto-collected), or billed/past_due (manually-collected).",
			},
			"reason": actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Why this adjustment is being made. Also used, best-effort, to detect whether an equivalent adjustment already exists — see this action's own top-level description.",
			},
			"items": actionschema.ListNestedAttribute{
				Optional: true,
				MarkdownDescription: "Line items to adjust. Required by Paddle's API when `type` is `partial` — not cross-field-validated " +
					"here, Paddle's own API error is authoritative (same default this provider already applies to " +
					"`paddle_discount`'s `discount_group_id`).",
				NestedObject: actionschema.NestedAttributeObject{
					Attributes: map[string]actionschema.Attribute{
						"item_id": actionschema.StringAttribute{
							Required:            true,
							MarkdownDescription: "The transaction line item being adjusted (`txnitm_...`).",
						},
						"type": actionschema.StringAttribute{
							Required:            true,
							MarkdownDescription: "`full`, `partial`, `tax`, or `proration`.",
							Validators:          []validator.String{stringvalidator.OneOf("full", "partial", "tax", "proration")},
						},
						"amount": actionschema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Required by Paddle's API when this item's `type` is `partial`; omit otherwise.",
						},
					},
				},
			},
		},
	}
}

func (a *AdjustmentAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "action")
	resp.Diagnostics.Append(diags...)
	a.client = c
}

// findMatchingAdjustment implements this action's search-before-invoke
// check (docs/guardrails/money-moving-actions-no-blanket-retry.md): a
// pure function over already-fetched adjustments so it's unit-testable
// without an HTTP round trip. Best-effort correlation on action+reason
// (and type, if the caller specified one) — Adjustments has no
// client-settable reference field to match on exactly (confirmed against
// the real API reference, unlike create-subscription-charge's
// custom_data), so this is the same caveat class as every other
// best-effort match in this provider's action layer.
func findMatchingAdjustment(existing []client.Adjustment, wantAction, wantReason, wantType string) *client.Adjustment {
	for i := range existing {
		e := existing[i]
		if e.Action != wantAction || e.Reason != wantReason {
			continue
		}
		if wantType != "" && e.Type != wantType {
			continue
		}
		return &existing[i]
	}
	return nil
}

func toAPIAdjustment(m adjustmentActionModel) client.Adjustment {
	adj := client.Adjustment{
		Action:        m.Action.ValueString(),
		TransactionID: m.TransactionID.ValueString(),
		Reason:        m.Reason.ValueString(),
	}
	if !m.Type.IsNull() && !m.Type.IsUnknown() {
		adj.Type = m.Type.ValueString()
	}
	if !m.TaxMode.IsNull() && !m.TaxMode.IsUnknown() {
		adj.TaxMode = m.TaxMode.ValueString()
	}
	for _, item := range m.Items {
		apiItem := client.AdjustmentItem{
			ItemID: item.ItemID.ValueString(),
			Type:   item.Type.ValueString(),
		}
		if !item.Amount.IsNull() && !item.Amount.IsUnknown() {
			amount := item.Amount.ValueString()
			apiItem.Amount = &amount
		}
		adj.Items = append(adj.Items, apiItem)
	}
	return adj
}

func (a *AdjustmentAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config adjustmentActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	wantAction := config.Action.ValueString()
	wantReason := config.Reason.ValueString()
	wantType := ""
	if !config.Type.IsNull() && !config.Type.IsUnknown() {
		wantType = config.Type.ValueString()
	}

	existing, err := a.client.ListAdjustments(ctx, config.TransactionID.ValueString())
	if err != nil {
		// TEMPORARY debug aid, 2026-08-11: FriendlyErrorMessage only
		// surfaces error.detail/error.code, which for this specific
		// failure is just "Invalid request. (bad_request)" -- too
		// generic to diagnose from CI logs alone. err.Error() includes
		// the full raw response body via *client.APIError.Error(). Revert
		// to FriendlyErrorMessage alone once the real cause is found.
		resp.Diagnostics.AddError("Error searching existing Paddle adjustments", client.FriendlyErrorMessage(err)+" | raw: "+err.Error())
		return
	}
	if match := findMatchingAdjustment(existing, wantAction, wantReason, wantType); match != nil {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{
				Message: fmt.Sprintf("An adjustment matching this action+reason already exists for %s (%s, status=%s) — not creating a duplicate.", config.TransactionID.ValueString(), match.ID, match.Status),
			})
		}
		return
	}

	created, err := a.client.CreateAdjustment(ctx, toAPIAdjustment(config))
	if err != nil {
		var nonRetryable *client.NonRetryableError
		if errors.As(err, &nonRetryable) {
			resp.Diagnostics.AddError(
				"Error creating Paddle adjustment — outcome unknown",
				"The request may or may not have been processed by Paddle before this failure. "+
					"Check the transaction's adjustments in the Paddle dashboard or API before retrying this action, "+
					"rather than re-running it blindly — Paddle has no idempotency-key protection against a duplicate refund/credit. "+
					client.FriendlyErrorMessage(err),
			)
			return
		}
		resp.Diagnostics.AddError("Error creating Paddle adjustment", client.FriendlyErrorMessage(err))
		return
	}

	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{
			Message: fmt.Sprintf("Created Paddle adjustment %s (status=%s) for transaction %s.", created.ID, created.Status, config.TransactionID.ValueString()),
		})
	}
}
