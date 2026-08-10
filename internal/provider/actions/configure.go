// Package actions holds this provider's Terraform Plugin Framework
// actions — one-time, irreversible operations against Paddle entities
// this provider doesn't otherwise manage as resources (Adjustments,
// Subscription lifecycle operations). See
// docs/decisions/0010-v3-scope-lifecycle-actions.md for why actions,
// rather than resources, are the right shape for these, and
// docs/guardrails/money-moving-actions-no-blanket-retry.md for the
// no-blind-retry / search-before-invoke requirements every action here
// follows.
package actions

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// configureClient extracts the *client.Client from an action's
// ConfigureRequest.ProviderData. Deliberately duplicated from
// internal/provider/configure.go's identically-named helper rather than
// exported and shared: provider.go must import this actions package to
// register the actions Actions() returns, so this package importing back
// to provider for a shared helper would be a cycle. Small enough (a
// handful of lines) that duplicating once for a genuine package boundary
// is the right call, not a regression of the reasoning that led to
// extracting the original after six copy-pasted, hand-drifted in-package
// copies (docs/plans/paddle-provider-v1.md).
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
