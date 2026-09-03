package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

var _ datasource.DataSource = &DiscountDataSource{}

func NewDiscountDataSource() datasource.DataSource {
	return &DiscountDataSource{}
}

type DiscountDataSource struct {
	client *client.Client
}

func (d *DiscountDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_discount"
}

func (d *DiscountDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing Paddle discount by ID. See [Paddle API Reference](https://developer.paddle.com/api-reference/discounts/overview).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Paddle discount ID (prefix `dsc_...`) to look up.",
			},
			"description":                 schema.StringAttribute{Computed: true, MarkdownDescription: "Internal discount description (1–500 characters)."},
			"type":                        schema.StringAttribute{Computed: true, MarkdownDescription: "Discount type: `flat`, `flat_per_seat`, or `percentage`."},
			"amount":                      schema.StringAttribute{Computed: true, MarkdownDescription: "Amount: `\"0.01\"`–`\"100\"` for `percentage`; lowest currency denomination for `flat`/`flat_per_seat` (e.g., `\"1000\"` = $10.00 for a 2-decimal currency)."},
			"code":                        schema.StringAttribute{Computed: true, MarkdownDescription: "Discount code (1–32 alphanumeric characters, case-insensitive)."},
			"enabled_for_checkout":        schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether this discount is enabled at checkout."},
			"mode":                        schema.StringAttribute{Computed: true, MarkdownDescription: "Discount mode: `standard` or `custom`."},
			"currency_code":               schema.StringAttribute{Computed: true, MarkdownDescription: "ISO 4217 currency code. Present when `type` is `flat` or `flat_per_seat`."},
			"recur":                       schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the discount applies to every billing period, not just the first."},
			"maximum_recurring_intervals": schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of billing periods the discount recurs for. Present when `recur` is `true`."},
			"usage_limit":                 schema.Int64Attribute{Computed: true, MarkdownDescription: "Maximum number of redemptions. Omitted for unlimited."},
			"restrict_to": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Product or price IDs this discount is restricted to. Omitted to apply to the whole catalog.",
			},
			"expires_at":        schema.StringAttribute{Computed: true, MarkdownDescription: "RFC 3339 date-time this discount expires. Omitted for a non-expiring discount."},
			"discount_group_id": schema.StringAttribute{Computed: true, MarkdownDescription: "Paddle discount group ID (prefix `dsg_...`), if this discount belongs to one."},
			"status":            schema.StringAttribute{Computed: true, MarkdownDescription: "Discount status: `active` or `archived`."},
			"times_used":        schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of times this discount has been redeemed."},
			"created_at":        schema.StringAttribute{Computed: true, MarkdownDescription: "RFC 3339 date-time this discount was created, set by Paddle."},
			"updated_at":        schema.StringAttribute{Computed: true, MarkdownDescription: "RFC 3339 date-time this discount was last updated, set by Paddle."},
			"custom_data":       schema.StringAttribute{Computed: true, MarkdownDescription: "Arbitrary key-value metadata attached to this discount."},
		},
	}
}

func (d *DiscountDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "data source")
	resp.Diagnostics.Append(diags...)
	d.client = c
}

func (d *DiscountDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	// Fetch just id, not the whole model — same reasoning as
	// product_data_source.go. Discount has no nested struct-typed field
	// today (RestrictTo is types.List, which handles null fine), so a
	// full Config.Get isn't actually broken here, but staying consistent
	// with the pattern that IS required for price_data_source.go avoids
	// this becoming a trap for whoever adds a nested attribute next.
	var config DiscountResourceModel
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("id"), &config.ID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	discount, err := d.client.GetDiscount(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading Paddle discount", client.FriendlyErrorMessage(err))
		return
	}

	resp.Diagnostics.Append(fromAPIDiscount(ctx, *discount, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
