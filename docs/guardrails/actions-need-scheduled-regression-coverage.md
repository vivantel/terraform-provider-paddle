---
title: Actions need regression coverage that runs independent of any push
status: active
date: 2026-08-11
tags: [paddle, provider, actions, ci, v4]
---

## Guardrail

Every Terraform action this provider ships must have real-sandbox
coverage that runs independent of any code push — not only in the
push/PR-triggered `ci.yaml` `acceptance` job. `.github/workflows/e2e.yaml`
(daily, tests the published Registry artifact) is that job; extend it
rather than creating a second scheduled workflow.

**Implementation note, added once Step 6 of
`docs/plans/paddle-provider-v4.md` was actually built**: `e2e.yaml` has no `go test`
step at all — unlike `ci.yaml`'s `acceptance` job, it applies real
Terraform HCL against the *published* Registry binary
(`action_paddle_*_acc_test.go`'s tests build the provider in-process via
`testAccProtoV6ProviderFactories`, so they're structurally incapable of
testing a published binary regardless of what's tagged). "Extend it to
run the actions' acceptance tests" in practice means adding an `action`
block to that HCL, not widening a `go test -run` pattern.

**Deliberate, reasoned exception, not full "every action" coverage**:
`e2e.yaml` only exercises `paddle_subscription_pause`/`resume` — safe and
fully reversible against a shared pinned fixture
(`PADDLE_TEST_SUBSCRIPTION_ID`) on a daily, unattended, `-auto-approve`
schedule. `paddle_subscription_cancel` (irreversible), `_charge`
(money-moving, and its own search-before-invoke has a known false-positive
edge case documented in its schema), and `paddle_adjustment` (also
money-moving, needs a disposable per-run fixture this HCL-only approach
can't script without duplicating `internal/client/client.go`'s test-fixture
support in bash/curl) are excluded on purpose — running any of those
unattended, daily, forever is a worse trade than the coverage gap it
would close. `ci.yaml`'s `acceptance` job already covers all five actions,
including these three, via `-run TestAcc` against the in-process build on
every push; the gap this guardrail closes (a regression from a Paddle-side
API change with no code push to trigger `ci.yaml`) is real but lower-risk
for pause/resume than for the other three, which is exactly why the
exception is scoped this way rather than applied uniformly.

## Why

Found via product review right after `v0.4.0` shipped
([[0011-v4-scope-data-sources-and-regression-guard]]): every one of
`v0.4.0-beta.1`'s three real action bugs (nested-price parsing,
`next_billing_period` search-before-invoke's two compounding bugs, the
cancel short-circuit's missing fixture) was found only because a human
happened to run the acceptance tests against the real sandbox — nothing
in this provider's CI re-verifies a *working* action stays working once
no one is actively touching that code. `ci.yaml`'s `acceptance` job only
runs on push/PR; a regression introduced by a Paddle-side API change (not
a code change here) could sit undetected indefinitely. `e2e.yaml` already
solves exactly this problem for catalog resources — daily, independent of
any push, against the actual published artifact — so this guardrail is
"extend an already-proven mechanism to a category of code that needs it
even more (money-moving, not just config-management)", not a new
pattern.

## Applies to

- `.github/workflows/e2e.yaml`.
- Every action in `internal/provider/actions/*.go` and its acceptance
  test(s) in `internal/provider/action_*_acc_test.go`.

## Related

- [[0011-v4-scope-data-sources-and-regression-guard]]
- `docs/guardrails/money-moving-actions-no-blanket-retry.md`
- `docs/plans/paddle-provider-v4.md`
