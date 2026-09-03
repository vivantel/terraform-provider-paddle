package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

var _ ephemeral.EphemeralResource = &NotificationSettingSecretEphemeral{}
var _ ephemeral.EphemeralResourceWithConfigure = &NotificationSettingSecretEphemeral{}

func NewNotificationSettingSecretEphemeral() ephemeral.EphemeralResource {
	return &NotificationSettingSecretEphemeral{}
}

type NotificationSettingSecretEphemeral struct {
	client *client.Client
}

type notificationSettingSecretEphemeralModel struct {
	NotificationSettingID types.String `tfsdk:"notification_setting_id"`
	EndpointSecretKey     types.String `tfsdk:"endpoint_secret_key"`
}

func (e *NotificationSettingSecretEphemeral) Metadata(_ context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_setting_secret"
}

func (e *NotificationSettingSecretEphemeral) Schema(_ context.Context, _ ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches a `paddle_notification_setting`'s webhook signing secret without writing it into Terraform state. `paddle_notification_setting` (the resource and its data source) also exposes `endpoint_secret_key` directly — `Computed`, `Sensitive` — but `Sensitive` only redacts CLI/log output, it does not encrypt state: that attribute still persists the real secret to your state file in plaintext, same as any other `Computed` attribute. `endpoint_secret_key` on `paddle_notification_setting` is deprecated for this reason; prefer this ephemeral resource wherever the secret only needs to exist for the current plan/apply (e.g. feeding it into a webhook consumer's own ephemeral/write-only configuration), not persisted at rest. Requires Terraform 1.10+ (ephemeral resource support) — see [Paddle API Reference](https://developer.paddle.com/api-reference/notification-settings/get-notification-setting), the same endpoint `paddle_notification_setting`'s own Read uses, called fresh on every `plan`/`apply` this ephemeral resource appears in rather than once.",
		Attributes: map[string]schema.Attribute{
			"notification_setting_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The notification setting to fetch the secret for (prefix `ntfset_...`) — see `paddle_notification_setting`.",
			},
			"endpoint_secret_key": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Secret key Paddle uses to sign webhook payloads sent to this destination. Never written to state.",
			},
		},
	}
}

func (e *NotificationSettingSecretEphemeral) Configure(_ context.Context, req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "ephemeral resource")
	resp.Diagnostics.Append(diags...)
	e.client = c
}

func (e *NotificationSettingSecretEphemeral) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	var config notificationSettingSecretEphemeralModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ns, err := e.client.GetNotificationSetting(ctx, config.NotificationSettingID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading Paddle notification setting", client.FriendlyErrorMessage(err))
		return
	}

	config.EndpointSecretKey = types.StringValue(ns.EndpointSecretKey)
	resp.Diagnostics.Append(resp.Result.Set(ctx, &config)...)
}
