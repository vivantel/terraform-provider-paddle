package provider

import (
	"context"
	"regexp"

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

var _ resource.Resource = &CheckoutSettingsResource{}
var _ resource.ResourceWithImportState = &CheckoutSettingsResource{}

// checkoutSettingsID is the fixed ID of the account's single checkout URL
// settings object. The API has no ID for it; Terraform needs one.
const checkoutSettingsID = "checkout-url"

var checkoutURLPattern = regexp.MustCompile(`^https://[^\s]+$`)

func NewCheckoutSettingsResource() resource.Resource {
	return &CheckoutSettingsResource{}
}

type CheckoutSettingsResource struct {
	client *client.Client
}

type CheckoutSettingsResourceModel struct {
	ID         types.String `tfsdk:"id"`
	DefaultURL types.String `tfsdk:"default_url"`
}

// checkoutSettingsResourceStateModel wraps the model with timeouts — see
// notificationSettingResourceStateModel for why.
type checkoutSettingsResourceStateModel struct {
	CheckoutSettingsResourceModel
	Timeouts timeouts.Value `tfsdk:"timeouts"`
}

func (r *CheckoutSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_checkout_settings"
}

func (r *CheckoutSettingsResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The account's checkout URL settings: the default payment link Paddle opens when a checkout is created without a URL of its own. See [Paddle API Reference](https://developer.paddle.com/api-reference/settings/get-checkout-url). This is a singleton — there is one per Paddle account and environment, so declare at most one of these per account. Paddle requires a default payment link before it accepts any transaction created through the API. `terraform destroy` clears the setting (sets it to `null`) rather than deleting anything.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Always `checkout-url`; the API has no ID for this singleton.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"default_url": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The default payment link: an `https` URL (1–1000 characters, no whitespace). Paddle appends `?_ptxn=` and the transaction ID, and the page must have Paddle.js set up to open the checkout from it. In sandbox any `https` URL is accepted; in live accounts the domain must be an approved checkout domain (see the `paddle_checkout_domain` data source), otherwise Paddle returns `checkout_url_domain_not_approved`.",
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 1000),
					stringvalidator.RegexMatches(checkoutURLPattern, "must be an https URL with no whitespace"),
				},
			},
			"timeouts": describedTimeouts(ctx),
		},
	}
}

func (r *CheckoutSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "resource")
	resp.Diagnostics.Append(diags...)
	r.client = c
}

// toAPICheckoutSettingsUpdate builds the PATCH body from the plan.
func toAPICheckoutSettingsUpdate(m CheckoutSettingsResourceModel) client.CheckoutURLSettingsUpdate {
	if m.DefaultURL.IsNull() || m.DefaultURL.IsUnknown() {
		return client.CheckoutURLSettingsUpdate{}
	}
	v := m.DefaultURL.ValueString()
	return client.CheckoutURLSettingsUpdate{DefaultURL: &v}
}

func fromAPICheckoutSettings(s client.CheckoutURLSettings, m *CheckoutSettingsResourceModel) {
	m.ID = types.StringValue(checkoutSettingsID)
	if s.DefaultURL == nil {
		m.DefaultURL = types.StringNull()
		return
	}
	m.DefaultURL = types.StringValue(*s.DefaultURL)
}

func (r *CheckoutSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan checkoutSettingsResourceStateModel
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

	got, err := r.client.UpdateCheckoutURLSettings(ctx, toAPICheckoutSettingsUpdate(plan.CheckoutSettingsResourceModel))
	if err != nil {
		resp.Diagnostics.AddError("Error setting Paddle checkout URL", client.FriendlyErrorMessage(err))
		return
	}
	fromAPICheckoutSettings(*got, &plan.CheckoutSettingsResourceModel)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *CheckoutSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Fetch only timeouts, not the whole model — same reasoning as every
	// other resource's Read() in this provider.
	var state checkoutSettingsResourceStateModel
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

	got, err := r.client.GetCheckoutURLSettings(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Paddle checkout URL", client.FriendlyErrorMessage(err))
		return
	}
	// Cleared outside Terraform (dashboard or API): the resource no longer
	// exists as far as config is concerned, so plan will re-create it.
	if got.DefaultURL == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	fromAPICheckoutSettings(*got, &state.CheckoutSettingsResourceModel)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *CheckoutSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan checkoutSettingsResourceStateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel, diags := resolveTimeout(ctx, plan.Timeouts, timeoutOpUpdate, defaultOpTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	defer cancel()

	got, err := r.client.UpdateCheckoutURLSettings(ctx, toAPICheckoutSettingsUpdate(plan.CheckoutSettingsResourceModel))
	if err != nil {
		resp.Diagnostics.AddError("Error updating Paddle checkout URL", client.FriendlyErrorMessage(err))
		return
	}
	fromAPICheckoutSettings(*got, &plan.CheckoutSettingsResourceModel)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *CheckoutSettingsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state checkoutSettingsResourceStateModel
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("timeouts"), &state.Timeouts)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel, diags := resolveTimeout(ctx, state.Timeouts, timeoutOpDelete, defaultOpTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	defer cancel()

	// There is nothing to delete: destroy clears the setting with an
	// explicit null.
	if _, err := r.client.UpdateCheckoutURLSettings(ctx, client.CheckoutURLSettingsUpdate{}); err != nil {
		resp.Diagnostics.AddError("Error clearing Paddle checkout URL", client.FriendlyErrorMessage(err))
	}
}

func (r *CheckoutSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID != checkoutSettingsID {
		resp.Diagnostics.AddError("Invalid import ID", "The checkout settings singleton is imported with the ID `"+checkoutSettingsID+"`.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), checkoutSettingsID)...)
}
