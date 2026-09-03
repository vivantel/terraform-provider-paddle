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

var _ datasource.DataSource = &SubscriptionsDataSource{}

func NewSubscriptionsDataSource() datasource.DataSource {
	return &SubscriptionsDataSource{}
}

type SubscriptionsDataSource struct {
	client *client.Client
}

type SubscriptionsDataSourceModel struct {
	CustomerID    types.String                  `tfsdk:"customer_id"`
	Status        types.String                  `tfsdk:"status"`
	Subscriptions []SubscriptionDataSourceModel `tfsdk:"subscriptions"`
}

func (d *SubscriptionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscriptions"
}

func (d *SubscriptionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "List every Paddle subscription matching `customer_id`/`status` filters — " +
			"the plural companion to `paddle_subscription` (which requires exactly one match). Leave both " +
			"filters unset to list every subscription in the account. See " +
			"[Paddle API Reference](https://developer.paddle.com/api-reference/subscriptions/overview).\n\n" +
			"**⚠️ An unfiltered (or loosely filtered) call to this data source lists every matching " +
			"subscription in the account, one API call per page of results — a real cost against a large " +
			"account, and a large `subscriptions` list written into your Terraform state file on every " +
			"`plan`/`refresh`.** Narrow with `customer_id`/`status` where practical.",
		Attributes: map[string]schema.Attribute{
			"customer_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Filter by the owning customer's ID (prefix `ctm_...`). Leave unset to match subscriptions for any customer.",
			},
			"status": schema.StringAttribute{
				Optional: true,
				Validators: []validator.String{
					stringvalidator.OneOf(subscriptionStatuses...),
				},
				MarkdownDescription: "Filter by status: one of `active`, `canceled`, `past_due`, `paused`, `trialing`. Leave unset to match any status.",
			},
			"subscriptions": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Matching subscriptions.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":             schema.StringAttribute{Computed: true, MarkdownDescription: "Paddle subscription ID (prefix `sub_...`)."},
						"customer_id":    schema.StringAttribute{Computed: true, MarkdownDescription: "Paddle customer ID (prefix `ctm_...`) owning this subscription."},
						"status":         schema.StringAttribute{Computed: true, MarkdownDescription: "One of `active`, `canceled`, `past_due`, `paused`, `trialing`."},
						"currency_code":  schema.StringAttribute{Computed: true, MarkdownDescription: "ISO 4217 code for this subscription's primary currency."},
						"next_billed_at": schema.StringAttribute{Computed: true, MarkdownDescription: "RFC 3339 date-time of the next scheduled billing, or `null` if there isn't one."},
						"created_at":     schema.StringAttribute{Computed: true, MarkdownDescription: "RFC 3339 date-time this subscription was created, set by Paddle."},
						"updated_at":     schema.StringAttribute{Computed: true, MarkdownDescription: "RFC 3339 date-time this subscription was last updated, set by Paddle."},
					},
				},
			},
		},
	}
}

func (d *SubscriptionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "data source")
	resp.Diagnostics.Append(diags...)
	d.client = c
}

func (d *SubscriptionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SubscriptionsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	filter := client.SubscriptionListFilter{}
	if !config.CustomerID.IsNull() && !config.CustomerID.IsUnknown() {
		filter.CustomerID = config.CustomerID.ValueString()
	}
	if !config.Status.IsNull() && !config.Status.IsUnknown() {
		filter.Status = config.Status.ValueString()
	}
	// Limit: 0 (unlimited) — deliberately different from the singular
	// data source's Limit: 2. An empty filter set is a legitimate use
	// case here ("list everything"), unlike the singular lookup, so
	// there's no "no filter set" hard-error guard the way
	// lookup_guard.go's *FilterEmpty functions give the singular data
	// sources — just the schema-level cost warning above.

	subs, err := d.client.ListSubscriptionsFiltered(ctx, filter)
	if err != nil {
		resp.Diagnostics.AddError("Error listing Paddle subscriptions", client.FriendlyErrorMessage(err))
		return
	}

	config.Subscriptions = make([]SubscriptionDataSourceModel, 0, len(subs))
	for _, s := range subs {
		var m SubscriptionDataSourceModel
		fromAPISubscription(s, &m)
		config.Subscriptions = append(config.Subscriptions, m)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
