package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
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

// DiscountGroupResourceModel is deliberately timeouts-free — see
// ProductResourceModel's comment in product_resource.go for why:
// discount_group_data_source.go decodes state into this exact type too,
// and its schema has no "timeouts" attribute.
type DiscountGroupResourceModel struct {
	ID     types.String `tfsdk:"id"`
	Name   types.String `tfsdk:"name"`
	Status types.String `tfsdk:"status"`
}

// discountGroupResourceStateModel is what Create/Read/Update/Delete decode
// Plan/State into — see productResourceStateModel's comment in
// product_resource.go for why this wrapper exists.
type discountGroupResourceStateModel struct {
	DiscountGroupResourceModel
	Timeouts timeouts.Value `tfsdk:"timeouts"`
}

func (r *DiscountGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_discount_group"
}

func (r *DiscountGroupResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Read:   true,
				Update: true,
				Delete: true,
			}),
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
	var plan discountGroupResourceStateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel, diags := resolveTimeout(ctx, plan.Timeouts, timeoutOpCreate, defaultOpTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	defer cancel()

	created, err := r.client.CreateDiscountGroup(ctx, toAPIDiscountGroup(plan.DiscountGroupResourceModel))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Paddle discount group", client.FriendlyErrorMessage(err))
		return
	}

	fromAPIDiscountGroup(*created, &plan.DiscountGroupResourceModel)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DiscountGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Fetch just id, not the whole model — same reasoning as every other
	// resource's Read() in this provider: DiscountGroupResourceModel has no
	// nested struct field today, so a full req.State.Get wouldn't actually
	// crash right now, but this keeps the resource correct by construction
	// rather than by accident if a future nested attribute is added.
	var state discountGroupResourceStateModel
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

	group, err := r.client.GetDiscountGroup(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Paddle discount group", client.FriendlyErrorMessage(err))
		return
	}

	fromAPIDiscountGroup(*group, &state.DiscountGroupResourceModel)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *DiscountGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan discountGroupResourceStateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state discountGroupResourceStateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel, diags := resolveTimeout(ctx, plan.Timeouts, timeoutOpUpdate, defaultOpTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	defer cancel()

	updated, err := r.client.UpdateDiscountGroup(ctx, state.ID.ValueString(), toAPIDiscountGroup(plan.DiscountGroupResourceModel))
	if err != nil {
		resp.Diagnostics.AddError("Error updating Paddle discount group", client.FriendlyErrorMessage(err))
		return
	}

	fromAPIDiscountGroup(*updated, &plan.DiscountGroupResourceModel)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DiscountGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state discountGroupResourceStateModel
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

	// A 404 here means the group is already gone — successful destroy, not
	// an error. Same tolerance every other resource's Delete() has for the
	// same status.
	if err := r.client.ArchiveDiscountGroup(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error archiving Paddle discount group", client.FriendlyErrorMessage(err))
	}
}

func (r *DiscountGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
