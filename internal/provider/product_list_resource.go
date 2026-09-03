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

var _ list.ListResource = &ProductListResource{}
var _ list.ListResourceWithConfigure = &ProductListResource{}

func NewProductListResource() list.ListResource {
	return &ProductListResource{}
}

// ProductListResource is paddle_product's `list` block counterpart
// (`terraform query`, Terraform 1.14+) — see product_resource.go's
// IdentitySchema comment for why identity has to exist first. Every result
// carries an Identity (required by the framework) built from that same
// schema, plus the full resource data when a query asks for it
// (`include_resource = true`).
type ProductListResource struct {
	client *client.Client
}

func (r *ProductListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	// Must match ProductResource.Metadata's TypeName exactly — the
	// framework errors on GetMetadata otherwise (list.ListResource's own
	// doc comment on Metadata).
	resp.TypeName = req.ProviderTypeName + "_product"
}

func (r *ProductListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		MarkdownDescription: "Lists every `paddle_product` in the account — Paddle's list-products endpoint takes no filters, so this block's config has nothing to set beyond the standard `include_resource`/`limit` query options.",
	}
}

func (r *ProductListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "list resource")
	resp.Diagnostics.Append(diags...)
	r.client = c
}

func (r *ProductListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	products, err := r.client.ListProducts(ctx)
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError("Error listing Paddle products", client.FriendlyErrorMessage(err))
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, p := range products {
			result := req.NewListResult(ctx)
			result.DisplayName = p.Name
			result.Diagnostics.Append(result.Identity.SetAttribute(ctx, path.Root("id"), p.ID)...)

			if req.IncludeResource {
				// The list result's Resource object shape is derived from
				// ProductResource's real schema — timeouts included — same
				// reason Create/Read/Update decode into
				// productResourceStateModel, not bare ProductResourceModel
				// (see that type's own comment). A listed product was never
				// actually configured through Terraform, so there's no real
				// timeouts value to report; nullTimeouts() is the correctly
				// typed way to say "none" (a bare zero-value timeouts.Value{}
				// fails state encoding — see nullTimeouts's own comment).
				m := productResourceStateModel{Timeouts: nullTimeouts()}
				if err := fromAPIProduct(p, &m.ProductResourceModel); err != nil {
					result.Diagnostics.AddError("Error decoding Paddle product response", err.Error())
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
