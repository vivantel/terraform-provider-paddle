---
title: Every catalog resource exposes custom_data
status: active
date: 2026-08-08
tags: [paddle, provider, schema]
---

## Guardrail

Any resource wrapping a Paddle entity that supports `custom_data`
(confirmed against the API reference, not assumed) must expose it as a
schema attribute and wire it through `toAPI*`/`fromAPI*`, in the same PR
that adds the resource — not as a follow-up retrofit. `paddle_product`,
`paddle_price`, and `paddle_discount` all had this gap
([[0008-custom-data-and-enum-validator-retrofit]]); it shouldn't recur for
`paddle_discount_group`/`paddle_notification_setting` or anything added
after them.

## Why

Derived from [[0008-custom-data-and-enum-validator-retrofit]]: the client
struct already having `CustomData map[string]any` but the schema not
exposing it made the field silently unreachable from Terraform for three
resources in a row before anyone noticed. Checking this at resource-design
time costs nothing; retrofitting it later costs a second decision record
and a second PR.

## Applies to

- Every new `internal/provider/*_resource.go` schema.
- PR review / self-review checklist for any new catalog resource, same as
  `docs/guardrails/catalog-resources-need-data-source.md`.

## Related

- [[0008-custom-data-and-enum-validator-retrofit]]
- `docs/guardrails/catalog-resources-need-data-source.md`
