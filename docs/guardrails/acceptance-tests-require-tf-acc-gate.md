---
title: Acceptance tests must be gated behind TF_ACC and never run by default
status: active
date: 2026-08-08
tags: [paddle, provider, testing, ci]
---

## Guardrail

Every acceptance test function (anything calling `resource.Test` /
`resource.ParallelTest` against the live Paddle sandbox — see
[[0003-acceptance-tests-against-live-sandbox]]) must call a
`testAccPreCheck(t)` helper at its start that `t.Skip`s (not fails) when
`PADDLE_API_KEY` is unset, and must rely on `terraform-plugin-testing`'s own
`TF_ACC` gate rather than any custom gating. Plain `go test ./...` with no
env vars set must complete with these tests skipped, not run and not failed,
so contributors without sandbox credentials can still run the rest of the
suite locally.

## Why

Derived from [[0003-acceptance-tests-against-live-sandbox]]: choosing to test
against a live sandbox only works long-term if it never becomes a barrier to
running the *rest* of the test suite, and never accidentally creates real
sandbox objects during an unrelated `go test ./...` run (e.g. someone running
tests before they've set up sandbox credentials at all).

## Applies to

- `internal/provider/*_resource_test.go`
- The CI workflow's separate "unit tests" vs "acceptance tests" jobs (see
  `docs/plans/paddle-provider-v1.md`) — only the acceptance job sets
  `TF_ACC=1` and injects `PADDLE_API_KEY` from secrets.

## Related

- [[0003-acceptance-tests-against-live-sandbox]]
- [[0002-paddle-sandbox-account-available]]
- `docs/plans/paddle-provider-v1.md`
