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

var _ action.Action = &SubscriptionCancelAction{}
var _ action.ActionWithConfigure = &SubscriptionCancelAction{}

func NewSubscriptionCancelAction() action.Action {
	return &SubscriptionCancelAction{}
}

type SubscriptionCancelAction struct {
	client *client.Client
}

type subscriptionCancelActionModel struct {
	SubscriptionID types.String `tfsdk:"subscription_id"`
	EffectiveFrom  types.String `tfsdk:"effective_from"`
}

func (a *SubscriptionCancelAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscription_cancel"
}

func (a *SubscriptionCancelAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Cancels a Paddle subscription — see https://developer.paddle.com/api-reference/subscriptions/cancel-subscription. " +
			"**Canceling can't be undone** (Paddle's own words: \"You can't reinstate a canceled subscription\"). Checks the " +
			"subscription's current status first and skips the call entirely if it's already `canceled`, rather than erroring or " +
			"re-invoking (docs/guardrails/money-moving-actions-no-blanket-retry.md). No `paddle_subscription` resource exists in " +
			"this provider — subscriptions are checkout-created, not declared in Terraform " +
			"(docs/decisions/0010-v3-scope-lifecycle-actions.md) — so `subscription_id` is a plain string, not a resource reference.",
		Attributes: map[string]actionschema.Attribute{
			"subscription_id": actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The subscription to cancel (`sub_...`).",
			},
			"effective_from": actionschema.StringAttribute{
				Required: true,
				MarkdownDescription: "`immediately` or `next_billing_period`. Required (no default applied here) even though Paddle " +
					"defaults to `next_billing_period` server-side if omitted — deliberately, so an irreversible immediate " +
					"cancellation is never an implicit choice.",
				Validators: []validator.String{stringvalidator.OneOf("immediately", "next_billing_period")},
			},
		},
	}
}

func (a *SubscriptionCancelAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "action")
	resp.Diagnostics.Append(diags...)
	a.client = c
}

func (a *SubscriptionCancelAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config subscriptionCancelActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	subID := config.SubscriptionID.ValueString()

	alreadyDone, status, err := checkAlreadyInTargetState(ctx, a.client, subID, "canceled")
	if err != nil {
		resp.Diagnostics.AddError("Error reading Paddle subscription", client.FriendlyErrorMessage(err))
		return
	}
	if alreadyDone {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{
				Message: fmt.Sprintf("Subscription %s is already canceled — not invoking cancel again.", subID),
			})
		}
		return
	}

	updated, err := a.client.CancelSubscription(ctx, subID, client.SubscriptionCancelRequest{
		EffectiveFrom: config.EffectiveFrom.ValueString(),
	})
	if err != nil {
		var nonRetryable *client.NonRetryableError
		if errors.As(err, &nonRetryable) {
			resp.Diagnostics.AddError(
				"Error canceling Paddle subscription — outcome unknown",
				fmt.Sprintf("The request may or may not have been processed by Paddle before this failure (subscription was %s beforehand). "+
					"Check subscription %s's status in the Paddle dashboard or API before retrying this action. %s",
					status, subID, client.FriendlyErrorMessage(err)),
			)
			return
		}
		resp.Diagnostics.AddError("Error canceling Paddle subscription", client.FriendlyErrorMessage(err))
		return
	}

	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{
			Message: fmt.Sprintf("Subscription %s cancel requested (status now %s).", subID, updated.Status),
		})
	}
}
