---
title: Existing Create* calls have no search-before-create protection against a retried duplicate
status: active
date: 2026-08-10
tags: [paddle, provider, resilience, backlog]
---

## Guardrail

Confirmed by reading `internal/client/client.go` (2026-08-10):
`CreateProduct`, `CreatePrice`, `CreateDiscount`, `CreateDiscountGroup`,
and `CreateNotificationSetting` are each a single `c.do(ctx, POST, ...)`
call with no preceding search/list/lookup, and `do()` retries any POST on
429/5xx exactly like every other call — this has been true since v1.
A retried `Create` on an ambiguous 5xx (request processed by Paddle,
response lost) can silently create a duplicate catalog object today.

**Per-entity mechanism, verified directly against the real client structs
(2026-08-10), not assumed uniform:**

| Entity | Mechanism | Notes |
|---|---|---|
| `Product` | Deterministic `custom_data` key | `CustomData map[string]any` exists on `client.Product`, client-settable, echoed back by Paddle. Generate a deterministic value (hash of the resource's own config inputs — name, tax_category, etc. — not a random UUID, since it must reproduce identically on a genuine retry) at `Create()` time, store it in `custom_data`, and search existing products for a match on that key before calling `CreateProduct`. |
| `Price` | Same as `Product` | `CustomData` exists on `client.Price`. |
| `Discount` | Same as `Product`, plus `Code` | `CustomData` exists on `client.Discount`; `Code` (`client.Discount.Code`) is also available and more naturally distinctive than a synthetic key, if practical to require/generate one. |
| `DiscountGroup` | **Search by `name`, no `custom_data` needed** | `client.DiscountGroup` has no `CustomData` field — confirmed absent, matches `0007`'s finding that Paddle's API genuinely doesn't offer one for this entity. Doesn't need a synthetic key anyway: this provider already discovered (`docs/plans/paddle-provider-v2.md` Step 4) that Paddle enforces **global uniqueness on `name`** for discount groups — search-before-create can just query by `name` directly, a stronger and already-guaranteed-unique key than any synthetic one. |
| `NotificationSetting` | **Best-effort only** | No `CustomData` field, no known uniqueness constraint on any field. Falls back to matching on `destination`+`type` (the latter is create-only/immutable, confirmed in `0007`) — imprecise, same caveat class as Adjustments' `reason`-based match in `docs/guardrails/money-moving-actions-no-blanket-retry.md`. Document the limitation in the resource's schema description rather than implying it's airtight, same discipline that guardrail requires for Adjustments. |

Going forward: any new resource's `Create*` client method should either
include a search-before-create check using the mechanism this table
implies for that entity's shape (a genuine unique/near-unique field →
search on it directly; only `custom_data`-style metadata available →
deterministic synthetic key; neither → documented best-effort or an
explicit note for why no check is needed) — never add a sixth `Create*`
method with the same silent gap without at least deciding one way or the
other.

## Why

Derived while scoping [[0010-v3-scope-lifecycle-actions]]'s actions layer:
designing search-before-invoke for the new money-moving actions
(`docs/guardrails/money-moving-actions-no-blanket-retry.md`) surfaced that
the same underlying gap — a retried POST can double-create, because
Paddle has no idempotency-key mechanism anywhere (confirmed 2026-08-10) —
already exists in every resource shipped through v2. It's lower severity
there (an extra `paddle_product` row is mildly annoying; an extra
`paddle_adjustment` moves real money) — that severity gap is why this
was initially scoped as a deferred backlog item rather than blocking v3.
Revisited the same day: checking each entity's actual field list (rather
than leaving "search-before-create" as an abstract future idea) took
minutes and turned up concrete, low-effort mechanisms for 4 of 5 entities
(3 via existing `custom_data`, 1 via an already-enforced uniqueness
constraint this provider already knew about) — cheap enough to fold into
[[0010-v3-scope-lifecycle-actions]]'s v3 scope directly rather than
deferring indefinitely. See `docs/plans/paddle-provider-v3.md` Step 7.

## Applies to

- `internal/client/client.go`'s `Create*` methods, present and future.

## How to apply

Implemented as `docs/plans/paddle-provider-v3.md` Step 7, using the
per-entity mechanism from the table above.

## Related

- [[0010-v3-scope-lifecycle-actions]]
- `docs/guardrails/money-moving-actions-no-blanket-retry.md`
- `docs/plans/paddle-provider-v3.md`
