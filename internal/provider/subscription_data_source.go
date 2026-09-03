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

var _ datasource.DataSource = &SubscriptionDataSource{}

func NewSubscriptionDataSource() datasource.DataSource {
	return &SubscriptionDataSource{}
}

type SubscriptionDataSource struct {
	client *client.Client
}

type SubscriptionDataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	CustomerID   types.String `tfsdk:"customer_id"`
	Status       types.String `tfsdk:"status"`
	CurrencyCode types.String `tfsdk:"currency_code"`
	NextBilledAt types.String `tfsdk:"next_billed_at"`
	CreatedAt    types.String `tfsdk:"created_at"`
	UpdatedAt    types.String `tfsdk:"updated_at"`
}

// subscriptionStatuses is Paddle's real subscription status enum,
// confirmed in docs/facts/0006-subscription-transaction-events-notifications-api-shapes.md
// — not invented from a guess at the pattern other filters used.
var subscriptionStatuses = []string{"active", "canceled", "past_due", "paused", "trialing"}

func (d *SubscriptionDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscription"
}

func (d *SubscriptionDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing Paddle subscription, either directly by `id` or by " +
			"`customer_id`/`status` filters — closes the discovery gap the `paddle_subscription_cancel`/" +
			"`pause`/`resume`/`charge` actions otherwise have, since each of those requires a " +
			"`subscription_id` with no other in-Terraform way to find one. See " +
			"[Paddle API Reference](https://developer.paddle.com/api-reference/subscriptions/overview). If `id` is set, every " +
			"other filter is ignored and that subscription is fetched directly. Otherwise, `customer_id` " +
			"and/or `status` are applied as server-side filters and exactly one subscription must match — " +
			"zero or more than one match is an error, not a silent first-result pick.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Paddle subscription ID (prefix `sub_...`) to look up directly. Leave unset to look up by `customer_id`/`status` instead.",
			},
			"customer_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Filter by the owning customer's ID (prefix `ctm_...`). Ignored if `id` is set.",
			},
			"status": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.OneOf(subscriptionStatuses...),
				},
				MarkdownDescription: "Filter by status: one of `active`, `canceled`, `past_due`, `paused`, `trialing`. Ignored if `id` is set. Also returned (computed) with the matched subscription's actual status.",
			},
			"currency_code": schema.StringAttribute{Computed: true, MarkdownDescription: "ISO 4217 code for the subscription's primary currency."},
			"next_billed_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "RFC 3339 date-time of the next scheduled billing, or `null` if there isn't one (e.g., a canceled/paused subscription).",
			},
			"created_at": schema.StringAttribute{Computed: true, MarkdownDescription: "RFC 3339 date-time this subscription was created, set by Paddle."},
			"updated_at": schema.StringAttribute{Computed: true, MarkdownDescription: "RFC 3339 date-time this subscription was last updated, set by Paddle."},
		},
	}
}

func (d *SubscriptionDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "data source")
	resp.Diagnostics.Append(diags...)
	d.client = c
}

func fromAPISubscription(s client.Subscription, m *SubscriptionDataSourceModel) {
	m.ID = types.StringValue(s.ID)
	m.CustomerID = types.StringValue(s.CustomerID)
	m.Status = types.StringValue(s.Status)
	m.CurrencyCode = types.StringValue(s.CurrencyCode)
	if s.NextBilledAt != nil {
		m.NextBilledAt = types.StringValue(*s.NextBilledAt)
	} else {
		m.NextBilledAt = types.StringNull()
	}
	m.CreatedAt = types.StringValue(s.CreatedAt)
	m.UpdatedAt = types.StringValue(s.UpdatedAt)
}

func (d *SubscriptionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SubscriptionDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !config.ID.IsNull() && !config.ID.IsUnknown() && config.ID.ValueString() != "" {
		sub, err := d.client.GetSubscription(ctx, config.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error reading Paddle subscription", client.FriendlyErrorMessage(err))
			return
		}
		fromAPISubscription(*sub, &config)
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	filter := client.SubscriptionListFilter{}
	if !config.CustomerID.IsNull() && !config.CustomerID.IsUnknown() {
		filter.CustomerID = config.CustomerID.ValueString()
	}
	if !config.Status.IsNull() && !config.Status.IsUnknown() {
		filter.Status = config.Status.ValueString()
	}

	if subscriptionFilterEmpty("", filter.CustomerID, filter.Status) {
		resp.Diagnostics.AddError(
			"Missing lookup key",
			"Set id, or at least one of customer_id/status, to look up a Paddle subscription.",
		)
		return
	}

	// Limit 2: this branch only needs to know whether 0, exactly 1, or
	// more than 1 subscription matches — no reason to paginate the whole
	// account to exhaustion just to discard everything past the second
	// match.
	filter.Limit = 2
	subs, err := d.client.ListSubscriptionsFiltered(ctx, filter)
	if err != nil {
		resp.Diagnostics.AddError("Error listing Paddle subscriptions", client.FriendlyErrorMessage(err))
		return
	}
	switch len(subs) {
	case 0:
		resp.Diagnostics.AddError(
			"No matching Paddle subscription",
			"No subscription matched the given customer_id/status filters. Narrow or correct your filter.",
		)
		return
	case 1:
		fromAPISubscription(subs[0], &config)
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	default:
		resp.Diagnostics.AddError(
			"Multiple matching Paddle subscriptions",
			fmt.Sprintf("%d subscriptions matched the given customer_id/status filters — narrow your filter (or set id directly) so exactly one matches.", len(subs)),
		)
	}
}
