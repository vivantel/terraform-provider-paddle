---
title: Every resource must implement ImportState
status: active
date: 2026-08-08
tags: [paddle, provider, import]
---

## Guardrail

Every resource (`paddle_product`, `paddle_price`, `paddle_discount`) must
implement `ImportState` (typically via
`resource.ImportStatePassthroughID` against the Paddle object ID, unless a
resource's ID scheme requires something more specific — e.g. a price
importing by its own ID rather than needing the parent product ID). A
resource shipped without import support is not considered complete, and its
acceptance test suite must include an import test step
(`ImportState: true`, `ImportStateVerify: true`).

## Why

Derived from: the user's explicit instruction to "follow best practices"
when scoping this provider. Import support is one of the most basic
expectations of a Terraform resource — without it, anyone with existing
Paddle catalog objects created outside Terraform (e.g. via the Paddle
dashboard, or a pre-Terraform integration) has no way to bring them under
management short of delete-and-recreate, which is unacceptable for billing
catalog objects that may be referenced by live subscriptions.

## Applies to

- `internal/provider/*_resource.go` — every resource's `ImportState` method.
- `internal/provider/*_resource_test.go` — every resource's acceptance test
  suite needs an import step.

## Related

- [[0001-catalog-only-scope-v1]]
- `docs/plans/paddle-provider-v1.md`
