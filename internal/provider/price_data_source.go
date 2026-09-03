package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

var _ datasource.DataSource = &PriceDataSource{}

func NewPriceDataSource() datasource.DataSource {
	return &PriceDataSource{}
}

type PriceDataSource struct {
	client *client.Client
}

func (d *PriceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_price"
}

func (d *PriceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing Paddle price by ID. See [Paddle API Reference](https://developer.paddle.com/api-reference/prices/overview).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Paddle price ID (prefix `pri_...`) to look up.",
			},
			"product_id":  schema.StringAttribute{Computed: true, MarkdownDescription: "Paddle product ID (prefix `pro_...`) this price belongs to."},
			"description": schema.StringAttribute{Computed: true, MarkdownDescription: "Internal price description (2–500 characters)."},
			"name":        schema.StringAttribute{Computed: true, MarkdownDescription: "Customer-facing price name (1–150 characters)."},
			"tax_mode":    schema.StringAttribute{Computed: true, MarkdownDescription: "Tax mode: `account_setting`, `external`, `internal`, or `location`."},
			"status":      schema.StringAttribute{Computed: true, MarkdownDescription: "Price status: `active` or `archived`."},
			"unit_price": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Price per unit, in the lowest denomination.",
				Attributes: map[string]schema.Attribute{
					"amount":        schema.StringAttribute{Computed: true, MarkdownDescription: "Amount in the lowest denomination as a string (e.g., \"1000\" = $10.00 for a 2-decimal currency)."},
					"currency_code": schema.StringAttribute{Computed: true, MarkdownDescription: "ISO 4217 currency code (e.g., USD)."},
				},
			},
			"billing_cycle": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Null for a one-time price.",
				Attributes: map[string]schema.Attribute{
					"interval":  schema.StringAttribute{Computed: true, MarkdownDescription: "Interval unit: one of `day`, `week`, `month`, or `year`."},
					"frequency": schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of intervals between each billing."},
				},
			},
			"quantity": schema.SingleNestedAttribute{
				Computed: true,
				Attributes: map[string]schema.Attribute{
					"minimum": schema.Int64Attribute{Computed: true, MarkdownDescription: "Minimum quantity per order."},
					"maximum": schema.Int64Attribute{Computed: true, MarkdownDescription: "Maximum quantity per order."},
				},
			},
			"custom_data": schema.StringAttribute{Computed: true, MarkdownDescription: "Arbitrary key-value metadata attached to this price."},
		},
	}
}

func (d *PriceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "data source")
	resp.Diagnostics.Append(diags...)
	d.client = c
}

func (d *PriceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	// Fetch just id, not the whole model — same reasoning as
	// price_resource.go's Read(). Only id is user-supplied in this data
	// source's config; unit_price/billing_cycle/quantity are Computed-only
	// here, so they're null in req.Config at this point, and decoding a
	// null unit_price into the non-pointer unitPriceModel struct crashes
	// the same way import did (confirmed against the real sandbox: "Value
	// Conversion Error ... Path: unit_price, Target Type:
	// provider.unitPriceModel"). id is all this needs before
	// fromAPIPrice overwrites config wholesale below.
	var config PriceResourceModel
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("id"), &config.ID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	price, err := d.client.GetPrice(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading Paddle price", client.FriendlyErrorMessage(err))
		return
	}

	if err := fromAPIPrice(*price, &config); err != nil {
		resp.Diagnostics.AddError("Error decoding Paddle price response", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
