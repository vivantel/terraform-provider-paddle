---
title: Actions need regression coverage that runs independent of any push
status: active
date: 2026-08-11
tags: [paddle, provider, actions, ci, v4]
---

## Guardrail

Every Terraform action this provider ships must have its real-sandbox
acceptance test(s) included in a scheduled CI job that runs independent
of any code push — not only in the push/PR-triggered `ci.yaml`
`acceptance` job. `.github/workflows/e2e.yaml` (daily, tests the
published Registry artifact) is that job; extend it to run the actions'
acceptance tests alongside the catalog resources' it already covers,
rather than creating a second scheduled workflow.

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
