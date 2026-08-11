---
title: Every ID an action requires needs a Terraform-native lookup path
status: active
date: 2026-08-11
tags: [paddle, provider, actions, data-sources, v4]
---

## Guardrail

Any Terraform action this provider ships that requires an entity ID as
config (`subscription_id`, `transaction_id`, `item_id`, and any future
equivalent) must have a corresponding read-only data source that can
resolve that ID from something a real user actually has on hand
(a customer email, a subscription's own attributes, etc.) — not just from
the ID itself. An action whose only usable inputs are opaque Paddle IDs
with no in-Terraform way to discover them is not considered complete,
even if the action's own logic is correct.

This generalizes `docs/guardrails/catalog-resources-need-data-source.md`'s
principle (every *resource* ships a matching data source) to *actions*:
the underlying reason is the same — Terraform config elsewhere in a root
module needs a way to reference existing objects without an out-of-band
step — but actions have no resource of their own to attach a data source
convention to by default, so this needed stating separately rather than
assumed to already be covered.

## Why

Found via a product-review pass right after `v0.4.0` shipped
([[0011-v4-scope-data-sources-and-regression-guard]]): all five v3
actions are correctly implemented and real-sandbox-verified, but every
one of them requires an ID with no discovery path inside this provider —
`paddle_subscription_cancel`/`pause`/`resume`/`charge` need
`subscription_id`; `paddle_adjustment` needs both `transaction_id` and an
`item_id` that (per this session's own debugging) lives three JSON-shapes
deep in the raw Transaction API and isn't even obviously discoverable via
a direct API call, let alone from Terraform. In practice this meant the
actions were only really usable by hardcoding IDs found via the Paddle
dashboard or a manual API call — a real usability gap, not a documentation
gap.

## Applies to

- Every action in `internal/provider/actions/*.go` and any future one.
- The data source(s) satisfying this for v4:
  `paddle_subscription`, `paddle_transaction` — see
  [[0011-v4-scope-data-sources-and-regression-guard]] and
  `docs/plans/paddle-provider-v4.md`.

## Related

- `docs/guardrails/catalog-resources-need-data-source.md`
- [[0011-v4-scope-data-sources-and-regression-guard]]
- `docs/guardrails/money-moving-actions-no-blanket-retry.md`
