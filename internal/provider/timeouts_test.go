package provider

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// timeoutsValueWithDelete builds a timeouts.Value whose "delete" attribute
// is set to raw (a duration string like "45m") and every other operation
// left null — matching the shape a real timeouts{} config block with only
// `delete` set produces (all five resources' schemas enable all four
// operations, see each resource's Schema(), so the value must carry all
// four attribute types even when only one is actually set — a value with
// just {"delete": ...} fails schema-based state encoding with a Value
// Conversion Error since it doesn't match the schema's full object type).
func timeoutsValueWithDelete(raw string) timeouts.Value {
	return timeouts.Value{
		Object: types.ObjectValueMust(
			timeoutsAttrTypes,
			map[string]attr.Value{
				"create": types.StringNull(),
				"read":   types.StringNull(),
				"update": types.StringNull(),
				"delete": types.StringValue(raw),
			},
		),
	}
}

func TestResolveTimeout(t *testing.T) {
	t.Run("unset uses the default", func(t *testing.T) {
		unset := timeouts.Value{Object: types.ObjectNull(map[string]attr.Type{"delete": types.StringType})}

		ctx, cancel, diags := resolveTimeout(context.Background(), unset, timeoutOpDelete, defaultOpTimeout)
		defer cancel()

		if diags.HasError() {
			t.Fatalf("unexpected diags: %v", diags)
		}
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("expected a deadline")
		}
		remaining := time.Until(deadline)
		if remaining <= 0 || remaining > defaultOpTimeout {
			t.Errorf("remaining = %v, want (0, %v]", remaining, defaultOpTimeout)
		}
	})

	t.Run("a configured value under the ceiling passes through", func(t *testing.T) {
		configured := timeoutsValueWithDelete("10m")

		ctx, cancel, diags := resolveTimeout(context.Background(), configured, timeoutOpDelete, defaultOpTimeout)
		defer cancel()

		if diags.HasError() {
			t.Fatalf("unexpected diags: %v", diags)
		}
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("expected a deadline")
		}
		remaining := time.Until(deadline)
		want := 10 * time.Minute
		// Allow a little slack for test execution time between resolveTimeout
		// computing the deadline and time.Until measuring it here.
		if remaining <= want-time.Second || remaining > want {
			t.Errorf("remaining = %v, want ~%v", remaining, want)
		}
	})

	t.Run("a configured value over the 30m ceiling is clamped", func(t *testing.T) {
		// docs/guardrails/configurable-timeouts-need-a-hard-ceiling.md: no
		// configured value, however large, may ever exceed maxResourceTimeout.
		configured := timeoutsValueWithDelete("24h")

		ctx, cancel, diags := resolveTimeout(context.Background(), configured, timeoutOpDelete, defaultOpTimeout)
		defer cancel()

		if diags.HasError() {
			t.Fatalf("unexpected diags: %v", diags)
		}
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("expected a deadline")
		}
		remaining := time.Until(deadline)
		if remaining <= 0 || remaining > maxResourceTimeout {
			t.Errorf("remaining = %v, want (0, %v] — the 24h configured value must be clamped to the ceiling", remaining, maxResourceTimeout)
		}
	})

	t.Run("an unparseable configured value surfaces a diagnostic", func(t *testing.T) {
		configured := timeoutsValueWithDelete("not-a-duration")

		_, cancel, diags := resolveTimeout(context.Background(), configured, timeoutOpDelete, defaultOpTimeout)
		defer cancel()

		if !diags.HasError() {
			t.Fatal("expected an error diagnostic for an unparseable duration")
		}
	})
}
