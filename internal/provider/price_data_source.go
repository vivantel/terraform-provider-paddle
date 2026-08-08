package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

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
		MarkdownDescription: "Look up an existing Paddle price by ID — see https://developer.paddle.com/api-reference/prices/overview.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Paddle price ID (`pri_...`) to look up.",
			},
			"product_id":  schema.StringAttribute{Computed: true},
			"description": schema.StringAttribute{Computed: true},
			"name":        schema.StringAttribute{Computed: true},
			"tax_mode":    schema.StringAttribute{Computed: true},
			"status":      schema.StringAttribute{Computed: true},
			"unit_price": schema.SingleNestedAttribute{
				Computed: true,
				Attributes: map[string]schema.Attribute{
					"amount":        schema.StringAttribute{Computed: true},
					"currency_code": schema.StringAttribute{Computed: true},
				},
			},
			"billing_cycle": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Null for a one-time price.",
				Attributes: map[string]schema.Attribute{
					"interval":  schema.StringAttribute{Computed: true},
					"frequency": schema.Int64Attribute{Computed: true},
				},
			},
			"quantity": schema.SingleNestedAttribute{
				Computed: true,
				Attributes: map[string]schema.Attribute{
					"minimum": schema.Int64Attribute{Computed: true},
					"maximum": schema.Int64Attribute{Computed: true},
				},
			},
		},
	}
}

func (d *PriceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *PriceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config PriceResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	price, err := d.client.GetPrice(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading Paddle price", err.Error())
		return
	}

	fromAPIPrice(*price, &config)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
