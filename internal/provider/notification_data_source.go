package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

var _ datasource.DataSource = &NotificationDataSource{}

func NewNotificationDataSource() datasource.DataSource {
	return &NotificationDataSource{}
}

type NotificationDataSource struct {
	client *client.Client
}

type NotificationLogModel struct {
	ID                  types.String `tfsdk:"id"`
	ResponseCode        types.Int64  `tfsdk:"response_code"`
	ResponseContentType types.String `tfsdk:"response_content_type"`
	ResponseBody        types.String `tfsdk:"response_body"`
	AttemptedAt         types.String `tfsdk:"attempted_at"`
}

type NotificationDataSourceModel struct {
	ID                    types.String           `tfsdk:"id"`
	NotificationSettingID types.String           `tfsdk:"notification_setting_id"`
	Status                types.String           `tfsdk:"status"`
	Type                  types.String           `tfsdk:"type"`
	OccurredAt            types.String           `tfsdk:"occurred_at"`
	DeliveredAt           types.String           `tfsdk:"delivered_at"`
	TimesAttempted        types.Int64            `tfsdk:"times_attempted"`
	Logs                  []NotificationLogModel `tfsdk:"logs"`
}

// notificationStatuses is Paddle's real notification status enum,
// confirmed against the real API reference, 2026-08-11 — checked
// specifically rather than assumed to mirror paddle_events' filters, per
// docs/plans/paddle-provider-v4.md Step 5's explicit instruction.
var notificationStatuses = []string{"not_attempted", "needs_retry", "delivered", "failed"}

func (d *NotificationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification"
}

func (d *NotificationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing Paddle notification (one delivery attempt to a webhook " +
			"endpoint or email destination), either directly by `id` or by " +
			"`notification_setting_id`/`status` filters — the natural read-side companion to the " +
			"`paddle_notification_setting` resource, which only configures *where* to deliver, not what " +
			"was actually delivered or whether it succeeded. `logs` surfaces the actual HTTP response code " +
			"and body Paddle recorded for each delivery attempt. See " +
			"[Paddle API Reference](https://developer.paddle.com/api-reference/notifications/overview). If `id` is set, every " +
			"other filter is ignored and that notification is fetched directly. Otherwise, filters are " +
			"applied server-side and exactly one notification must match — zero or more than one match is " +
			"an error, not a silent first-result pick.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Paddle notification ID (prefix `ntf_...`) to look up directly. Leave unset to look up by the other filters instead.",
			},
			"notification_setting_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Filter by the owning notification destination's ID (prefix `ntfset_...`). Ignored if `id` is set.",
			},
			"status": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.OneOf(notificationStatuses...),
				},
				MarkdownDescription: "Filter by status: one of `not_attempted`, `needs_retry`, `delivered`, `failed`. Ignored if `id` is set. Also returned (computed) with the matched notification's actual status.",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Event type this notification carries, in `entity.event_type` format.",
			},
			"occurred_at": schema.StringAttribute{Computed: true, MarkdownDescription: "RFC 3339 date-time Paddle recorded for this notification."},
			"delivered_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "RFC 3339 date-time this notification was delivered, or `null` if not yet delivered.",
			},
			"times_attempted": schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of delivery attempts Paddle has recorded for this notification."},
			"logs": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Every delivery attempt Paddle recorded for this notification.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                    schema.StringAttribute{Computed: true, MarkdownDescription: "Paddle notification log ID (prefix `ntflog_...`)."},
						"response_code":         schema.Int64Attribute{Computed: true, MarkdownDescription: "HTTP status code the receiving server returned."},
						"response_content_type": schema.StringAttribute{Computed: true, MarkdownDescription: "Content type of the receiving server's response."},
						"response_body":         schema.StringAttribute{Computed: true, MarkdownDescription: "Body of the receiving server's response."},
						"attempted_at":          schema.StringAttribute{Computed: true, MarkdownDescription: "RFC 3339 date-time Paddle attempted this delivery."},
					},
				},
			},
		},
	}
}

func (d *NotificationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "data source")
	resp.Diagnostics.Append(diags...)
	d.client = c
}

func fromAPINotification(ctx context.Context, n client.Notification, logs []client.NotificationLog, m *NotificationDataSourceModel) {
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

	m.Logs = make([]NotificationLogModel, 0, len(logs))
	for _, l := range logs {
		m.Logs = append(m.Logs, NotificationLogModel{
			ID:                  types.StringValue(l.ID),
			ResponseCode:        types.Int64Value(l.ResponseCode),
			ResponseContentType: types.StringValue(l.ResponseContentType),
			ResponseBody:        types.StringValue(l.ResponseBody),
			AttemptedAt:         types.StringValue(l.AttemptedAt),
		})
	}
}

func (d *NotificationDataSource) fetchLogs(ctx context.Context, notificationID string) ([]client.NotificationLog, error) {
	return d.client.ListNotificationLogs(ctx, notificationID)
}

func (d *NotificationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config NotificationDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !config.ID.IsNull() && !config.ID.IsUnknown() && config.ID.ValueString() != "" {
		n, err := d.client.GetNotification(ctx, config.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error reading Paddle notification", client.FriendlyErrorMessage(err))
			return
		}
		logs, err := d.fetchLogs(ctx, n.ID)
		if err != nil {
			resp.Diagnostics.AddError("Error reading Paddle notification logs", client.FriendlyErrorMessage(err))
			return
		}
		fromAPINotification(ctx, *n, logs, &config)
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	filter := client.NotificationListFilter{}
	if !config.NotificationSettingID.IsNull() && !config.NotificationSettingID.IsUnknown() {
		filter.NotificationSettingID = config.NotificationSettingID.ValueString()
	}
	if !config.Status.IsNull() && !config.Status.IsUnknown() {
		filter.Status = config.Status.ValueString()
	}

	if notificationFilterEmpty("", filter.NotificationSettingID, filter.Status) {
		resp.Diagnostics.AddError(
			"Missing lookup key",
			"Set id, or at least one of notification_setting_id/status, to look up a Paddle notification.",
		)
		return
	}

	// Limit 2 — see subscription_data_source.go's identical comment.
	filter.Limit = 2
	notifications, err := d.client.ListNotificationsFiltered(ctx, filter)
	if err != nil {
		resp.Diagnostics.AddError("Error listing Paddle notifications", client.FriendlyErrorMessage(err))
		return
	}
	switch len(notifications) {
	case 0:
		resp.Diagnostics.AddError(
			"No matching Paddle notification",
			"No notification matched the given notification_setting_id/status filters. Narrow or correct your filter.",
		)
		return
	case 1:
		logs, err := d.fetchLogs(ctx, notifications[0].ID)
		if err != nil {
			resp.Diagnostics.AddError("Error reading Paddle notification logs", client.FriendlyErrorMessage(err))
			return
		}
		fromAPINotification(ctx, notifications[0], logs, &config)
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	default:
		resp.Diagnostics.AddError(
			"Multiple matching Paddle notifications",
			fmt.Sprintf("%d notifications matched the given notification_setting_id/status filters — narrow your filter (or set id directly) so exactly one matches.", len(notifications)),
		)
	}
}
