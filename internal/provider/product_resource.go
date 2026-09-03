package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
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

// ProductResourceModel is deliberately timeouts-free: product_data_source.go
// decodes state into this exact type too, and its schema has no "timeouts"
// attribute (only resources get one) — a Timeouts field here would make
// every data source Read() fail with "Value Conversion Error: Struct
// defines fields not found in object: timeouts", confirmed the hard way
// via a real acceptance-test failure, 2026-08-12. See
// productResourceStateModel below for where the resource-only timeouts
// field actually lives.
type ProductResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	TaxCategory types.String `tfsdk:"tax_category"`
	Description types.String `tfsdk:"description"`
	Type        types.String `tfsdk:"type"`
	ImageURL    types.String `tfsdk:"image_url"`
	Status      types.String `tfsdk:"status"`
	CustomData  types.String `tfsdk:"custom_data"`
}

// productResourceStateModel is what Create/Read/Update/Delete actually
// decode Plan/State into — ProductResourceModel plus the resource-only
// "timeouts" attribute. The embedded field's tfsdk-tagged fields are
// promoted by terraform-plugin-framework's reflection (confirmed via
// internal/reflect/helpers.go's explicit anonymous-field support), so
// toAPIProduct/fromAPIProduct keep operating on plain ProductResourceModel
// values unchanged — call sites just pass state.ProductResourceModel.
type productResourceStateModel struct {
	ProductResourceModel
	Timeouts timeouts.Value `tfsdk:"timeouts"`
}

func (r *ProductResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_product"
}

func (r *ProductResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Paddle product is the top-level catalog entity that prices and subscriptions attach to. See [Paddle API Reference](https://developer.paddle.com/api-reference/products/overview). Paddle has no hard delete for products; `terraform destroy` archives the product instead (status becomes `archived`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Paddle product ID (prefix `pro_...`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Product name (1–200 characters).",
			},
			"tax_category": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Tax category. One of: `digital-goods`, `ebooks`, `implementation-services`, `professional-services`, `saas`, `software-programming-services`, `standard`, `training-services`, `website-hosting`.",
				Validators: []validator.String{
					stringvalidator.OneOf(
						"digital-goods", "ebooks", "implementation-services",
						"professional-services", "saas", "software-programming-services",
						"standard", "training-services", "website-hosting",
					),
				},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Product description (1–200 characters).",
			},
			"type": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Product type: `standard` or `custom`. Defaults to `standard`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Validators: []validator.String{
					stringvalidator.OneOf("standard", "custom"),
				},
			},
			"image_url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Publicly accessible HTTPS URL for the product image.",
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Product status: `active` or `archived`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"custom_data": customDataAttribute(),
			"timeouts":    describedTimeouts(ctx),
		},
	}
}

func (r *ProductResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "resource")
	resp.Diagnostics.Append(diags...)
	r.client = c
}

func toAPIProduct(m ProductResourceModel) (client.Product, error) {
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
	customData, err := customDataToAPI(m.CustomData)
	if err != nil {
		return client.Product{}, err
	}
	p.CustomData = customData
	return p, nil
}

func fromAPIProduct(p client.Product, m *ProductResourceModel) error {
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
	customData, err := customDataFromAPI(p.CustomData)
	if err != nil {
		return err
	}
	m.CustomData = customData
	return nil
}

func (r *ProductResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan productResourceStateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiProduct, err := toAPIProduct(plan.ProductResourceModel)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("custom_data"), "Invalid custom_data", err.Error())
		return
	}

	ctx, cancel, diags := resolveTimeout(ctx, plan.Timeouts, timeoutOpCreate, defaultOpTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	defer cancel()

	created, err := r.client.CreateProduct(ctx, apiProduct)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Paddle product", client.FriendlyErrorMessage(err))
		return
	}

	if err := fromAPIProduct(*created, &plan.ProductResourceModel); err != nil {
		resp.Diagnostics.AddError("Error decoding Paddle product response", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ProductResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Fetch just id and timeouts, not the whole model. Every
	// ProductResourceModel field is types.String today, which handles a
	// null Computed-only attribute fine — so a full State.Get isn't
	// actually broken here the way it was for price_resource.go's
	// Read()/import (see that file's comment) — but fetching only what
	// this method needs before fromAPIProduct overwrites state wholesale
	// below keeps this resource correct by construction, not by accident
	// of which field types it happens to have today. timeouts must be
	// fetched explicitly (not part of fromAPIProduct's output) so the
	// previously-configured value round-trips through this Read instead
	// of being lost.
	var state productResourceStateModel
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &state.ID)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("timeouts"), &state.Timeouts)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel, diags := resolveTimeout(ctx, state.Timeouts, timeoutOpRead, defaultOpTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	defer cancel()

	product, err := r.client.GetProduct(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Paddle product", client.FriendlyErrorMessage(err))
		return
	}

	if err := fromAPIProduct(*product, &state.ProductResourceModel); err != nil {
		resp.Diagnostics.AddError("Error decoding Paddle product response", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ProductResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan productResourceStateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state productResourceStateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiProduct, err := toAPIProduct(plan.ProductResourceModel)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("custom_data"), "Invalid custom_data", err.Error())
		return
	}

	ctx, cancel, diags := resolveTimeout(ctx, plan.Timeouts, timeoutOpUpdate, defaultOpTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	defer cancel()

	updated, err := r.client.UpdateProduct(ctx, state.ID.ValueString(), apiProduct)
	if err != nil {
		resp.Diagnostics.AddError("Error updating Paddle product", client.FriendlyErrorMessage(err))
		return
	}

	if err := fromAPIProduct(*updated, &plan.ProductResourceModel); err != nil {
		resp.Diagnostics.AddError("Error decoding Paddle product response", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ProductResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state productResourceStateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel, diags := resolveTimeout(ctx, state.Timeouts, timeoutOpDelete, defaultOpTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	defer cancel()

	// A 404 here means the product is already gone (archived/deleted
	// outside Terraform, or a prior partial destroy) — that's a
	// successful destroy from Terraform's perspective, not an error, the
	// same tolerance Read() already has for the same status.
	if err := r.client.ArchiveProduct(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error archiving Paddle product", client.FriendlyErrorMessage(err))
	}
}

func (r *ProductResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
