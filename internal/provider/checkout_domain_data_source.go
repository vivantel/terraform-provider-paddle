package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

var _ datasource.DataSource = &CheckoutDomainDataSource{}

func NewCheckoutDomainDataSource() datasource.DataSource {
	return &CheckoutDomainDataSource{}
}

type CheckoutDomainDataSource struct {
	client *client.Client
}

type applePayVerificationModel struct {
	Status types.String `tfsdk:"status"`
}

type paymentMethodVerificationModel struct {
	ApplePay applePayVerificationModel `tfsdk:"apple_pay"`
}

type CheckoutDomainDataSourceModel struct {
	ID                        types.String                   `tfsdk:"id"`
	Domain                    types.String                   `tfsdk:"domain"`
	Status                    types.String                   `tfsdk:"status"`
	PaymentMethodVerification paymentMethodVerificationModel `tfsdk:"payment_method_verification"`
	CreatedAt                 types.String                   `tfsdk:"created_at"`
	UpdatedAt                 types.String                   `tfsdk:"updated_at"`
}

func (d *CheckoutDomainDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_checkout_domain"
}

func (d *CheckoutDomainDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing Paddle checkout domain by ID. See " +
			"[Paddle API Reference](https://developer.paddle.com/api-reference/checkout-domains/overview). There is no matching " +
			"`paddle_checkout_domain` resource: Paddle's API has no create or update operation for this " +
			"entity at all — a domain can only be added via the dashboard (Paddle > Checkout > Website " +
			"approval > Domain approval), confirmed against the real API reference rather than assumed. " +
			"This data source is read-only lookup for a domain approved that way.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Paddle checkout domain ID (prefix `chedom_...`) to look up.",
			},
			"domain": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The domain name (FQDN), e.g., `checkout.example.com`.",
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Domain status: `pending_review`, `in_review`, `approved`, `rejected`, or `action_required`.",
			},
			"payment_method_verification": schema.SingleNestedAttribute{
				Computed: true,
				Attributes: map[string]schema.Attribute{
					"apple_pay": schema.SingleNestedAttribute{
						Computed: true,
						Attributes: map[string]schema.Attribute{
							"status": schema.StringAttribute{
								Computed:            true,
								MarkdownDescription: "Apple Pay verification status: `verified` or `unverified`.",
							},
						},
					},
				},
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "RFC 3339 date-time this domain was created, set by Paddle.",
			},
			"updated_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "RFC 3339 date-time this domain was last updated, set by Paddle.",
			},
		},
	}
}

func (d *CheckoutDomainDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "data source")
	resp.Diagnostics.Append(diags...)
	d.client = c
}

func fromAPICheckoutDomain(cd client.CheckoutDomain, m *CheckoutDomainDataSourceModel) {
	m.ID = types.StringValue(cd.ID)
	m.Domain = types.StringValue(cd.Domain)
	m.Status = types.StringValue(cd.Status)
	m.PaymentMethodVerification = paymentMethodVerificationModel{
		ApplePay: applePayVerificationModel{
			Status: types.StringValue(cd.PaymentMethodVerification.ApplePay.Status),
		},
	}
	m.CreatedAt = types.StringValue(cd.CreatedAt)
	m.UpdatedAt = types.StringValue(cd.UpdatedAt)
}

func (d *CheckoutDomainDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	// Fetch just id, not the whole model — same reasoning as every other
	// data source in this provider: this model has Required non-pointer
	// nested struct fields (payment_method_verification and its nested
	// apple_pay), so a full req.Config.Get here would hit the exact
	// null-into-non-pointer-struct crash price_resource.go's Read()
	// comment documents, the very first time this data source is used
	// (config only ever sets id — payment_method_verification is
	// Computed-only, genuinely null in config).
	var config CheckoutDomainDataSourceModel
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("id"), &config.ID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domain, err := d.client.GetCheckoutDomain(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading Paddle checkout domain", client.FriendlyErrorMessage(err))
		return
	}

	fromAPICheckoutDomain(*domain, &config)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
