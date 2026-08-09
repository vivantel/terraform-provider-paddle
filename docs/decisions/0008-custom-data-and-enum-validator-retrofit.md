---
title: Retrofit custom_data exposure and enum validators onto v1 resources
status: accepted
date: 2026-08-08
tags: [paddle, provider, schema, retrofit]
---

## Decision

Before (or alongside) v2's new resources, fix two known inconsistencies in
the three v1 resources:

1. **Expose `custom_data`** on `paddle_product`, `paddle_price`, and
   `paddle_discount`. `client.Product`, `client.Price`, and
   `client.Discount` all already have a `CustomData map[string]any` field
   — the client-side support exists — but none of the three resource
   schemas expose it as an attribute, so it's currently unreachable from
   Terraform entirely.
2. **Add `stringvalidator.OneOf` enum validation** to `paddle_product`'s
   `tax_category` and `paddle_price`'s `type`/`tax_mode`, matching what
   `paddle_discount`'s `type`/`mode` already have. Right now a typo in
   `tax_category` (e.g. `"stadnard"`) only surfaces as an opaque 400 from
   Paddle at apply time; the same typo in `paddle_discount.type` is caught
   at plan/validate time with a clear Terraform-native error.

## Why

Both are inconsistencies already noticed and explicitly deferred, not new
findings:

- `custom_data` was flagged when reviewing `product_resource.go` during
  the Step 2 discount work: "client.Product.CustomData exists but isn't
  exposed in the Terraform schema at all — dead code, or reserved for
  future use." Confirmed dead-on-arrival for all three resources, not just
  Product.
- The validator gap was called out directly in the commit that added
  `paddle_discount`'s validators: "First validators in this repo;
  `paddle_product`/`paddle_price` predate this and don't have equivalent
  validators on their enum-like fields... worth a retrofit pass later for
  consistency."

`custom_data` in particular is a real, common use case for this class of
provider — attaching arbitrary key/value metadata to catalog objects for
downstream systems to read (analytics tagging, internal categorization,
cross-referencing an external system's ID) — not a hypothetical.

## Consequences

- `custom_data` as `types.Map`/`map[string]types.String` (or
  `map[string]any`-shaped via a custom type) needs the same
  null-vs-unknown handling every other Optional field in this codebase
  needs — check `docs/plans/paddle-provider-v1.md`'s recorded lesson list
  (Step 2's status block) before writing the `toAPI*`/`fromAPI*` wiring
  from scratch.
- New unit tests per `docs/guardrails/pure-logic-needs-unit-tests.md` for
  the `custom_data` clear/set round-trip on all three resources.
- Validators alone don't need new client changes or sandbox verification —
  they're pure schema-level plan-time checks, no API interaction — but do
  need a regenerated `docs/resources/*.md` (validator constraints show up
  in the generated schema docs) per
  `docs/guardrails/docs-must-be-regenerated-before-merge.md`.
- New resources added under [[0007-v2-scope-discount-groups-and-notification-settings]]
  should get `custom_data` exposure and enum validators from the start,
  not as a later retrofit — this decision exists specifically so that
  doesn't need repeating a third time. See
  `docs/guardrails/expose-custom-data-on-catalog-resources.md`.

## Related

- [[0007-v2-scope-discount-groups-and-notification-settings]]
- `docs/guardrails/expose-custom-data-on-catalog-resources.md`
- `docs/guardrails/pure-logic-needs-unit-tests.md`
- `docs/plans/paddle-provider-v2.md`
