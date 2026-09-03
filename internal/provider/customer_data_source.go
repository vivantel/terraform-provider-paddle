package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

var _ datasource.DataSource = &CustomerDataSource{}

func NewCustomerDataSource() datasource.DataSource {
	return &CustomerDataSource{}
}

type CustomerDataSource struct {
	client *client.Client
}

type CustomerDataSourceModel struct {
	ID     types.String `tfsdk:"id"`
	Email  types.String `tfsdk:"email"`
	Name   types.String `tfsdk:"name"`
	Status types.String `tfsdk:"status"`
}

func (d *CustomerDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_customer"
}

func (d *CustomerDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// PII warning follows
		// docs/guardrails/pii-bearing-data-sources-need-state-security-warning.md
		// exactly — same posture README.md's Actions section already
		// takes for financial risk (⚠️ callout, concrete consequence,
		// concrete mitigation), applied to PII instead. Don't shorten or
		// soften this; the guardrail exists because "read-only" is a
		// common, wrong assumption that this concern doesn't apply.
		MarkdownDescription: "Look up an existing Paddle customer, either directly by `id` or by `email` " +
			"(exact match) — closes the discovery gap of finding a subscription's or transaction's owning " +
			"customer from inside Terraform. See [Paddle API Reference](https://developer.paddle.com/api-reference/customers/overview).\n\n" +
			"**⚠️ This data source exposes customer PII (email, name) and writes it into your Terraform " +
			"state file.** Terraform persists every data source read into state, in plaintext by default, " +
			"exactly as durably as a resource's state — not just once, but on every `plan`/`refresh` this " +
			"data source is used in. \"Read-only\" does not make this go away: a data source's state write " +
			"happens on every refresh where a resource's happens once at apply-time, so using " +
			"`paddle_customer` puts real customer PII into your state file repeatedly, not incidentally. " +
			"Treat any state file that uses this data source as sensitive — an encrypted, access-controlled " +
			"remote backend, not local state or an unencrypted bucket — the same recommendation this " +
			"provider's Actions section gives for financial risk, applied here to data exposure instead. " +
			"There is no `paddle_address` data source or resource in this provider — email/name alone " +
			"resolves the actual discovery gap without pulling in postal-address PII too.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Paddle customer ID (prefix `ctm_...`) to look up directly. Leave unset to look up by `email` instead.",
			},
			"email": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Filter by email address (exact match — Paddle's `/customers` API does not support partial/fuzzy email matching here). Ignored if `id` is set.",
			},
			"name":   schema.StringAttribute{Computed: true, MarkdownDescription: "Customer's full name."},
			"status": schema.StringAttribute{Computed: true, MarkdownDescription: "Customer status: `active` or `archived`."},
		},
	}
}

func (d *CustomerDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "data source")
	resp.Diagnostics.Append(diags...)
	d.client = c
}

func fromAPICustomer(cust client.Customer, m *CustomerDataSourceModel) {
	m.ID = types.StringValue(cust.ID)
	m.Email = types.StringValue(cust.Email)
	m.Name = types.StringValue(cust.Name)
	m.Status = types.StringValue(cust.Status)
}

func (d *CustomerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config CustomerDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !config.ID.IsNull() && !config.ID.IsUnknown() && config.ID.ValueString() != "" {
		cust, err := d.client.GetCustomer(ctx, config.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error reading Paddle customer", client.FriendlyErrorMessage(err))
			return
		}
		fromAPICustomer(*cust, &config)
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
		return
	}

	if config.Email.IsNull() || config.Email.IsUnknown() || config.Email.ValueString() == "" {
		resp.Diagnostics.AddError(
			"Missing lookup key",
			"Set either id or email to look up a Paddle customer.",
		)
		return
	}

	custs, err := d.client.ListCustomersByEmail(ctx, config.Email.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error listing Paddle customers", client.FriendlyErrorMessage(err))
		return
	}
	switch len(custs) {
	case 0:
		resp.Diagnostics.AddError(
			"No matching Paddle customer",
			"No customer matched the given email. Confirm the address is correct (this is an exact match, not partial/fuzzy).",
		)
		return
	case 1:
		fromAPICustomer(custs[0], &config)
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	default:
		resp.Diagnostics.AddError(
			"Multiple matching Paddle customers",
			fmt.Sprintf("%d customers matched email %q — narrow your filter (or set id directly) so exactly one matches.", len(custs), config.Email.ValueString()),
		)
	}
}
