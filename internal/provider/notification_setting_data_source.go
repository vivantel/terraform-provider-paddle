package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

var _ datasource.DataSource = &NotificationSettingDataSource{}

func NewNotificationSettingDataSource() datasource.DataSource {
	return &NotificationSettingDataSource{}
}

type NotificationSettingDataSource struct {
	client *client.Client
}

func (d *NotificationSettingDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_setting"
}

func (d *NotificationSettingDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing Paddle notification setting by ID — see https://developer.paddle.com/api-reference/notification-settings/overview.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Paddle notification setting ID (`ntfset_...`) to look up.",
			},
			"description": schema.StringAttribute{Computed: true},
			"type":        schema.StringAttribute{Computed: true},
			"destination": schema.StringAttribute{Computed: true},
			"active":      schema.BoolAttribute{Computed: true},
			"subscribed_events": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
			},
			"api_version":              schema.Int64Attribute{Computed: true},
			"include_sensitive_fields": schema.BoolAttribute{Computed: true},
			"traffic_source":           schema.StringAttribute{Computed: true},
			"endpoint_secret_key": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Secret key Paddle uses to sign webhook payloads sent to this destination.",
			},
		},
	}
}

func (d *NotificationSettingDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "data source")
	resp.Diagnostics.Append(diags...)
	d.client = c
}

func (d *NotificationSettingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	// Fetch just id, not the whole model — same reasoning as every other
	// data source in this provider.
	var config NotificationSettingResourceModel
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("id"), &config.ID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ns, err := d.client.GetNotificationSetting(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading Paddle notification setting", client.FriendlyErrorMessage(err))
		return
	}

	resp.Diagnostics.Append(fromAPINotificationSetting(ctx, *ns, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
