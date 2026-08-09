package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestJSONEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{name: "identical", a: `{"a":1}`, b: `{"a":1}`, want: true},
		{name: "different key order", a: `{"a":1,"b":2}`, b: `{"b":2,"a":1}`, want: true},
		{name: "different whitespace", a: `{"a":1}`, b: `{ "a" : 1 }`, want: true},
		{name: "different value", a: `{"a":1}`, b: `{"a":2}`, want: false},
		{name: "nested object equal regardless of order", a: `{"a":{"x":1,"y":2}}`, b: `{"a":{"y":2,"x":1}}`, want: true},
		{name: "malformed a", a: `{not json`, b: `{}`, want: false},
		{name: "malformed b", a: `{}`, b: `{not json`, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := jsonEqual(tc.a, tc.b); got != tc.want {
				t.Errorf("jsonEqual(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestCustomDataToAPI(t *testing.T) {
	t.Run("null returns nil map, no error", func(t *testing.T) {
		m, err := customDataToAPI(types.StringNull())
		if err != nil {
			t.Fatalf("customDataToAPI: %v", err)
		}
		if m != nil {
			t.Errorf("m = %v, want nil", m)
		}
	})
	t.Run("unknown returns nil map, no error", func(t *testing.T) {
		m, err := customDataToAPI(types.StringUnknown())
		if err != nil {
			t.Fatalf("customDataToAPI: %v", err)
		}
		if m != nil {
			t.Errorf("m = %v, want nil", m)
		}
	})
	t.Run("valid JSON decodes", func(t *testing.T) {
		m, err := customDataToAPI(types.StringValue(`{"internal_id":123,"flag":true}`))
		if err != nil {
			t.Fatalf("customDataToAPI: %v", err)
		}
		if m["internal_id"] != float64(123) || m["flag"] != true {
			t.Errorf("m = %v, want internal_id=123 flag=true", m)
		}
	})
	t.Run("malformed JSON errors", func(t *testing.T) {
		_, err := customDataToAPI(types.StringValue(`{not json`))
		if err == nil {
			t.Fatal("customDataToAPI: got nil error for malformed JSON, want an error")
		}
	})
}

func TestCustomDataFromAPI(t *testing.T) {
	t.Run("nil map becomes StringNull", func(t *testing.T) {
		v, err := customDataFromAPI(nil)
		if err != nil {
			t.Fatalf("customDataFromAPI: %v", err)
		}
		if !v.IsNull() {
			t.Errorf("v = %v, want null", v)
		}
	})
	t.Run("empty non-nil map round-trips as empty object, not null", func(t *testing.T) {
		v, err := customDataFromAPI(map[string]any{})
		if err != nil {
			t.Fatalf("customDataFromAPI: %v", err)
		}
		if v.IsNull() {
			t.Error("v is null, want a non-null empty-object JSON string")
		}
		if !jsonEqual(v.ValueString(), "{}") {
			t.Errorf("v = %q, want {}", v.ValueString())
		}
	})
	t.Run("real map round-trips", func(t *testing.T) {
		v, err := customDataFromAPI(map[string]any{"internal_id": float64(123)})
		if err != nil {
			t.Fatalf("customDataFromAPI: %v", err)
		}
		if !jsonEqual(v.ValueString(), `{"internal_id":123}`) {
			t.Errorf("v = %q, want {\"internal_id\":123}", v.ValueString())
		}
	})
}

func TestCustomDataPlanModifier_KeepsStateOnSemanticEquality(t *testing.T) {
	m := customDataPlanModifier{}

	req := planmodifier.StringRequest{
		StateValue: types.StringValue(`{"a":1,"b":2}`),
		PlanValue:  types.StringValue(`{"b":2,"a":1}`), // same data, different key order
	}
	resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
	m.PlanModifyString(context.Background(), req, resp)

	if resp.PlanValue.ValueString() != req.StateValue.ValueString() {
		t.Errorf("PlanValue = %q, want the prior state value %q (semantically equal, should not diff)",
			resp.PlanValue.ValueString(), req.StateValue.ValueString())
	}
}

func TestCustomDataPlanModifier_LetsRealChangesThrough(t *testing.T) {
	m := customDataPlanModifier{}

	req := planmodifier.StringRequest{
		StateValue: types.StringValue(`{"a":1}`),
		PlanValue:  types.StringValue(`{"a":2}`), // genuinely different
	}
	resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
	m.PlanModifyString(context.Background(), req, resp)

	if resp.PlanValue.ValueString() != req.PlanValue.ValueString() {
		t.Errorf("PlanValue = %q, want the new planned value %q to pass through unchanged",
			resp.PlanValue.ValueString(), req.PlanValue.ValueString())
	}
}

func TestCustomDataPlanModifier_SkipsUnknownAndNullStates(t *testing.T) {
	m := customDataPlanModifier{}

	t.Run("unknown plan value is left alone", func(t *testing.T) {
		req := planmodifier.StringRequest{
			StateValue: types.StringValue(`{"a":1}`),
			PlanValue:  types.StringUnknown(),
		}
		resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
		m.PlanModifyString(context.Background(), req, resp)
		if !resp.PlanValue.IsUnknown() {
			t.Errorf("PlanValue = %v, want it to stay Unknown", resp.PlanValue)
		}
	})

	t.Run("null state value (first create) is left alone", func(t *testing.T) {
		req := planmodifier.StringRequest{
			StateValue: types.StringNull(),
			PlanValue:  types.StringValue(`{"a":1}`),
		}
		resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
		m.PlanModifyString(context.Background(), req, resp)
		if resp.PlanValue.ValueString() != `{"a":1}` {
			t.Errorf("PlanValue = %v, want the planned value unchanged", resp.PlanValue)
		}
	})
}
