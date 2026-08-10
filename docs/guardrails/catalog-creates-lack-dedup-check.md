---
title: Existing Create* calls have no search-before-create protection against a retried duplicate
status: active — resolved by analysis, not by adding a search-before-create code path (see "Why", 2026-08-10 revision)
date: 2026-08-10
tags: [paddle, provider, resilience, backlog]
---

## Guardrail

Confirmed by reading `internal/client/client.go` (2026-08-10):
`CreateProduct`, `CreatePrice`, `CreateDiscount`, `CreateDiscountGroup`,
and `CreateNotificationSetting` are each a single `c.do(ctx, POST, ...)`
call with no preceding search/list/lookup, and `do()` retries any POST on
429/5xx exactly like every other call. A retried `Create` on an ambiguous
5xx (request processed by Paddle, response lost) can, in principle,
silently create a duplicate catalog object.

**Revised 2026-08-10, at implementation time**: this guardrail originally
prescribed a search-before-create fix mirroring
`docs/guardrails/money-moving-actions-no-blanket-retry.md`'s
search-before-invoke pattern. Attempting to actually implement it
surfaced two problems serious enough to change the fix:

1. **No server-side filter exists** for the fields that would need
   searching (`name` for Discount Groups, `destination`/`type` for
   Notification Settings — confirmed against both list endpoints'
   real query parameters) — a search means a full, unbounded
   client-side list-and-scan. Tolerable for Discount Groups/Notification
   Settings (genuinely low-cardinality per account in practice), but
   **not** for Products/Prices/Discounts, which a real catalog can have
   thousands of — running a full list-and-scan before every single
   `Create` would be a real performance/cost regression for exactly the
   accounts this provider is meant to serve well, to fix a rare,
   low-severity edge case.
2. **"Adopt a found match into state" is a materially worse risk for a
   *resource* than the equivalent "skip, no-op" is for an *action*.** An
   action has no persisted state — its worst case for a wrong match is
   skipping a legitimate second invocation. A resource's `Create()`
   populating Terraform state from a *coincidentally* name-matching (but
   actually unrelated) pre-existing object is a real correctness bug: a
   later `terraform destroy` would archive an object this config never
   actually created. Discount Group names and Notification Setting
   destinations are user-chosen strings, not resource-scoped identifiers
   — a collision with an unrelated object created by a different
   config/account activity is a real, not merely theoretical, risk this
   provider has no way to rule out from a name/destination match alone.

**The actual fix, once the real failure mode is examined closely: no
`Create()` code change is needed.** For Discount Groups specifically,
Paddle already enforces server-side name uniqueness — the "silent
duplicate" this guardrail worried about **cannot happen** for this
entity; a retried `Create` after an ambiguous failure surfaces a clear,
self-diagnosing `409 discount_group_name_conflict` (this is the same
error this project already hit and fixed once in
`.github/workflows/e2e.yaml`, for exactly this reason). That's a
categorically different, much less severe symptom than the money-moving
actions guardrail's silent-double-execution risk — a confusing error a
user can investigate and resolve, not an invisible financial duplicate.
`FriendlyErrorMessage` (`internal/client/client.go`) already surfaces
Paddle's specific error code/detail clearly, confirmed by reading it —
nothing further to add. Notification Settings and Product/Price/Discount
have no such server-side uniqueness constraint, so a retried `Create`
there can genuinely produce a duplicate object — a real but low-severity
outcome (an extra catalog object, not money movement), and, per the two
points above, not safely fixable by search-before-create without
introducing a worse risk (Notification Settings/Discount Groups) or an
unacceptable performance cost (Product/Price/Discount).

## Why

Derived while scoping [[0010-v3-scope-lifecycle-actions]]'s actions
layer: designing search-before-invoke for the new money-moving actions
surfaced this same-shaped, lower-severity gap in already-shipped
resources. Initially assumed the same fix pattern would generalize
directly; implementation-time analysis (2026-08-10) found it doesn't —
see the revised Guardrail section above for the two specific reasons.
This is the kind of gap this project's own
`docs/skills/verify-before-claiming.md` exists to catch: a roadmap-time
assumption ("the actions fix generalizes to resources") that looked
reasonable until actually built, at which point it turned out to
introduce a worse problem (state-corrupting misadoption) than the one it
was meant to solve (an occasional confusing error message).

## Applies to

- `internal/client/client.go`'s `Create*` methods, present and future —
  as a documented, analyzed non-issue for Discount Groups (server-side
  uniqueness already prevents the duplicate; the error is already clear)
  and an accepted, low-severity, not-worth-the-risk-to-fix gap for
  Product/Price/Discount/Notification Setting.

## How to apply

No code change. If a future entity's `Create` has both (a) a
cheap, precise, server-side-filterable uniqueness key (not a
fuzzy client-side name/field match) and (b) low enough cardinality that a
list-and-scan is cheap, search-before-create may be worth revisiting for
that entity specifically — evaluate case by case, don't generalize this
guardrail's original blanket prescription again without re-checking both
conditions.

## Related

- [[0010-v3-scope-lifecycle-actions]]
- `docs/guardrails/money-moving-actions-no-blanket-retry.md`
- `docs/plans/paddle-provider-v3.md`
