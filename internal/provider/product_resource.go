package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

var _ resource.Resource = &ProductResource{}
var _ resource.ResourceWithImportState = &ProductResource{}

func NewProductResource() resource.Resource {
	return &ProductResource{}
}

type ProductResource struct {
	client *client.Client
}

type ProductResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	TaxCategory types.String `tfsdk:"tax_category"`
	Description types.String `tfsdk:"description"`
	Type        types.String `tfsdk:"type"`
	ImageURL    types.String `tfsdk:"image_url"`
	Status      types.String `tfsdk:"status"`
}

func (r *ProductResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_product"
}

func (r *ProductResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Paddle product — see https://developer.paddle.com/api-reference/products/overview. Paddle has no hard delete for products; `terraform destroy` archives it instead (status becomes `archived`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Paddle product ID (`pro_...`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "1-200 characters.",
			},
			"tax_category": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "One of: digital-goods, ebooks, implementation-services, professional-services, saas, software-programming-services, standard, training-services, website-hosting.",
			},
			"description": schema.StringAttribute{
				Optional: true,
			},
			"type": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "`standard` or `custom`. Defaults to `standard`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"image_url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Must be a publicly accessible HTTPS URL.",
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "`active` or `archived`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *ProductResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "resource")
	resp.Diagnostics.Append(diags...)
	r.client = c
}

func toAPIProduct(m ProductResourceModel) client.Product {
	p := client.Product{
		Name:        m.Name.ValueString(),
		TaxCategory: m.TaxCategory.ValueString(),
	}
	if !m.Description.IsNull() {
		v := m.Description.ValueString()
		p.Description = &v
	}
	if !m.Type.IsNull() && !m.Type.IsUnknown() {
		p.Type = m.Type.ValueString()
	}
	if !m.ImageURL.IsNull() {
		v := m.ImageURL.ValueString()
		p.ImageURL = &v
	}
	return p
}

func fromAPIProduct(p client.Product, m *ProductResourceModel) {
	m.ID = types.StringValue(p.ID)
	m.Name = types.StringValue(p.Name)
	m.TaxCategory = types.StringValue(p.TaxCategory)
	if p.Description != nil {
		m.Description = types.StringValue(*p.Description)
	} else {
		m.Description = types.StringNull()
	}
	m.Type = types.StringValue(p.Type)
	if p.ImageURL != nil {
		m.ImageURL = types.StringValue(*p.ImageURL)
	} else {
		m.ImageURL = types.StringNull()
	}
	m.Status = types.StringValue(p.Status)
}

func (r *ProductResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ProductResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateProduct(ctx, toAPIProduct(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Paddle product", err.Error())
		return
	}

	fromAPIProduct(*created, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ProductResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Fetch just id, not the whole model. Every ProductResourceModel field
	// is types.String today, which handles a null Computed-only attribute
	// fine — so a full State.Get isn't actually broken here the way it
	// was for price_resource.go's Read()/import (see that file's comment)
	// — but fetching only what this method needs (id) before
	// fromAPIProduct overwrites state wholesale below keeps this resource
	// correct by construction, not by accident of which field types it
	// happens to have today (/code-review high finding: this was the one
	// resource still doing a full decode).
	var state ProductResourceModel
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &state.ID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	product, err := r.client.GetProduct(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Paddle product", err.Error())
		return
	}

	fromAPIProduct(*product, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ProductResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ProductResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state ProductResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateProduct(ctx, state.ID.ValueString(), toAPIProduct(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating Paddle product", err.Error())
		return
	}

	fromAPIProduct(*updated, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ProductResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ProductResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// A 404 here means the product is already gone (archived/deleted
	// outside Terraform, or a prior partial destroy) — that's a
	// successful destroy from Terraform's perspective, not an error, the
	// same tolerance Read() already has for the same status.
	if err := r.client.ArchiveProduct(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error archiving Paddle product", err.Error())
	}
}

func (r *ProductResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
