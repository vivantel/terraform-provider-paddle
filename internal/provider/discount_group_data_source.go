package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

var _ datasource.DataSource = &DiscountGroupDataSource{}

func NewDiscountGroupDataSource() datasource.DataSource {
	return &DiscountGroupDataSource{}
}

type DiscountGroupDataSource struct {
	client *client.Client
}

func (d *DiscountGroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_discount_group"
}

func (d *DiscountGroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing Paddle discount group by ID. See [Paddle API Reference](https://developer.paddle.com/api-reference/discount-groups/overview).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Paddle discount group ID (prefix `dsg_...`) to look up.",
			},
			"name":   schema.StringAttribute{Computed: true, MarkdownDescription: "Discount group name (1–500 characters)."},
			"status": schema.StringAttribute{Computed: true, MarkdownDescription: "Group status: `active` or `archived`."},
		},
	}
}

func (d *DiscountGroupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "data source")
	resp.Diagnostics.Append(diags...)
	d.client = c
}

func (d *DiscountGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	// Fetch just id, not the whole model — same reasoning as every other
	// data source in this provider.
	var config DiscountGroupResourceModel
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("id"), &config.ID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	group, err := d.client.GetDiscountGroup(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading Paddle discount group", client.FriendlyErrorMessage(err))
		return
	}

	fromAPIDiscountGroup(*group, &config)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
