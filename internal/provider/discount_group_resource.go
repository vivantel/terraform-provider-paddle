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

var _ resource.Resource = &DiscountGroupResource{}
var _ resource.ResourceWithImportState = &DiscountGroupResource{}

func NewDiscountGroupResource() resource.Resource {
	return &DiscountGroupResource{}
}

type DiscountGroupResource struct {
	client *client.Client
}

type DiscountGroupResourceModel struct {
	ID     types.String `tfsdk:"id"`
	Name   types.String `tfsdk:"name"`
	Status types.String `tfsdk:"status"`
}

func (r *DiscountGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_discount_group"
}

func (r *DiscountGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Paddle discount group — see https://developer.paddle.com/api-reference/discount-groups/overview. " +
			"Groups multiple `paddle_discount`s together (via their `discount_group_id`) so their combined usage can be capped " +
			"as a set. Paddle has no hard delete for discount groups; `terraform destroy` archives it instead (status becomes `archived`), " +
			"the same pattern `paddle_product`/`paddle_price` use.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Paddle discount group ID (`dsg_...`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "1-500 characters.",
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "`active` or `archived`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *DiscountGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "resource")
	resp.Diagnostics.Append(diags...)
	r.client = c
}

func toAPIDiscountGroup(m DiscountGroupResourceModel) client.DiscountGroup {
	return client.DiscountGroup{
		Name: m.Name.ValueString(),
	}
}

func fromAPIDiscountGroup(g client.DiscountGroup, m *DiscountGroupResourceModel) {
	m.ID = types.StringValue(g.ID)
	m.Name = types.StringValue(g.Name)
	m.Status = types.StringValue(g.Status)
}

func (r *DiscountGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DiscountGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateDiscountGroup(ctx, toAPIDiscountGroup(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Paddle discount group", err.Error())
		return
	}

	fromAPIDiscountGroup(*created, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DiscountGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Fetch just id, not the whole model — same reasoning as every other
	// resource's Read() in this provider: DiscountGroupResourceModel has no
	// nested struct field today, so a full req.State.Get wouldn't actually
	// crash right now, but this keeps the resource correct by construction
	// rather than by accident if a future nested attribute is added.
	var state DiscountGroupResourceModel
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &state.ID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	group, err := r.client.GetDiscountGroup(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Paddle discount group", err.Error())
		return
	}

	fromAPIDiscountGroup(*group, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *DiscountGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DiscountGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state DiscountGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateDiscountGroup(ctx, state.ID.ValueString(), toAPIDiscountGroup(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating Paddle discount group", err.Error())
		return
	}

	fromAPIDiscountGroup(*updated, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DiscountGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DiscountGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// A 404 here means the group is already gone — successful destroy, not
	// an error. Same tolerance every other resource's Delete() has for the
	// same status.
	if err := r.client.ArchiveDiscountGroup(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error archiving Paddle discount group", err.Error())
	}
}

func (r *DiscountGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
