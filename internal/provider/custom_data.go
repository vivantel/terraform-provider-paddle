package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// customDataAttribute is the shared schema definition for custom_data
// across paddle_product/paddle_price/paddle_discount — see
// docs/decisions/0008-custom-data-and-enum-validator-retrofit.md. Paddle's
// custom_data is arbitrary structured JSON (confirmed against the API
// reference: nested objects, arrays, booleans, null all appear in real
// examples, not just flat string values), so this is modeled as a
// JSON-encoded string attribute — the same pattern used for "arbitrary
// JSON blob" attributes elsewhere in the Terraform ecosystem (e.g. AWS's
// `policy` attributes) — with a semantic-equality plan modifier so
// key-ordering/whitespace differences between what a user writes and what
// Paddle echoes back don't cause a perpetual diff every plan.
func customDataAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		Optional: true,
		MarkdownDescription: "Arbitrary structured JSON data, e.g. " +
			"`jsonencode({ internal_id = 123 })`. Compared semantically, " +
			"not byte-for-byte — key ordering or whitespace differences " +
			"between what you write and what Paddle echoes back won't " +
			"produce a diff.",
		PlanModifiers: []planmodifier.String{customDataPlanModifier{}},
	}
}

// customDataPlanModifier keeps the prior state value when the planned
// value is JSON-semantically equal to it, so re-serialization differences
// (key order, whitespace) don't cause a permanent diff. Without this, any
// custom_data round-trip through Paddle's API would look like a change on
// every subsequent plan even when nothing was actually modified — the
// exact "diff never settles" bug class this project has hit repeatedly
// with other attributes (see docs/plans/paddle-provider-v1.md's recorded
// Default/UseStateForUnknown lessons), addressed here before it could
// happen instead of after a sandbox crash.
type customDataPlanModifier struct{}

func (m customDataPlanModifier) Description(_ context.Context) string {
	return "Compares custom_data by JSON-decoded value, not raw string, to avoid spurious diffs from re-serialization."
}

func (m customDataPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m customDataPlanModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || req.PlanValue.IsNull() || req.PlanValue.IsUnknown() {
		return
	}
	if jsonEqual(req.StateValue.ValueString(), req.PlanValue.ValueString()) {
		resp.PlanValue = req.StateValue
	}
}

// jsonEqual reports whether two JSON strings decode to equal values,
// ignoring key order and whitespace. Malformed JSON on either side is
// treated as unequal (falls through to a normal string diff, which will
// surface Terraform's own validation/API error rather than this modifier
// silently masking bad input).
func jsonEqual(a, b string) bool {
	var va, vb any
	if err := json.Unmarshal([]byte(a), &va); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(b), &vb); err != nil {
		return false
	}
	na, err := json.Marshal(va)
	if err != nil {
		return false
	}
	nb, err := json.Marshal(vb)
	if err != nil {
		return false
	}
	return string(na) == string(nb)
}

// customDataToAPI converts the schema's JSON-string custom_data into the
// map[string]any the client structs expect. Returns (nil, nil) for a null/
// unset attribute — omitted from the request, per the same "leave nil,
// let omitempty handle it" pattern every other Optional field in this
// codebase already uses for genuinely-absent values. A non-null-but-empty
// object ("{}"​) round-trips as an empty (non-nil) map, distinct from unset.
func customDataToAPI(v types.String) (map[string]any, error) {
	if v.IsNull() || v.IsUnknown() {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(v.ValueString()), &m); err != nil {
		return nil, err
	}
	return m, nil
}

// customDataFromAPI converts the client's map[string]any back into the
// schema's JSON-string representation — types.StringNull() if Paddle
// returned no custom_data at all, matching the null-in/null-out symmetry
// every other nullable field in this codebase maintains.
func customDataFromAPI(m map[string]any) (types.String, error) {
	if m == nil {
		return types.StringNull(), nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return types.StringNull(), err
	}
	return types.StringValue(string(b)), nil
}
