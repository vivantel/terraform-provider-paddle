package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
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
				MarkdownDescription: "Paddle product ID (`pro_...`) this price belongs to.",
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
				Attributes: map[string]schema.Attribute{
					"minimum": schema.Int64Attribute{
						Required: true,
					},
					"maximum": schema.Int64Attribute{
						Required: true,
					},
				},
			},
		},
	}
}

func (r *PriceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func toAPIPrice(m PriceResourceModel) client.Price {
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
	if m.Quantity != nil && !m.Quantity.Minimum.IsUnknown() {
		p.Quantity = &client.Quantity{
			Minimum: m.Quantity.Minimum.ValueInt64(),
			Maximum: m.Quantity.Maximum.ValueInt64(),
		}
	}
	return p
}

func fromAPIPrice(p client.Price, m *PriceResourceModel) {
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
	}
}

func (r *PriceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PriceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreatePrice(ctx, toAPIPrice(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Paddle price", err.Error())
		return
	}

	fromAPIPrice(*created, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PriceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PriceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	price, err := r.client.GetPrice(ctx, state.ID.ValueString())
	if err != nil {
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Paddle price", err.Error())
		return
	}

	fromAPIPrice(*price, &state)
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

	updated, err := r.client.UpdatePrice(ctx, state.ID.ValueString(), toAPIPrice(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating Paddle price", err.Error())
		return
	}

	fromAPIPrice(*updated, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PriceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PriceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.ArchivePrice(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error archiving Paddle price", err.Error())
	}
}

func (r *PriceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
