package actions

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

var _ action.Action = &NotificationReplayAction{}
var _ action.ActionWithConfigure = &NotificationReplayAction{}

func NewNotificationReplayAction() action.Action {
	return &NotificationReplayAction{}
}

type NotificationReplayAction struct {
	client *client.Client
}

type notificationReplayActionModel struct {
	NotificationID types.String `tfsdk:"notification_id"`
}

func (a *NotificationReplayAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_replay"
}

func (a *NotificationReplayAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Replays a Paddle notification — attempts to resend a `delivered` or `failed` " +
			"notification to its destination. See https://developer.paddle.com/api-reference/notifications/" +
			"replay-notification. Replaying creates a *new* notification entity linked to the same underlying " +
			"event; it does not mutate or re-deliver the original notification record referenced by " +
			"`notification_id` in place. Unlike the money-moving actions in this provider " +
			"(docs/guardrails/money-moving-actions-no-blanket-retry.md), this action has no " +
			"search-before-invoke check and no special no-retry handling — a replay isn't financial or " +
			"irreversible; the worst case of an accidental duplicate invocation is one extra webhook " +
			"delivery attempt, not a real-world harm like a duplicate charge " +
			"(docs/decisions/0012-v5-scope-pii-data-sources-timeouts-testing.md item 4). Look up the " +
			"`notification_id` you need via `paddle_notification`/`paddle_notifications`.",
		Attributes: map[string]actionschema.Attribute{
			"notification_id": actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The notification to replay (`ntf_...`).",
			},
		},
	}
}

func (a *NotificationReplayAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "action")
	resp.Diagnostics.Append(diags...)
	a.client = c
}

func (a *NotificationReplayAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var config notificationReplayActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	notificationID := config.NotificationID.ValueString()

	replayed, err := a.client.ReplayNotification(ctx, notificationID)
	if err != nil {
		resp.Diagnostics.AddError("Error replaying Paddle notification", client.FriendlyErrorMessage(err))
		return
	}

	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{
			Message: fmt.Sprintf("Notification %s replayed as new notification %s.", notificationID, replayed.ID),
		})
	}
}
