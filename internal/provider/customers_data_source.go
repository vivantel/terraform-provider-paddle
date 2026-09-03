package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

var _ datasource.DataSource = &CustomersDataSource{}

func NewCustomersDataSource() datasource.DataSource {
	return &CustomersDataSource{}
}

type CustomersDataSource struct {
	client *client.Client
}

type CustomersDataSourceModel struct {
	Email     types.String              `tfsdk:"email"`
	Status    types.String              `tfsdk:"status"`
	Customers []CustomerDataSourceModel `tfsdk:"customers"`
}

func (d *CustomersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_customers"
}

func (d *CustomersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// PII warning follows
		// docs/guardrails/pii-bearing-data-sources-need-state-security-warning.md's
		// "plural/list variants compound this, not just repeat it" treatment
		// — this returns *multiple* customers' PII per use, not a
		// copy-pasted singular-lookup sentence. Don't shorten or soften this.
		MarkdownDescription: "List every Paddle customer matching `email`/`status` filters — the plural " +
			"companion to `paddle_customer` (which requires exactly one match, or resolves by `id` " +
			"directly). Leave both filters unset to list every customer in the account. See " +
			"[Paddle API Reference](https://developer.paddle.com/api-reference/customers/overview).\n\n" +
			"**⚠️ This data source exposes multiple customers' PII (email, name) at once and writes it " +
			"into your Terraform state file.** It compounds the same risk `paddle_customer` (singular) " +
			"carries — Terraform persists every data source read into state, in plaintext by default, on " +
			"every `plan`/`refresh` — except here, every use writes *however many customers matched*, not " +
			"just one. An unfiltered (or loosely filtered) call lists every customer in the account into " +
			"state at once. Treat any state file that uses `paddle_customers` as sensitive — an encrypted, " +
			"access-controlled remote backend, not local state or an unencrypted bucket — the same " +
			"recommendation this provider's Actions section and `paddle_customer`'s own warning give, " +
			"applied here at a larger scale. Narrow with `email`/`status` where practical, both to reduce " +
			"the amount of PII persisted and the cost of an unfiltered account-wide list.\n\n" +
			"There is no `paddle_address` data source or resource in this provider — email/name alone " +
			"resolves the actual discovery gap without pulling in postal-address PII too.",
		Attributes: map[string]schema.Attribute{
			"email": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Filter by email address (exact match — Paddle's `/customers` API does not support partial/fuzzy email matching here). Leave unset to match any email.",
			},
			"status": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Filter by status: `active` or `archived`. Leave unset to match any status.",
			},
			"customers": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Matching customers.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":     schema.StringAttribute{Computed: true, MarkdownDescription: "Paddle customer ID (prefix `ctm_...`)."},
						"email":  schema.StringAttribute{Computed: true, MarkdownDescription: "Customer's email address."},
						"name":   schema.StringAttribute{Computed: true, MarkdownDescription: "Customer's full name."},
						"status": schema.StringAttribute{Computed: true, MarkdownDescription: "Customer status: `active` or `archived`."},
					},
				},
			},
		},
	}
}

func (d *CustomersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "data source")
	resp.Diagnostics.Append(diags...)
	d.client = c
}

func (d *CustomersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config CustomersDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	filter := client.CustomerListFilter{}
	if !config.Email.IsNull() && !config.Email.IsUnknown() {
		filter.Email = config.Email.ValueString()
	}
	if !config.Status.IsNull() && !config.Status.IsUnknown() {
		filter.Status = config.Status.ValueString()
	}
	// Limit: 0 (unlimited) — see subscriptions_data_source.go's identical
	// comment on why this data source has no "no filter set" guard. The
	// PII-compounding concern this data source carries is addressed by
	// documentation (the warning above), not by a technical filter — the
	// same posture the guardrail specifies.

	custs, err := d.client.ListCustomersFiltered(ctx, filter)
	if err != nil {
		resp.Diagnostics.AddError("Error listing Paddle customers", client.FriendlyErrorMessage(err))
		return
	}

	config.Customers = make([]CustomerDataSourceModel, 0, len(custs))
	for _, c := range custs {
		var m CustomerDataSourceModel
		fromAPICustomer(c, &m)
		config.Customers = append(config.Customers, m)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
