package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// configureClient extracts the *client.Client from ProviderData, the same
// boilerplate every resource's and data source's Configure method needs.
// Was previously copy-pasted 6 times (product/price/discount resources and
// data sources); the error message wording had already drifted between the
// resource and data source copies ("resource configure type" vs "data
// source configure type") purely from hand-copying, not any functional
// difference — kind identifies which in the one shared message now.
func configureClient(providerData any, kind string) (*client.Client, diag.Diagnostics) {
	var diags diag.Diagnostics
	if providerData == nil {
		return nil, diags
	}
	c, ok := providerData.(*client.Client)
	if !ok {
		diags.AddError(
			fmt.Sprintf("Unexpected %s configure type", kind),
			fmt.Sprintf("expected *client.Client, got %T", providerData),
		)
		return nil, diags
	}
	return c, diags
}
