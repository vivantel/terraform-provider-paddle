package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

var _ resource.Resource = &PriceResource{}
var _ resource.ResourceWithImportState = &PriceResource{}

func NewPriceResource() resource.Resource {
	return &PriceResource{}
}

type PriceResource struct {
	client *client.Client
}

type unitPriceModel struct {
	Amount       types.String `tfsdk:"amount"`
	CurrencyCode types.String `tfsdk:"currency_code"`
}

type billingCycleModel struct {
	Interval  types.String `tfsdk:"interval"`
	Frequency types.Int64  `tfsdk:"frequency"`
}

type quantityModel struct {
	Minimum types.Int64 `tfsdk:"minimum"`
	Maximum types.Int64 `tfsdk:"maximum"`
}

type PriceResourceModel struct {
	ID           types.String       `tfsdk:"id"`
	ProductID    types.String       `tfsdk:"product_id"`
	Description  types.String       `tfsdk:"description"`
	UnitPrice    unitPriceModel     `tfsdk:"unit_price"`
	Name         types.String       `tfsdk:"name"`
	BillingCycle *billingCycleModel `tfsdk:"billing_cycle"`
	Quantity     *quantityModel     `tfsdk:"quantity"`
	TaxMode      types.String       `tfsdk:"tax_mode"`
	Status       types.String       `tfsdk:"status"`
	CustomData   types.String       `tfsdk:"custom_data"`
}

func (r *PriceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_price"
}

func (r *PriceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Paddle price — see https://developer.paddle.com/api-reference/prices/overview. Paddle has no hard delete for prices; `terraform destroy` archives it instead (status becomes `archived`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Paddle price ID (`pri_...`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"product_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Paddle product ID (`pro_...`) this price belongs to. Changing this replaces the price — Paddle prices aren't reparented in place.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"description": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "2-500 characters. Internal only, never shown to customers.",
			},
			"name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "1-150 characters. Customer-facing.",
			},
			"tax_mode": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "`account_setting` (default), `external`, `internal`, or `location`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Validators: []validator.String{
					stringvalidator.OneOf("account_setting", "external", "internal", "location"),
				},
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "`active` or `archived`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"unit_price": schema.SingleNestedAttribute{
				Required: true,
				Attributes: map[string]schema.Attribute{
					"amount": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "Lowest denomination as a string, e.g. \"1000\" = $10.00 for a 2-decimal currency.",
					},
					"currency_code": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "ISO 4217 code, e.g. USD.",
					},
				},
			},
			"billing_cycle": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Omit for a one-time price.",
				Attributes: map[string]schema.Attribute{
					"interval": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "day, week, month, or year.",
					},
					"frequency": schema.Int64Attribute{
						Required: true,
					},
				},
			},
			"quantity": schema.SingleNestedAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Defaults to 1-100 if omitted.",
				PlanModifiers:       []planmodifier.Object{objectplanmodifier.UseStateForUnknown()},
				// Required so this is never Unknown on Create: an
				// Optional+Computed nested attribute with nothing in
				// config plans as Unknown when there's no prior state to
				// draw UseStateForUnknown from, and *quantityModel (a
				// plain struct pointer) can't represent Unknown — that
				// combination crashed the very first real sandbox apply
				// with "Value Conversion Error: target type cannot handle
				// unknown values". A static default matching Paddle's own
				// documented 1-100 default means it's always a known
				// value by the time Create() decodes the plan.
				Default: objectdefault.StaticValue(types.ObjectValueMust(
					map[string]attr.Type{
						"minimum": types.Int64Type,
						"maximum": types.Int64Type,
					},
					map[string]attr.Value{
						"minimum": types.Int64Value(1),
						"maximum": types.Int64Value(100),
					},
				)),
				Attributes: map[string]schema.Attribute{
					"minimum": schema.Int64Attribute{
						Required: true,
					},
					"maximum": schema.Int64Attribute{
						Required: true,
					},
				},
			},
			"custom_data": customDataAttribute(),
		},
	}
}

func (r *PriceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "resource")
	resp.Diagnostics.Append(diags...)
	r.client = c
}

func toAPIPrice(m PriceResourceModel) (client.Price, error) {
	p := client.Price{
		ProductID:   m.ProductID.ValueString(),
		Description: m.Description.ValueString(),
		UnitPrice: client.Money{
			Amount:       m.UnitPrice.Amount.ValueString(),
			CurrencyCode: m.UnitPrice.CurrencyCode.ValueString(),
		},
	}
	if !m.Name.IsNull() {
		v := m.Name.ValueString()
		p.Name = &v
	}
	if !m.TaxMode.IsNull() && !m.TaxMode.IsUnknown() {
		p.TaxMode = m.TaxMode.ValueString()
	}
	if m.BillingCycle != nil {
		p.BillingCycle = &client.BillingCycle{
			Interval:  m.BillingCycle.Interval.ValueString(),
			Frequency: m.BillingCycle.Frequency.ValueInt64(),
		}
	}
	// Minimum and Maximum are only ever unknown/known together in practice
	// (both come from the same nested "quantity" object), but check both
	// explicitly rather than assuming it: if only one were somehow unknown,
	// ValueInt64() on the other would silently send 0 instead of the real
	// bound.
	if m.Quantity != nil && !m.Quantity.Minimum.IsUnknown() && !m.Quantity.Maximum.IsUnknown() {
		p.Quantity = &client.Quantity{
			Minimum: m.Quantity.Minimum.ValueInt64(),
			Maximum: m.Quantity.Maximum.ValueInt64(),
		}
	}
	customData, err := customDataToAPI(m.CustomData)
	if err != nil {
		return client.Price{}, err
	}
	p.CustomData = customData
	return p, nil
}

// toAPIPriceUpdate builds the PATCH body for updating an existing price.
// It reuses toAPIPrice for the fields they share, then drops ProductID —
// Paddle's price-update endpoint rejects the field outright if present at
// all (see client.PriceUpdate), and product_id is RequiresReplace in the
// schema anyway, so it can never legitimately differ from what's already
// on the price.
func toAPIPriceUpdate(m PriceResourceModel) (client.PriceUpdate, error) {
	full, err := toAPIPrice(m)
	if err != nil {
		return client.PriceUpdate{}, err
	}
	return client.PriceUpdate{
		Description:  full.Description,
		UnitPrice:    full.UnitPrice,
		Type:         full.Type,
		Name:         full.Name,
		BillingCycle: full.BillingCycle,
		Quantity:     full.Quantity,
		TaxMode:      full.TaxMode,
		CustomData:   full.CustomData,
		Status:       full.Status,
	}, nil
}

func fromAPIPrice(p client.Price, m *PriceResourceModel) error {
	m.ID = types.StringValue(p.ID)
	m.ProductID = types.StringValue(p.ProductID)
	m.Description = types.StringValue(p.Description)
	m.UnitPrice = unitPriceModel{
		Amount:       types.StringValue(p.UnitPrice.Amount),
		CurrencyCode: types.StringValue(p.UnitPrice.CurrencyCode),
	}
	if p.Name != nil {
		m.Name = types.StringValue(*p.Name)
	} else {
		m.Name = types.StringNull()
	}
	m.TaxMode = types.StringValue(p.TaxMode)
	m.Status = types.StringValue(p.Status)
	if p.BillingCycle != nil {
		m.BillingCycle = &billingCycleModel{
			Interval:  types.StringValue(p.BillingCycle.Interval),
			Frequency: types.Int64Value(p.BillingCycle.Frequency),
		}
	} else {
		m.BillingCycle = nil
	}
	if p.Quantity != nil {
		m.Quantity = &quantityModel{
			Minimum: types.Int64Value(p.Quantity.Minimum),
			Maximum: types.Int64Value(p.Quantity.Maximum),
		}
	} else {
		m.Quantity = nil
	}
	customData, err := customDataFromAPI(p.CustomData)
	if err != nil {
		return err
	}
	m.CustomData = customData
	return nil
}

func (r *PriceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PriceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiPrice, err := toAPIPrice(plan)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("custom_data"), "Invalid custom_data", err.Error())
		return
	}

	created, err := r.client.CreatePrice(ctx, apiPrice)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Paddle price", client.FriendlyErrorMessage(err))
		return
	}

	if err := fromAPIPrice(*created, &plan); err != nil {
		resp.Diagnostics.AddError("Error decoding Paddle price response", client.FriendlyErrorMessage(err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PriceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Fetch just the id attribute rather than decoding the whole model via
	// req.State.Get: right after ImportStatePassthroughID (which sets only
	// id, leaving every other attribute genuinely null) the framework
	// calls this Read to fill in the rest, and a full Get() at that point
	// tries to decode that null unit_price into unitPriceModel — a plain,
	// non-pointer struct that (unlike types.String) can't represent null,
	// which crashed the real sandbox import test with "Value Conversion
	// Error: target type cannot handle null values, Path: unit_price".
	// id is all this method needs before fromAPIPrice overwrites the rest
	// of the model wholesale below.
	var state PriceResourceModel
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &state.ID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	price, err := r.client.GetPrice(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Paddle price", client.FriendlyErrorMessage(err))
		return
	}

	if err := fromAPIPrice(*price, &state); err != nil {
		resp.Diagnostics.AddError("Error decoding Paddle price response", client.FriendlyErrorMessage(err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *PriceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan PriceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state PriceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiPriceUpdate, err := toAPIPriceUpdate(plan)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("custom_data"), "Invalid custom_data", err.Error())
		return
	}

	updated, err := r.client.UpdatePrice(ctx, state.ID.ValueString(), apiPriceUpdate)
	if err != nil {
		resp.Diagnostics.AddError("Error updating Paddle price", client.FriendlyErrorMessage(err))
		return
	}

	if err := fromAPIPrice(*updated, &plan); err != nil {
		resp.Diagnostics.AddError("Error decoding Paddle price response", client.FriendlyErrorMessage(err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PriceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PriceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// A 404 means the price is already gone — successful destroy, not an
	// error. Same tolerance Read() already has for the same status.
	if err := r.client.ArchivePrice(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error archiving Paddle price", client.FriendlyErrorMessage(err))
	}
}

func (r *PriceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
