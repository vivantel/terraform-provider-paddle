package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

var _ list.ListResource = &PriceListResource{}
var _ list.ListResourceWithConfigure = &PriceListResource{}

func NewPriceListResource() list.ListResource {
	return &PriceListResource{}
}

// PriceListResource is paddle_price's `list` block counterpart (`terraform
// query`, Terraform 1.14+) — see product_list_resource.go's comment for what
// this is and why it needs identity on the resource first. Every result
// carries an Identity (required by the framework) built from that same
// schema, plus the full resource data when a query asks for it
// (`include_resource = true`).
type PriceListResource struct {
	client *client.Client
}

func (r *PriceListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	// Must match PriceResource.Metadata's TypeName exactly — the framework
	// errors on GetMetadata otherwise (list.ListResource's own doc comment
	// on Metadata).
	resp.TypeName = req.ProviderTypeName + "_price"
}

func (r *PriceListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		MarkdownDescription: "Lists every `paddle_price` in the account — Paddle's list-prices endpoint takes no filters, so this block's config has nothing to set beyond the standard `include_resource`/`limit` query options.",
	}
}

func (r *PriceListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "list resource")
	resp.Diagnostics.Append(diags...)
	r.client = c
}

func (r *PriceListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	prices, err := r.client.ListPrices(ctx)
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError("Error listing Paddle prices", client.FriendlyErrorMessage(err))
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, p := range prices {
			result := req.NewListResult(ctx)
			// Prices have no top-level "name" the way products do —
			// Description is the closest human-readable field Paddle
			// gives back (Name is optional/customer-facing and often
			// unset), so it's the most useful DisplayName available.
			result.DisplayName = p.Description
			result.Diagnostics.Append(result.Identity.SetAttribute(ctx, path.Root("id"), p.ID)...)

			if req.IncludeResource {
				// Same reasoning as product_list_resource.go's List: the
				// Resource object's shape is derived from PriceResource's
				// real schema (timeouts included), and a listed price was
				// never actually configured through Terraform, so
				// nullTimeouts() is the correctly typed way to say "none".
				m := priceResourceStateModel{Timeouts: nullTimeouts()}
				if err := fromAPIPrice(p, &m.PriceResourceModel); err != nil {
					result.Diagnostics.AddError("Error decoding Paddle price response", err.Error())
				} else {
					result.Diagnostics.Append(result.Resource.Set(ctx, &m)...)
				}
			}

			if !push(result) {
				return
			}
		}
	}
}
