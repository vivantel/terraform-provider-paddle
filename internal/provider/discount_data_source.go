package provider

import (
	"context"
	"fmt"

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
		MarkdownDescription: "Look up an existing Paddle discount by ID — see https://developer.paddle.com/api-reference/discounts/overview.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Paddle discount ID (`dsc_...`) to look up.",
			},
			"description":                 schema.StringAttribute{Computed: true},
			"type":                        schema.StringAttribute{Computed: true},
			"amount":                      schema.StringAttribute{Computed: true},
			"code":                        schema.StringAttribute{Computed: true},
			"enabled_for_checkout":        schema.BoolAttribute{Computed: true},
			"mode":                        schema.StringAttribute{Computed: true},
			"currency_code":               schema.StringAttribute{Computed: true},
			"recur":                       schema.BoolAttribute{Computed: true},
			"maximum_recurring_intervals": schema.Int64Attribute{Computed: true},
			"usage_limit":                 schema.Int64Attribute{Computed: true},
			"restrict_to": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
			},
			"expires_at":        schema.StringAttribute{Computed: true},
			"discount_group_id": schema.StringAttribute{Computed: true},
			"status":            schema.StringAttribute{Computed: true},
			"times_used":        schema.Int64Attribute{Computed: true},
			"created_at":        schema.StringAttribute{Computed: true},
			"updated_at":        schema.StringAttribute{Computed: true},
		},
	}
}

func (d *DiscountDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected data source configure type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
		return
	}
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
		resp.Diagnostics.AddError("Error reading Paddle discount", err.Error())
		return
	}

	resp.Diagnostics.Append(fromAPIDiscount(ctx, *discount, &config)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
