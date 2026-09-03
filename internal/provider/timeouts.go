package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// defaultOpTimeout is what every resource operation uses when a user
// doesn't configure a timeouts{} block at all — matches the previous
// hardcoded client.retryOverallBudget exactly, so behavior is unchanged
// for anyone who doesn't opt in. See
// docs/decisions/0013-configurable-timeouts-architecture.md.
const defaultOpTimeout = 60 * time.Second

// maxResourceTimeout is the hard ceiling every resource's effective
// timeout is clamped to, no matter what a user configures — see
// docs/guardrails/configurable-timeouts-need-a-hard-ceiling.md. Applies to
// every operation on every resource that implements timeouts{} support,
// present and future.
const maxResourceTimeout = 30 * time.Minute

// timeoutOp identifies which of a timeouts.Value's four operation
// accessors resolveTimeout should read.
type timeoutOp int

const (
	timeoutOpCreate timeoutOp = iota
	timeoutOpRead
	timeoutOpUpdate
	timeoutOpDelete
)

// timeoutsAttrTypes is the attr.Type map every one of this provider's
// resources uses for its "timeouts" attribute — all five resources enable
// all four operations (see each resource's Schema()), so this one shared
// map covers all of them rather than each resource needing its own.
var timeoutsAttrTypes = map[string]attr.Type{
	"create": types.StringType,
	"read":   types.StringType,
	"update": types.StringType,
	"delete": types.StringType,
}

// nullTimeouts returns a properly-typed null timeouts.Value — the value an
// unset "timeouts" attribute has. Building a model struct literal with a
// bare zero-value timeouts.Value{} instead (easy to do by accident in a
// test) produces an Object with no attribute types at all, which fails
// schema-based state encoding with "Value Conversion Error: Expected
// framework type ... underlying type: tftypes.Object[...], Received ...
// tftypes.Object[]" — this exists so tests (and any other code building a
// model outside the normal Plan/State.Get() path) get a value that
// actually matches the schema instead of tripping that error.
func nullTimeouts() timeouts.Value {
	return timeouts.Value{Object: types.ObjectNull(timeoutsAttrTypes)}
}

// resolveTimeout reads op's configured (or defaulted to defaultOpTimeout)
// duration from configured, clamps it to maxResourceTimeout, and returns a
// context derived from ctx carrying that duration as its deadline — ready
// to pass straight to a client call. One shared helper for all five
// resources' four operations each, not five copy-pasted implementations,
// per docs/guardrails/configurable-timeouts-need-a-hard-ceiling.md.
//
// The returned CancelFunc must be deferred by the caller exactly like any
// context.WithTimeout result. On a diagnostics error (a malformed
// configured value), ctx is returned unchanged with a no-op cancel — the
// caller is expected to check diags.HasError() and return before using the
// context, the same convention every other diagnostics-returning call in
// this codebase already follows.
func resolveTimeout(ctx context.Context, configured timeouts.Value, op timeoutOp, defaultTimeout time.Duration) (context.Context, context.CancelFunc, diag.Diagnostics) {
	var d time.Duration
	var diags diag.Diagnostics

	switch op {
	case timeoutOpCreate:
		d, diags = configured.Create(ctx, defaultTimeout)
	case timeoutOpRead:
		d, diags = configured.Read(ctx, defaultTimeout)
	case timeoutOpUpdate:
		d, diags = configured.Update(ctx, defaultTimeout)
	case timeoutOpDelete:
		d, diags = configured.Delete(ctx, defaultTimeout)
	}
	if diags.HasError() {
		return ctx, func() {}, diags
	}

	if d > maxResourceTimeout {
		d = maxResourceTimeout
	}

	derived, cancel := context.WithTimeout(ctx, d)
	return derived, cancel, diags
}

// describedTimeouts returns the standard timeouts attribute with a
// MarkdownDescription that documents the default and hard-ceiling durations
// (interpolated from defaultOpTimeout/maxResourceTimeout so the prose can't
// drift from the actual values), per
// docs/guardrails/configurable-timeouts-need-a-hard-ceiling.md.
func describedTimeouts(ctx context.Context) schema.Attribute {
	base := timeouts.Attributes(ctx, timeouts.Opts{
		Create: true,
		Read:   true,
		Update: true,
		Delete: true,
	})
	// terraform-plugin-framework-timeouts always returns a
	// SingleNestedAttribute today (verified against the pinned v0.7.0
	// source) — panic rather than silently ship an attribute with no
	// MarkdownDescription if a future dependency bump ever changes that,
	// since this runs at Schema() construction time and any acceptance or
	// unit test that exercises a resource's Schema() will catch it
	// immediately.
	sn, ok := base.(schema.SingleNestedAttribute)
	if !ok {
		panic("describedTimeouts: timeouts.Attributes returned unexpected type; update this function for the new shape")
	}
	sn.MarkdownDescription = fmt.Sprintf(
		"Each operation defaults to %d seconds and is capped at a %d-minute hard ceiling, regardless of what is configured here.",
		int(defaultOpTimeout/time.Second), int(maxResourceTimeout/time.Minute),
	)
	return sn
}
