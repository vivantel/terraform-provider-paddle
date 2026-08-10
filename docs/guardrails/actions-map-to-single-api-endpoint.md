---
title: Every Terraform action maps 1:1 to a single Paddle API endpoint
status: active
date: 2026-08-10
tags: [paddle, provider, actions]
---

## Guardrail

A new `action.Action` implementation must correspond to exactly one real
Paddle API endpoint — never invent a merge (bundling two endpoints behind
one action) or a split (breaking one endpoint into several actions) that
the API itself doesn't have. Where Paddle's own endpoint already
distinguishes behavior via a request field (e.g. `create-adjustment`'s
`action: refund|credit`), mirror that with a single action and the same
field/values — don't split it into separate actions per value.

## Why

Derived from [[0010-v3-scope-lifecycle-actions]]'s adjustments decision:
the real `create-adjustment` endpoint is one `POST` with an `action` field
distinguishing refund from credit. Matching it 1:1 (`paddle_adjustment`,
one action) was chosen over inventing a `paddle_refund`/`paddle_credit`
split — even though some other providers do split similarly-shaped
operations (Stripe's `refund`/`fee_refund` actions are genuinely two
different Stripe endpoints, not a naming preference to imitate; checked
directly, not assumed). Splitting or merging beyond what the API actually
does adds a translation layer this provider has to design, document, and
keep in sync forever, and makes the schema lie about what's actually one
underlying operation.

## Applies to

- `internal/provider/actions/*.go` (or wherever action files land) and PR
  review for any new action.

## Related

- [[0010-v3-scope-lifecycle-actions]]
- `docs/plans/paddle-provider-v3.md`
