package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

var _ datasource.DataSource = &ProductDataSource{}

func NewProductDataSource() datasource.DataSource {
	return &ProductDataSource{}
}

type ProductDataSource struct {
	client *client.Client
}

func (d *ProductDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_product"
}

func (d *ProductDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing Paddle product by ID — see https://developer.paddle.com/api-reference/products/overview.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Paddle product ID (`pro_...`) to look up.",
			},
			"name":         schema.StringAttribute{Computed: true},
			"tax_category": schema.StringAttribute{Computed: true},
			"description":  schema.StringAttribute{Computed: true},
			"type":         schema.StringAttribute{Computed: true},
			"image_url":    schema.StringAttribute{Computed: true},
			"status":       schema.StringAttribute{Computed: true},
		},
	}
}

func (d *ProductDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ProductDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	// Fetch just id, not the whole model. Every ProductResourceModel field
	// is types.String today, which handles a null Computed-only attribute
	// fine at Read-time — so a full Config.Get isn't actually broken here
	// the way it was for price_data_source.go — but fetching only what
	// this method actually needs (id) before fromAPIProduct overwrites
	// config wholesale below keeps this resource correct by construction,
	// not by accident of which field types it happens to have today.
	var config ProductResourceModel
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("id"), &config.ID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	product, err := d.client.GetProduct(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading Paddle product", err.Error())
		return
	}

	fromAPIProduct(*product, &config)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
