package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

var _ datasource.DataSource = &TransactionsDataSource{}

func NewTransactionsDataSource() datasource.DataSource {
	return &TransactionsDataSource{}
}

type TransactionsDataSource struct {
	client *client.Client
}

// TransactionSummaryModel deliberately excludes line_items —
// paddle_transaction (singular)'s line_items requires a per-ID re-fetch
// (see transaction_data_source.go's fromAPITransaction comment: the list
// response doesn't carry details.line_items at all) — doing that for
// every result here would mean an N+1 API call per match, a real cost for
// a data source whose whole point can be "list everything." Use
// paddle_transactions to find the transaction ID you need, then
// paddle_transaction (singular, by id) to get its line_items for feeding
// into paddle_adjustment — the same two-step discovery pattern this
// provider already uses elsewhere (see examples/lookup-then-act).
type TransactionSummaryModel struct {
	ID             types.String `tfsdk:"id"`
	SubscriptionID types.String `tfsdk:"subscription_id"`
	CustomerID     types.String `tfsdk:"customer_id"`
	Status         types.String `tfsdk:"status"`
	Origin         types.String `tfsdk:"origin"`
}

type TransactionsDataSourceModel struct {
	SubscriptionID types.String              `tfsdk:"subscription_id"`
	CustomerID     types.String              `tfsdk:"customer_id"`
	Status         types.String              `tfsdk:"status"`
	Transactions   []TransactionSummaryModel `tfsdk:"transactions"`
}

func (d *TransactionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_transactions"
}

func (d *TransactionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "List every Paddle transaction matching `subscription_id`/`customer_id`/`status` " +
			"filters — the plural companion to `paddle_transaction` (which requires exactly one match). " +
			"Leave all filters unset to list every transaction in the account. Deliberately excludes " +
			"`line_items` — unlike the singular data source, which re-fetches each match by ID to get it, " +
			"doing that for every result here would be an N+1 API call per match; look up a transaction's " +
			"`id` here, then feed it into `paddle_transaction` (singular) to get `line_items` for " +
			"`paddle_adjustment`. See https://developer.paddle.com/api-reference/transactions/overview.\n\n" +
			"**⚠️ An unfiltered (or loosely filtered) call to this data source lists every matching " +
			"transaction in the account, one API call per page of results — a real cost against a large " +
			"account, and a large `transactions` list written into your Terraform state file on every " +
			"`plan`/`refresh`.** Narrow with `subscription_id`/`customer_id`/`status` where practical.",
		Attributes: map[string]schema.Attribute{
			"subscription_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Filter by the owning subscription's ID (`sub_...`). Leave unset to match transactions for any subscription.",
			},
			"customer_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Filter by the owning customer's ID (`ctm_...`). Leave unset to match transactions for any customer.",
			},
			"status": schema.StringAttribute{
				Optional: true,
				Validators: []validator.String{
					stringvalidator.OneOf(transactionStatuses...),
				},
				MarkdownDescription: "Filter by status. Leave unset to match any status.",
			},
			"transactions": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Matching transactions (without `line_items` — see above).",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":              schema.StringAttribute{Computed: true, MarkdownDescription: "Paddle transaction ID (`txn_...`)."},
						"subscription_id": schema.StringAttribute{Computed: true},
						"customer_id":     schema.StringAttribute{Computed: true},
						"status":          schema.StringAttribute{Computed: true},
						"origin":          schema.StringAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *TransactionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "data source")
	resp.Diagnostics.Append(diags...)
	d.client = c
}

func fromAPITransactionSummary(txn client.Transaction, m *TransactionSummaryModel) {
	m.ID = types.StringValue(txn.ID)
	m.SubscriptionID = types.StringValue(txn.SubscriptionID)
	m.CustomerID = types.StringValue(txn.CustomerID)
	m.Status = types.StringValue(txn.Status)
	m.Origin = types.StringValue(txn.Origin)
}

func (d *TransactionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config TransactionsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	filter := client.TransactionListFilter{}
	if !config.SubscriptionID.IsNull() && !config.SubscriptionID.IsUnknown() {
		filter.SubscriptionID = config.SubscriptionID.ValueString()
	}
	if !config.CustomerID.IsNull() && !config.CustomerID.IsUnknown() {
		filter.CustomerID = config.CustomerID.ValueString()
	}
	if !config.Status.IsNull() && !config.Status.IsUnknown() {
		filter.Status = config.Status.ValueString()
	}
	// Limit: 0 (unlimited) — see subscriptions_data_source.go's identical
	// comment on why this data source has no "no filter set" guard.

	txns, err := d.client.ListTransactionsFiltered(ctx, filter)
	if err != nil {
		resp.Diagnostics.AddError("Error listing Paddle transactions", client.FriendlyErrorMessage(err))
		return
	}

	config.Transactions = make([]TransactionSummaryModel, 0, len(txns))
	for _, txn := range txns {
		var m TransactionSummaryModel
		fromAPITransactionSummary(txn, &m)
		config.Transactions = append(config.Transactions, m)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
