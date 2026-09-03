package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

var _ datasource.DataSource = &TransactionDataSource{}

func NewTransactionDataSource() datasource.DataSource {
	return &TransactionDataSource{}
}

type TransactionDataSource struct {
	client *client.Client
}

type TransactionLineItemModel struct {
	ItemID   types.String `tfsdk:"item_id"`
	PriceID  types.String `tfsdk:"price_id"`
	Quantity types.Int64  `tfsdk:"quantity"`
}

type TransactionDataSourceModel struct {
	ID             types.String               `tfsdk:"id"`
	SubscriptionID types.String               `tfsdk:"subscription_id"`
	CustomerID     types.String               `tfsdk:"customer_id"`
	Status         types.String               `tfsdk:"status"`
	Origin         types.String               `tfsdk:"origin"`
	LineItems      []TransactionLineItemModel `tfsdk:"line_items"`
}

// transactionStatuses is Paddle's real transaction status enum,
// confirmed in docs/facts/0006-subscription-transaction-events-notifications-api-shapes.md.
var transactionStatuses = []string{"draft", "ready", "billed", "paid", "completed", "canceled", "past_due"}

func (d *TransactionDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_transaction"
}

func (d *TransactionDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing Paddle transaction, either directly by `id` or by " +
			"`subscription_id`/`customer_id`/`status` filters — closes the discovery gap " +
			"`paddle_adjustment` otherwise has: that action needs both a `transaction_id` and an " +
			"`item_id`, and `item_id` in particular lives three JSON shapes deep in Paddle's raw API " +
			"(see `internal/client/lineitem.go`'s doc comment). `line_items` here surfaces exactly the " +
			"`item_id`/`price_id` pairs `paddle_adjustment`'s config can be built from directly. See " +
			"[Paddle API Reference](https://developer.paddle.com/api-reference/transactions/overview). If `id` is set, every " +
			"other filter is ignored and that transaction is fetched directly. Otherwise, filters are " +
			"applied server-side and exactly one transaction must match — zero or more than one match " +
			"is an error, not a silent first-result pick.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Paddle transaction ID (prefix `txn_...`) to look up directly. Leave unset to look up by the other filters instead.",
			},
			"subscription_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Filter by the owning subscription's ID (prefix `sub_...`). Ignored if `id` is set.",
			},
			"customer_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Filter by the owning customer's ID (prefix `ctm_...`). Ignored if `id` is set.",
			},
			"status": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.OneOf(transactionStatuses...),
				},
				MarkdownDescription: "Filter by status. Ignored if `id` is set. Also returned (computed) with the matched transaction's actual status.",
			},
			"origin": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "How this transaction was created (e.g., `subscription_charge`, `subscription_recurring`, `web`, `api`).",
			},
			"line_items": schema.ListNestedAttribute{
				Computed: true,
				MarkdownDescription: "This transaction's billed line items, from Paddle's `details.line_items` " +
					"(a fully-calculated breakdown, distinct from the transaction's own top-level `items` " +
					"field) — `item_id` here is the exact value `paddle_adjustment`'s `item_id` config expects.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"item_id":  schema.StringAttribute{Computed: true, MarkdownDescription: "Paddle transaction item ID (prefix `txnitm_...`)."},
						"price_id": schema.StringAttribute{Computed: true, MarkdownDescription: "Paddle price ID (prefix `pri_...`) for this line item."},
						"quantity": schema.Int64Attribute{Computed: true, MarkdownDescription: "Quantity of this line item."},
					},
				},
			},
		},
	}
}

func (d *TransactionDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "data source")
	resp.Diagnostics.Append(diags...)
	d.client = c
}

func fromAPITransaction(txn client.Transaction, m *TransactionDataSourceModel) {
	m.ID = types.StringValue(txn.ID)
	m.SubscriptionID = types.StringValue(txn.SubscriptionID)
	m.CustomerID = types.StringValue(txn.CustomerID)
	m.Status = types.StringValue(txn.Status)
	m.Origin = types.StringValue(txn.Origin)

	resolved := client.ResolveLineItems(&txn)
	m.LineItems = make([]TransactionLineItemModel, 0, len(resolved))
	for _, li := range resolved {
		m.LineItems = append(m.LineItems, TransactionLineItemModel{
			ItemID:   types.StringValue(li.ItemID),
			PriceID:  types.StringValue(li.PriceID),
			Quantity: types.Int64Value(li.Quantity),
		})
	}
}

func (d *TransactionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config TransactionDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !config.ID.IsNull() && !config.ID.IsUnknown() && config.ID.ValueString() != "" {
		txn, err := d.client.GetTransaction(ctx, config.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error reading Paddle transaction", client.FriendlyErrorMessage(err))
			return
		}
		fromAPITransaction(*txn, &config)
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
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

	if transactionFilterEmpty("", filter.SubscriptionID, filter.CustomerID, filter.Status) {
		resp.Diagnostics.AddError(
			"Missing lookup key",
			"Set id, or at least one of subscription_id/customer_id/status, to look up a Paddle transaction.",
		)
		return
	}

	// Limit 2 — see subscription_data_source.go's identical comment.
	filter.Limit = 2
	txns, err := d.client.ListTransactionsFiltered(ctx, filter)
	if err != nil {
		resp.Diagnostics.AddError("Error listing Paddle transactions", client.FriendlyErrorMessage(err))
		return
	}
	switch len(txns) {
	case 0:
		resp.Diagnostics.AddError(
			"No matching Paddle transaction",
			"No transaction matched the given subscription_id/customer_id/status filters. Narrow or correct your filter.",
		)
		return
	case 1:
		// The list response doesn't carry details.line_items — re-fetch
		// by ID to get it, same reasoning GetTransaction's own comment
		// gives for the sweeper's per-transaction re-fetch: not assumed
		// present on whatever list call found this transaction.
		full, err := d.client.GetTransaction(ctx, txns[0].ID)
		if err != nil {
			resp.Diagnostics.AddError("Error reading Paddle transaction", client.FriendlyErrorMessage(err))
			return
		}
		fromAPITransaction(*full, &config)
		resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
	default:
		resp.Diagnostics.AddError(
			"Multiple matching Paddle transactions",
			fmt.Sprintf("%d transactions matched the given subscription_id/customer_id/status filters — narrow your filter (or set id directly) so exactly one matches.", len(txns)),
		)
	}
}
