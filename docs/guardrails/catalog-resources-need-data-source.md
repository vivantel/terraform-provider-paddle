---
title: Every catalog resource ships a matching data source
status: active
date: 2026-08-08
tags: [paddle, provider, data-sources]
---

## Guardrail

Every catalog resource type this provider manages
(`paddle_product`, `paddle_price`, `paddle_discount` — see
[[0001-catalog-only-scope-v1]]) must ship a matching read-only data source
(`data "paddle_product"`, `data "paddle_price"`, `data "paddle_discount"`)
in the same PR that adds the resource. A resource without its data source
counterpart is not considered complete.

## Why

Derived from: the [[0001-catalog-only-scope-v1]] commitment to a "mature"
provider, plus the standard Terraform convention that config elsewhere in a
root module needs a way to look up and reference existing objects of a type
without importing them into that module's state. Shipping resources without
data sources is a common gap that makes providers awkward to compose with
existing infrastructure.

## Applies to

- `internal/provider/*_resource.go` and the data source files that must
  accompany each one (`internal/provider/*_data_source.go`).
- PR review / self-review checklist for any new catalog resource.

## Related

- [[0001-catalog-only-scope-v1]]
- `docs/plans/paddle-provider-v1.md`
- `docs/guardrails/lookup-data-sources-required-for-action-inputs.md` —
  the same underlying principle, generalized to actions' required ID
  inputs rather than catalog resources specifically (added 2026-08-11,
  [[0011-v4-scope-data-sources-and-regression-guard]]).
