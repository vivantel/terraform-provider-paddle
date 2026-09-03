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

var _ action.Action = &SubscriptionResumeAction{}
var _ action.ActionWithConfigure = &SubscriptionResumeAction{}

func NewSubscriptionResumeAction() action.Action {
	return &SubscriptionResumeAction{}
}

type SubscriptionResumeAction struct {
	client *client.Client
}

type subscriptionResumeActionModel struct {
	SubscriptionID types.String `tfsdk:"subscription_id"`
	EffectiveFrom  types.String `tfsdk:"effective_from"`
	OnResume       types.String `tfsdk:"on_resume"`
}

func (a *SubscriptionResumeAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscription_resume"
}

func (a *SubscriptionResumeAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Resumes a paused Paddle subscription. See [Paddle API Reference](https://developer.paddle.com/api-reference/subscriptions/resume-subscription). Checks the subscription's current status first and skips the call entirely if it's already `active`, rather than erroring or re-invoking. **This check is deliberately an exact match on `active`, not \"anything other than paused\"** — a `canceled` subscription is also not `paused`, but resume can't reach it from there; treating that as already-done would silently mask a real failure. Any other status falls through to Paddle's own response. No `paddle_subscription` resource exists in this provider — subscriptions are checkout-created, not declared in Terraform — so `subscription_id` is a plain string.",
		Attributes: map[string]actionschema.Attribute{
			"subscription_id": actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The paused subscription to resume (prefix `sub_...`).",
			},
			"effective_from": actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "When the resume takes effect: `immediately` or an RFC 3339 timestamp to schedule a future resume. Required — Paddle's own API treats this as required too, not merely defaulted.",
			},
			"on_resume": actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Behavior on resume: `continue_existing_billing_period` or `start_new_billing_period`. Left to Paddle's own default (`start_new_billing_period`) if omitted.",
				Validators:          []validator.String{stringvalidator.OneOf("continue_existing_billing_period", "start_new_billing_period")},
			},
		},
	}
}

func (a *SubscriptionResumeAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "action")
	resp.Diagnostics.Append(diags...)
	a.client = c
}

func (a *SubscriptionResumeAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config subscriptionResumeActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	subID := config.SubscriptionID.ValueString()

	// Deliberately "already active", never "not paused" — see this
	// action's Schema description and
	// docs/guardrails/money-moving-actions-no-blanket-retry.md.
	alreadyDone, status, err := checkAlreadyInTargetState(ctx, a.client, subID, "active")
	if err != nil {
		resp.Diagnostics.AddError("Error reading Paddle subscription", client.FriendlyErrorMessage(err))
		return
	}
	if alreadyDone {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{
				Message: fmt.Sprintf("Subscription %s is already active — not invoking resume again.", subID),
			})
		}
		return
	}

	resumeReq := client.SubscriptionResumeRequest{EffectiveFrom: config.EffectiveFrom.ValueString()}
	if !config.OnResume.IsNull() && !config.OnResume.IsUnknown() {
		resumeReq.OnResume = config.OnResume.ValueString()
	}

	updated, err := a.client.ResumeSubscription(ctx, subID, resumeReq)
	if err != nil {
		var nonRetryable *client.NonRetryableError
		if errors.As(err, &nonRetryable) {
			resp.Diagnostics.AddError(
				"Error resuming Paddle subscription — outcome unknown",
				fmt.Sprintf("The request may or may not have been processed by Paddle before this failure (subscription was %s beforehand). "+
					"Check subscription %s's status in the Paddle dashboard or API before retrying this action. %s",
					status, subID, client.FriendlyErrorMessage(err)),
			)
			return
		}
		resp.Diagnostics.AddError("Error resuming Paddle subscription", client.FriendlyErrorMessage(err))
		return
	}

	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{
			Message: fmt.Sprintf("Subscription %s resume requested (status now %s).", subID, updated.Status),
		})
	}
}
