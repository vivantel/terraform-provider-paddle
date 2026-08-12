package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

var _ datasource.DataSource = &NotificationsDataSource{}

func NewNotificationsDataSource() datasource.DataSource {
	return &NotificationsDataSource{}
}

type NotificationsDataSource struct {
	client *client.Client
}

// NotificationSummaryModel deliberately excludes logs — including each
// notification's delivery logs here would mean one ListNotificationLogs
// call per notification returned (a real N+1-calls-per-result cost, per
// docs/plans/paddle-provider-v5.md Step 4's explicit call-out), unlike
// paddle_notification (singular), where exactly one extra call for exactly
// one match is cheap. Use paddle_notifications to find the notification ID
// you need, then paddle_notification (singular, by id) to get its logs.
type NotificationSummaryModel struct {
	ID                    types.String `tfsdk:"id"`
	NotificationSettingID types.String `tfsdk:"notification_setting_id"`
	Status                types.String `tfsdk:"status"`
	Type                  types.String `tfsdk:"type"`
	OccurredAt            types.String `tfsdk:"occurred_at"`
	DeliveredAt           types.String `tfsdk:"delivered_at"`
	TimesAttempted        types.Int64  `tfsdk:"times_attempted"`
}

type NotificationsDataSourceModel struct {
	NotificationSettingID types.String               `tfsdk:"notification_setting_id"`
	Status                types.String               `tfsdk:"status"`
	Notifications         []NotificationSummaryModel `tfsdk:"notifications"`
}

func (d *NotificationsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notifications"
}

func (d *NotificationsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "List every Paddle notification (delivery attempt) matching " +
			"`notification_setting_id`/`status` filters — the plural companion to `paddle_notification` " +
			"(which requires exactly one match). Leave both filters unset to list every notification in " +
			"the account. Deliberately excludes `logs` — unlike the singular data source, which fetches " +
			"one notification's delivery logs cheaply, doing that for every result here would be an " +
			"N+1 API call per match; look up a notification's `id` here, then feed it into " +
			"`paddle_notification` (singular) to get `logs`. See " +
			"https://developer.paddle.com/api-reference/notifications/overview.\n\n" +
			"**⚠️ An unfiltered (or loosely filtered) call to this data source lists every matching " +
			"notification in the account, one API call per page of results — a real cost against a large " +
			"account, and a large `notifications` list written into your Terraform state file on every " +
			"`plan`/`refresh`.** Narrow with `notification_setting_id`/`status` where practical.",
		Attributes: map[string]schema.Attribute{
			"notification_setting_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Filter by the owning notification destination's ID (`ntfset_...`). Leave unset to match notifications for any destination.",
			},
			"status": schema.StringAttribute{
				Optional: true,
				Validators: []validator.String{
					stringvalidator.OneOf(notificationStatuses...),
				},
				MarkdownDescription: "Filter by status: one of `not_attempted`, `needs_retry`, `delivered`, `failed`. Leave unset to match any status.",
			},
			"notifications": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Matching notifications (without `logs` — see above).",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                      schema.StringAttribute{Computed: true, MarkdownDescription: "Paddle notification ID (`ntf_...`)."},
						"notification_setting_id": schema.StringAttribute{Computed: true},
						"status":                  schema.StringAttribute{Computed: true},
						"type":                    schema.StringAttribute{Computed: true, MarkdownDescription: "Event type this notification carries, in `entity.event_type` format."},
						"occurred_at":             schema.StringAttribute{Computed: true},
						"delivered_at":            schema.StringAttribute{Computed: true, MarkdownDescription: "RFC 3339 date-time this notification was delivered, or `null` if not yet delivered."},
						"times_attempted":         schema.Int64Attribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *NotificationsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "data source")
	resp.Diagnostics.Append(diags...)
	d.client = c
}

func fromAPINotificationSummary(n client.Notification, m *NotificationSummaryModel) {
	m.ID = types.StringValue(n.ID)
	m.NotificationSettingID = types.StringValue(n.NotificationSettingID)
	m.Status = types.StringValue(n.Status)
	m.Type = types.StringValue(n.Type)
	m.OccurredAt = types.StringValue(n.OccurredAt)
	if n.DeliveredAt != nil {
		m.DeliveredAt = types.StringValue(*n.DeliveredAt)
	} else {
		m.DeliveredAt = types.StringNull()
	}
	m.TimesAttempted = types.Int64Value(n.TimesAttempted)
}

func (d *NotificationsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config NotificationsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	filter := client.NotificationListFilter{}
	if !config.NotificationSettingID.IsNull() && !config.NotificationSettingID.IsUnknown() {
		filter.NotificationSettingID = config.NotificationSettingID.ValueString()
	}
	if !config.Status.IsNull() && !config.Status.IsUnknown() {
		filter.Status = config.Status.ValueString()
	}
	// Limit: 0 (unlimited) — see subscriptions_data_source.go's identical
	// comment on why this data source has no "no filter set" guard.

	notifications, err := d.client.ListNotificationsFiltered(ctx, filter)
	if err != nil {
		resp.Diagnostics.AddError("Error listing Paddle notifications", client.FriendlyErrorMessage(err))
		return
	}

	config.Notifications = make([]NotificationSummaryModel, 0, len(notifications))
	for _, n := range notifications {
		var m NotificationSummaryModel
		fromAPINotificationSummary(n, &m)
		config.Notifications = append(config.Notifications, m)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
