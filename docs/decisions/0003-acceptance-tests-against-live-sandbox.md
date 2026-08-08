---
title: Acceptance tests run against a live Paddle sandbox account
status: accepted
date: 2026-08-08
tags: [paddle, provider, testing]
---

## Decision

CRUD acceptance tests for every resource (`paddle_product`, `paddle_price`,
`paddle_discount`) use `terraform-plugin-testing`'s `resource.Test` /
`resource.ParallelTest` against a **real Paddle sandbox account**, using the
`PADDLE_API_KEY` env var (see [[0002-paddle-sandbox-account-available]]).
Tests are gated behind `TF_ACC=1`, exactly like `go test` acceptance tests in
AWS/GitHub/other mainstream providers — `go test ./...` without `TF_ACC` set
must not attempt any network calls.

## Why

Paddle does not publish an official local mock or sandbox emulator for its
Billing API, so three options existed:

1. **Live sandbox** *(chosen)* — real confidence that the client and
   resources actually work against Paddle's real API shape, including any
   quirks the docs don't fully capture. Standard pattern for Terraform
   providers wrapping a cloud API with no official mock.
2. **HTTP recording/replay (VCR-style)** — hermetic CI, no live dependency,
   but requires building/maintaining a recording harness before any tests can
   be written, and recorded fixtures silently go stale as Paddle's API
   evolves.
3. **Stubbed client unit tests only** — zero external dependency, fastest,
   but provides no real confidence the provider works against Paddle at all;
   wouldn't catch real API drift or auth/serialization bugs.

You already have a sandbox account, which removes the main practical
objection to option 1 (needing to provision one first).

## Consequences

- CI needs `PADDLE_API_KEY` (sandbox key) wired in as a GitHub Actions
  secret, and a workflow step that sets `TF_ACC=1` when running the
  acceptance suite.
- Every acceptance test file needs a `testAccPreCheck(t)` helper that skips
  (via `t.Skip`, not fails) when `PADDLE_API_KEY` isn't set, so local
  `go test ./...` stays usable without sandbox credentials.
- Every resource needs a `CheckDestroy` that verifies the object is actually
  gone in Paddle after `terraform destroy`, not just gone from state.
- Sandbox test runs create and destroy real (sandboxed) Products/Prices/
  Discounts — tests must clean up after themselves even on failure, to avoid
  the sandbox account accumulating orphaned objects over time.

## Related

- [[0002-paddle-sandbox-account-available]]
- `docs/guardrails/acceptance-tests-require-tf-acc-gate.md`
- `docs/plans/paddle-provider-v1.md`
