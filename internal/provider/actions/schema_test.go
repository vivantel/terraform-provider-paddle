package actions

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
)

// TestActions_MetadataAndSchemaBuildWithoutError is a cheap, network-free
// smoke check: every action's Metadata/Schema methods run without
// panicking or producing diagnostics, and TypeName follows the
// provider_verb naming convention. Doesn't replace real acceptance
// testing against the sandbox (Schema() can be internally valid and still
// mismatch what Paddle's actual API accepts) — it only catches wiring
// bugs (a nil map, a validator constructed wrong) before they'd otherwise
// only surface via `terraform plan`/TF_ACC, which this environment
// couldn't run this session (docs/plans/paddle-provider-v3.md's Step
// 0-4 status notes).
func TestActions_MetadataAndSchemaBuildWithoutError(t *testing.T) {
	newActions := map[string]func() action.Action{
		"paddle_adjustment":          NewAdjustmentAction,
		"paddle_subscription_cancel": NewSubscriptionCancelAction,
		"paddle_subscription_pause":  NewSubscriptionPauseAction,
		"paddle_subscription_resume": NewSubscriptionResumeAction,
		"paddle_subscription_charge": NewSubscriptionChargeAction,
	}

	for wantTypeName, newAction := range newActions {
		t.Run(wantTypeName, func(t *testing.T) {
			a := newAction()

			var metaResp action.MetadataResponse
			a.Metadata(context.Background(), action.MetadataRequest{ProviderTypeName: "paddle"}, &metaResp)
			if metaResp.TypeName != wantTypeName {
				t.Errorf("TypeName = %q, want %q", metaResp.TypeName, wantTypeName)
			}

			var schemaResp action.SchemaResponse
			a.Schema(context.Background(), action.SchemaRequest{}, &schemaResp)
			if schemaResp.Diagnostics.HasError() {
				t.Errorf("Schema() diagnostics: %v", schemaResp.Diagnostics)
			}
			if len(schemaResp.Schema.Attributes) == 0 {
				t.Error("Schema().Attributes is empty, want at least one attribute")
			}
		})
	}
}
