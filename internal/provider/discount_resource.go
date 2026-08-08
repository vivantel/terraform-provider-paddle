package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

var _ resource.Resource = &DiscountResource{}
var _ resource.ResourceWithImportState = &DiscountResource{}

func NewDiscountResource() resource.Resource {
	return &DiscountResource{}
}

type DiscountResource struct {
	client *client.Client
}

type DiscountResourceModel struct {
	ID                        types.String `tfsdk:"id"`
	Description               types.String `tfsdk:"description"`
	Type                      types.String `tfsdk:"type"`
	Amount                    types.String `tfsdk:"amount"`
	Code                      types.String `tfsdk:"code"`
	EnabledForCheckout        types.Bool   `tfsdk:"enabled_for_checkout"`
	Mode                      types.String `tfsdk:"mode"`
	CurrencyCode              types.String `tfsdk:"currency_code"`
	Recur                     types.Bool   `tfsdk:"recur"`
	MaximumRecurringIntervals types.Int64  `tfsdk:"maximum_recurring_intervals"`
	UsageLimit                types.Int64  `tfsdk:"usage_limit"`
	RestrictTo                types.List   `tfsdk:"restrict_to"`
	ExpiresAt                 types.String `tfsdk:"expires_at"`
	DiscountGroupID           types.String `tfsdk:"discount_group_id"`
	Status                    types.String `tfsdk:"status"`
	TimesUsed                 types.Int64  `tfsdk:"times_used"`
	CreatedAt                 types.String `tfsdk:"created_at"`
	UpdatedAt                 types.String `tfsdk:"updated_at"`
}

func (r *DiscountResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_discount"
}

func (r *DiscountResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Paddle discount — see https://developer.paddle.com/api-reference/discounts/overview. " +
			"Paddle has no delete operation for discounts at all; `terraform destroy` sets `status = \"archived\"` " +
			"via a normal update, the same as `paddle_product`/`paddle_price` archive on destroy (though those two " +
			"have an actual archive semantic — discounts genuinely have no other removal path per Paddle's docs).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Paddle discount ID (`dsc_...`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"description": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "1-500 characters. Internal only, never shown to customers.",
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "`flat`, `flat_per_seat`, or `percentage`.",
				Validators: []validator.String{
					stringvalidator.OneOf("flat", "flat_per_seat", "percentage"),
				},
			},
			"amount": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "\"0.01\"-\"100\" for `percentage`; lowest currency denomination for `flat`/`flat_per_seat` (e.g. \"1000\" = $10.00 for a 2-decimal currency).",
			},
			"code": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "1-32 alphanumeric characters, case-insensitive. Optional+Computed, not " +
					"purely user-set: confirmed against the real sandbox that Paddle auto-generates a code " +
					"when this is omitted (e.g. \"3268E6WW3W\") rather than leaving it null, so modeling this " +
					"as Optional-only produced \"Provider produced inconsistent result after apply\" on the " +
					"very first real Create.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enabled_for_checkout": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Defaults to `true`.",
				Default:             booldefault.StaticBool(true),
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"mode": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "`standard` or `custom`. Defaults to `standard`.",
				Default:             stringdefault.StaticString("standard"),
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Validators: []validator.String{
					stringvalidator.OneOf("standard", "custom"),
				},
			},
			"currency_code": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "ISO 4217 code. Required by Paddle's API when `type` is `flat` or `flat_per_seat`; not accepted for `percentage` — the API enforces this, not this schema.",
			},
			"recur": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether the discount applies to every billing period of a recurring price, not just the first. Defaults to `false`.",
				Default:             booldefault.StaticBool(false),
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"maximum_recurring_intervals": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Minimum 1. Requires `recur = true` — the API enforces this, not this schema. Omit (or set null) for no limit.",
			},
			"usage_limit": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Minimum 1. Omit (or set null) for unlimited redemptions.",
			},
			"restrict_to": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Product or price IDs this discount is restricted to. Omit (or set null) to apply to the whole catalog.",
			},
			"expires_at": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "RFC 3339 date-time. Omit (or set null) for a discount that never expires.",
			},
			"discount_group_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Paddle discount group ID (`dsg_...`), if this discount belongs to one. A discount belongs to at most one group.",
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "`active` or `archived`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"times_used": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Number of times this discount has been redeemed. Paddle-assigned; not settable, and can drift between plans as it's used outside Terraform.",
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "RFC 3339 date-time this discount was created, set by Paddle.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"updated_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "RFC 3339 date-time this discount was last updated, set by Paddle. Deliberately has no UseStateForUnknown — it genuinely changes on every update, so it should show as \"known after apply\" whenever anything else changes.",
			},
		},
	}
}

func (r *DiscountResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected resource configure type", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
		return
	}
	r.client = c
}

func toAPIDiscount(ctx context.Context, m DiscountResourceModel) (client.Discount, diag.Diagnostics) {
	var diags diag.Diagnostics

	d := client.Discount{
		Description: m.Description.ValueString(),
		Type:        m.Type.ValueString(),
		Amount:      m.Amount.ValueString(),
	}
	if !m.Code.IsNull() {
		v := m.Code.ValueString()
		d.Code = &v
	}
	if !m.EnabledForCheckout.IsNull() && !m.EnabledForCheckout.IsUnknown() {
		v := m.EnabledForCheckout.ValueBool()
		d.EnabledForCheckout = &v
	}
	if !m.Mode.IsNull() && !m.Mode.IsUnknown() {
		d.Mode = m.Mode.ValueString()
	}
	if !m.CurrencyCode.IsNull() {
		v := m.CurrencyCode.ValueString()
		d.CurrencyCode = &v
	}
	if !m.Recur.IsNull() && !m.Recur.IsUnknown() {
		v := m.Recur.ValueBool()
		d.Recur = &v
	}
	if !m.MaximumRecurringIntervals.IsNull() {
		v := int(m.MaximumRecurringIntervals.ValueInt64())
		d.MaximumRecurringIntervals = &v
	}
	if !m.UsageLimit.IsNull() {
		v := int(m.UsageLimit.ValueInt64())
		d.UsageLimit = &v
	}
	if !m.RestrictTo.IsNull() && !m.RestrictTo.IsUnknown() {
		var items []string
		diags.Append(m.RestrictTo.ElementsAs(ctx, &items, false)...)
		d.RestrictTo = items
	}
	if !m.ExpiresAt.IsNull() {
		v := m.ExpiresAt.ValueString()
		d.ExpiresAt = &v
	}
	if !m.DiscountGroupID.IsNull() {
		v := m.DiscountGroupID.ValueString()
		d.DiscountGroupID = &v
	}
	return d, diags
}

func fromAPIDiscount(ctx context.Context, d client.Discount, m *DiscountResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	m.ID = types.StringValue(d.ID)
	m.Description = types.StringValue(d.Description)
	m.Type = types.StringValue(d.Type)
	m.Amount = types.StringValue(d.Amount)

	if d.Code != nil {
		m.Code = types.StringValue(*d.Code)
	} else {
		m.Code = types.StringNull()
	}
	if d.EnabledForCheckout != nil {
		m.EnabledForCheckout = types.BoolValue(*d.EnabledForCheckout)
	} else {
		m.EnabledForCheckout = types.BoolNull()
	}
	m.Mode = types.StringValue(d.Mode)
	if d.CurrencyCode != nil {
		m.CurrencyCode = types.StringValue(*d.CurrencyCode)
	} else {
		m.CurrencyCode = types.StringNull()
	}
	if d.Recur != nil {
		m.Recur = types.BoolValue(*d.Recur)
	} else {
		m.Recur = types.BoolNull()
	}
	if d.MaximumRecurringIntervals != nil {
		m.MaximumRecurringIntervals = types.Int64Value(int64(*d.MaximumRecurringIntervals))
	} else {
		m.MaximumRecurringIntervals = types.Int64Null()
	}
	if d.UsageLimit != nil {
		m.UsageLimit = types.Int64Value(int64(*d.UsageLimit))
	} else {
		m.UsageLimit = types.Int64Null()
	}
	if d.RestrictTo != nil {
		listVal, elemDiags := types.ListValueFrom(ctx, types.StringType, d.RestrictTo)
		diags.Append(elemDiags...)
		m.RestrictTo = listVal
	} else {
		m.RestrictTo = types.ListNull(types.StringType)
	}
	if d.ExpiresAt != nil {
		m.ExpiresAt = types.StringValue(*d.ExpiresAt)
	} else {
		m.ExpiresAt = types.StringNull()
	}
	if d.DiscountGroupID != nil {
		m.DiscountGroupID = types.StringValue(*d.DiscountGroupID)
	} else {
		m.DiscountGroupID = types.StringNull()
	}
	m.Status = types.StringValue(d.Status)
	m.TimesUsed = types.Int64Value(int64(d.TimesUsed))
	m.CreatedAt = types.StringValue(d.CreatedAt)
	m.UpdatedAt = types.StringValue(d.UpdatedAt)

	return diags
}

func (r *DiscountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DiscountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiDiscount, diags := toAPIDiscount(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateDiscount(ctx, apiDiscount)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Paddle discount", err.Error())
		return
	}

	resp.Diagnostics.Append(fromAPIDiscount(ctx, *created, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DiscountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Fetch just id, not the whole model — see the equivalent comment in
	// price_resource.go's Read(). Discount has no Required nested
	// attribute today (unlike Price's unit_price), so a full req.State.Get
	// wouldn't actually crash right now, but using the same narrow-fetch
	// pattern here keeps this resource correct by construction rather than
	// by accident if a future nested attribute is added.
	var state DiscountResourceModel
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &state.ID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	discount, err := r.client.GetDiscount(ctx, state.ID.ValueString())
	if err != nil {
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Paddle discount", err.Error())
		return
	}

	resp.Diagnostics.Append(fromAPIDiscount(ctx, *discount, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *DiscountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DiscountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state DiscountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiDiscount, diags := toAPIDiscount(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateDiscount(ctx, state.ID.ValueString(), apiDiscount)
	if err != nil {
		resp.Diagnostics.AddError("Error updating Paddle discount", err.Error())
		return
	}

	resp.Diagnostics.Append(fromAPIDiscount(ctx, *updated, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DiscountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DiscountResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.ArchiveDiscount(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error archiving Paddle discount", err.Error())
	}
}

func (r *DiscountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
