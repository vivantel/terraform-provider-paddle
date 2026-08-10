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

var _ action.Action = &SubscriptionPauseAction{}
var _ action.ActionWithConfigure = &SubscriptionPauseAction{}

func NewSubscriptionPauseAction() action.Action {
	return &SubscriptionPauseAction{}
}

type SubscriptionPauseAction struct {
	client *client.Client
}

type subscriptionPauseActionModel struct {
	SubscriptionID types.String `tfsdk:"subscription_id"`
	EffectiveFrom  types.String `tfsdk:"effective_from"`
	ResumeAt       types.String `tfsdk:"resume_at"`
	OnResume       types.String `tfsdk:"on_resume"`
}

func (a *SubscriptionPauseAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscription_pause"
}

func (a *SubscriptionPauseAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Pauses a Paddle subscription — see https://developer.paddle.com/api-reference/subscriptions/pause-subscription. " +
			"Checks the subscription's current status first and skips the call entirely if it's already `paused`, rather than " +
			"erroring or re-invoking (docs/guardrails/money-moving-actions-no-blanket-retry.md). No `paddle_subscription` resource " +
			"exists in this provider (docs/decisions/0010-v3-scope-lifecycle-actions.md) — `subscription_id` is a plain string.",
		Attributes: map[string]actionschema.Attribute{
			"subscription_id": actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The subscription to pause (`sub_...`).",
			},
			"effective_from": actionschema.StringAttribute{
				Required: true,
				MarkdownDescription: "`immediately` or `next_billing_period`. Required (no default applied here) even though " +
					"Paddle defaults to `next_billing_period` server-side if omitted — deliberately, so an immediate pause is " +
					"never an implicit choice.",
				Validators: []validator.String{stringvalidator.OneOf("immediately", "next_billing_period")},
			},
			"resume_at": actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "RFC 3339 timestamp: when the subscription should resume automatically. Omit for an indefinite pause (a deliberate choice, not this provider's default — Paddle's own default when the field is absent).",
			},
			"on_resume": actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "`continue_existing_billing_period` or `start_new_billing_period`. Left to Paddle's own default (`start_new_billing_period`) if omitted.",
				Validators:          []validator.String{stringvalidator.OneOf("continue_existing_billing_period", "start_new_billing_period")},
			},
		},
	}
}

func (a *SubscriptionPauseAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "action")
	resp.Diagnostics.Append(diags...)
	a.client = c
}

func (a *SubscriptionPauseAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config subscriptionPauseActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	subID := config.SubscriptionID.ValueString()

	alreadyDone, status, err := checkAlreadyInTargetState(ctx, a.client, subID, "paused")
	if err != nil {
		resp.Diagnostics.AddError("Error reading Paddle subscription", client.FriendlyErrorMessage(err))
		return
	}
	if alreadyDone {
		if resp.SendProgress != nil {
			resp.SendProgress(action.InvokeProgressEvent{
				Message: fmt.Sprintf("Subscription %s is already paused — not invoking pause again.", subID),
			})
		}
		return
	}

	pauseReq := client.SubscriptionPauseRequest{EffectiveFrom: config.EffectiveFrom.ValueString()}
	if !config.ResumeAt.IsNull() && !config.ResumeAt.IsUnknown() {
		v := config.ResumeAt.ValueString()
		pauseReq.ResumeAt = &v
	}
	if !config.OnResume.IsNull() && !config.OnResume.IsUnknown() {
		pauseReq.OnResume = config.OnResume.ValueString()
	}

	updated, err := a.client.PauseSubscription(ctx, subID, pauseReq)
	if err != nil {
		var nonRetryable *client.NonRetryableError
		if errors.As(err, &nonRetryable) {
			resp.Diagnostics.AddError(
				"Error pausing Paddle subscription — outcome unknown",
				fmt.Sprintf("The request may or may not have been processed by Paddle before this failure (subscription was %s beforehand). "+
					"Check subscription %s's status in the Paddle dashboard or API before retrying this action. %s",
					status, subID, client.FriendlyErrorMessage(err)),
			)
			return
		}
		resp.Diagnostics.AddError("Error pausing Paddle subscription", client.FriendlyErrorMessage(err))
		return
	}

	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{
			Message: fmt.Sprintf("Subscription %s pause requested (status now %s).", subID, updated.Status),
		})
	}
}
