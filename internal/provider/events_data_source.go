package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

var _ datasource.DataSource = &EventsDataSource{}

func NewEventsDataSource() datasource.DataSource {
	return &EventsDataSource{}
}

type EventsDataSource struct {
	client *client.Client
}

type EventModel struct {
	ID         types.String `tfsdk:"id"`
	Type       types.String `tfsdk:"type"`
	OccurredAt types.String `tfsdk:"occurred_at"`
	Data       types.String `tfsdk:"data"`
}

type EventsDataSourceModel struct {
	Type   types.List   `tfsdk:"type"`
	Events []EventModel `tfsdk:"events"`
}

func (d *EventsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_events"
}

func (d *EventsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "List Paddle account events, optionally filtered by `type` — general " +
			"account-activity lookup. See https://developer.paddle.com/api-reference/events/overview.\n\n" +
			"**Paddle retains events for 90 days only — events older than that are gone, not just " +
			"paginated away.** A query for something that happened more than 90 days ago returns an empty " +
			"`events` list, indistinguishable from \"nothing of that type ever happened\"; there is no way " +
			"to look further back. Paddle's `/events` API also has no date-range filter at all (confirmed " +
			"against the real API reference, 2026-08-11) — `type` is the only server-side filter this data " +
			"source can apply.",
		Attributes: map[string]schema.Attribute{
			"type": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Filter by event type(s), e.g. `product.created`. Leave unset to list every event type (subject to the 90-day retention window above).",
			},
			"events": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Matching events, most recent first (Paddle's default `id[DESC]` ordering).",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":   schema.StringAttribute{Computed: true, MarkdownDescription: "Paddle event ID (`evt_...`)."},
						"type": schema.StringAttribute{Computed: true, MarkdownDescription: "Event type, in `entity.event_type` format, e.g. `product.created`."},
						"occurred_at": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "RFC 3339 date-time this event occurred.",
						},
						"data": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "The event's `data` payload (the new or changed entity), JSON-encoded as a string — its shape varies by event type, so it isn't broken out into typed attributes here.",
						},
					},
				},
			},
		},
	}
}

func (d *EventsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "data source")
	resp.Diagnostics.Append(diags...)
	d.client = c
}

func (d *EventsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config EventsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var eventTypes []string
	if !config.Type.IsNull() && !config.Type.IsUnknown() {
		resp.Diagnostics.Append(config.Type.ElementsAs(ctx, &eventTypes, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	events, err := d.client.ListEvents(ctx, eventTypes)
	if err != nil {
		resp.Diagnostics.AddError("Error listing Paddle events", client.FriendlyErrorMessage(err))
		return
	}

	config.Events = make([]EventModel, 0, len(events))
	for _, e := range events {
		data := string(e.Data)
		if data == "" {
			data = "null"
		}
		config.Events = append(config.Events, EventModel{
			ID:         types.StringValue(e.ID),
			Type:       types.StringValue(e.Type),
			OccurredAt: types.StringValue(e.OccurredAt),
			Data:       types.StringValue(data),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
