---
title: Configurable timeouts — caller-wins precedence, a hard ceiling, and a mock-server verification approach
status: accepted
date: 2026-08-12
tags: [paddle, provider, timeouts, client, v5, testing]
---

## Decision

Adds a real `github.com/hashicorp/terraform-plugin-framework-timeouts`
`timeouts{}` block to every resource
(`product`/`price`/`discount`/`discount_group`/`notification_setting`),
covering all four operations (`create`/`read`/`update`/`delete`) —
matching that module's own recommended shape and what mature providers
(AWS, GCP, Azure) do, confirmed real via HashiCorp's own docs
([[0007-replay-endpoint-and-timeouts-module-confirmed]]), not assumed.

Four sub-decisions this required, each argued through directly rather
than defaulted to:

1. **Timeout precedence: caller-supplied deadline wins entirely.**
   `internal/client/client.go`'s `do()` currently *unconditionally*
   imposes its own `retryOverallBudget` (60s) via
   `context.WithTimeout(ctx, retryOverallBudget)` — since
   `context.WithTimeout` always takes the earlier of two deadlines, a
   longer Terraform-configured timeout could never actually take effect
   under the current code; it could only ever make things fail *faster*,
   never wait longer. That defeats the actual point of the feature (the
   case that prompted this whole conversation — the sweeper needing more
   patience for a slow operation). Fixed by having `do()` only impose its
   own 60s default when the incoming context carries no deadline at all;
   if a resource already set one (derived from the user's `timeouts{}`
   value), that value is respected as-is, not intersected with 60s.
2. **All four operations get a timeout**, not just `delete` — even
   though a slow `delete`/archive was the motivating case, matching the
   module's own convention and users' actual expectations (having only
   `delete` configurable would be a surprising, inconsistent gap).
3. **Default value when unconfigured: 60s**, matching current behavior
   exactly — not HashiCorp's own doc-example default of 20 minutes,
   which targets genuinely slow operations (cloud VM boot, service
   discovery) this provider's simple REST-backed CRUD doesn't have.
   Nothing changes for anyone who doesn't opt into the block.
4. **Hard ceiling: 30 minutes, regardless of what's configured.** Caller-
   wins precedence (item 1) with no cap at all would let a pathological
   config (a typo, a misunderstanding) hang a `terraform apply`
   indefinitely on a genuinely stuck call. 30m gives real headroom over
   today's fixed 60s for legitimate slow-operation cases while still
   bounding the worst case. See
   [[configurable-timeouts-need-a-hard-ceiling]] — this is a standing
   rule any future resource adding timeout support must also respect, not
   just a one-time implementation detail here.

**Verification approach**: Paddle's real API can't be forced to respond
slowly on demand, so timeout-firing behavior can't be proven against the
live sandbox the way this project normally proves things
([[0003-acceptance-tests-against-live-sandbox]]). A deliberately slow/
hanging `httptest` handler is the only way to verify this deterministically
— this is the seed of the broader mock-server testing investment
[[0012-v5-scope-pii-data-sources-timeouts-testing]] scopes in alongside
it (item 5), rather than building a one-off test double just for this
feature and discarding the pattern afterward.

## Why

Surfaced directly from a real debugging session (the same night as the
v0.5.0 release): the sweeper's fixed 60s `retryOverallBudget` caused real,
repeated pain against a genuinely slow/rate-limited Paddle sandbox, and
investigating it exposed that this provider has *no* user-facing timeout
configuration anywhere — a real, independent gap from the sweeper bug
itself (the sweeper doesn't go through Terraform's resource lifecycle at
all, so this feature wouldn't have fixed that night's actual failure, but
the underlying "60s hardcoded, no override" gap is real for actual
`terraform apply`/`destroy` users hitting a slow response too).

The precedence decision specifically (item 1) is the one piece of this
plan that changes existing client behavior, not just adds new surface —
worth its own decision record separate from the pure scope-and-priority
call in [[0012-v5-scope-pii-data-sources-timeouts-testing]], the same way
[[0009-tflog-observability-and-acceptance-test-sweepers]] was kept
separate from the scope decisions around it.

## Consequences

- `internal/client/client.go`'s `do()`/`doNoRetry()` need a real change,
  not just additive code: the current unconditional
  `context.WithTimeout(ctx, retryOverallBudget)` must become conditional
  on whether the incoming context already carries a deadline. Existing
  callers that never set one (the sweeper, actions, every resource
  before this plan) keep exactly today's 60s behavior — this is a
  behavior-preserving refactor for everyone who doesn't opt in, not a
  silent behavior change.
- Every resource's `Create`/`Read`/`Update`/`Delete` needs to derive a
  context from the configured (or defaulted) timeout and pass it through
  to the client calls it makes — a real, mechanical change across all
  five resources, not a schema-only addition.
- The 30m ceiling needs enforcing at the point the configured value is
  read (before it's turned into a context deadline), not just documented
  — see [[configurable-timeouts-need-a-hard-ceiling]] for the exact
  guardrail.

## Related

- [[0012-v5-scope-pii-data-sources-timeouts-testing]]
- [[0007-replay-endpoint-and-timeouts-module-confirmed]]
- [[configurable-timeouts-need-a-hard-ceiling]]
- [[mock-tests-supplement-not-replace-acceptance-tests]]
- `docs/decisions/0003-acceptance-tests-against-live-sandbox.md`
- `docs/plans/paddle-provider-v5.md`
