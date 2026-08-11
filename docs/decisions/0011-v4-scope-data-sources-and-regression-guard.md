---
title: v4 scope is lookup data sources for actions' inputs, a shape-resolution refactor, and scheduled action regression coverage
status: accepted
date: 2026-08-11
tags: [paddle, provider, scope, v4, data-sources]
---

## Decision

Following a product-management-style review of the provider's state right
after [[v0.4.0 stable release]] (`docs/plans/paddle-provider-v3.md`), the
next release takes on everything that review surfaced, in one `v0.5.0`:

1. **`paddle_subscription` data source** — lookup by ID, plus filter by
   `customer_id`/`status` (server-side filters confirmed real, see
   [[0006-subscription-transaction-events-notifications-api-shapes]]).
   Closes the actual usability gap the review found: the four
   subscription actions (`cancel`/`pause`/`resume`/`charge`) all require
   a `subscription_id` that, today, has no discovery path inside
   Terraform at all.
2. **`paddle_transaction` data source** — same shape (by ID, filter by
   `subscription_id`/`customer_id`/`status`), surfacing
   `Details.LineItems` in its schema. Closes the equivalent gap for
   `paddle_adjustment`, which needs both a `transaction_id` and an
   `item_id` three JSON-shapes deep in the raw API (see
   `docs/guardrails/money-moving-actions-no-blanket-retry.md`'s account
   of this session's bugs).
3. **A centralized line-item-shape resolution helper**, built *first* as
   this release's Step 1 — not opportunistically alongside the
   transaction data source. Two of the three real bugs fixed en route to
   `v0.4.0` stable came from the same root cause (three different,
   undocumented-at-a-glance item shapes on Transaction-adjacent API
   responses); the transaction data source is about to become a fourth
   consumer of that same surface, so it's designed against the clean
   abstraction from the start rather than against the raw shapes a
   second time.
4. **`paddle_customer` data source** (read-only, by ID or `email`) —
   reopens part of [[0010-v3-scope-lifecycle-actions]]'s deferral, with
   the PII-in-state concern that decision raised addressed head-on rather
   than redesigned around: Terraform stores data source reads in state
   just like resource state, plaintext by default, so this ships with an
   explicit, loud warning (schema description + a new README section)
   rather than pretending "read-only" makes the PII concern disappear.
   No `paddle_address` — customer email/name alone resolves the actual
   discovery gap (finding a subscription/transaction's owning customer)
   without pulling in postal-address PII too.
5. **`paddle_events` and `paddle_notification` data sources** — general
   account-activity lookup (`GET /events`, filterable by `type`, 90-day
   retention — must be documented, not silently discovered) and
   notification-delivery inspection (the natural read-side companion to
   the `paddle_notification_setting` resource this provider already
   manages: it configures *where* to deliver, this answers *did it
   arrive*).
6. **Extend `.github/workflows/e2e.yaml`** (the existing daily,
   push-independent job that tests the published Registry artifact) to
   include the actions' acceptance tests, not just the catalog resources'.
   Closes a real gap the review named: every one of `v0.4.0-beta.1`'s
   three real action bugs was only ever caught by a human manually
   running acceptance tests — nothing re-verifies a working action stays
   working between pushes to that code, the way `e2e.yaml` already does
   for catalog resources.
7. **Housekeeping, not scoped work**: fix `README.md`'s stale "Pre-1.0
   (`v0.2.x`)" status line (never updated since `v0.3.0`, nothing checks
   prose freshness the way `docs/` schema drift is checked in CI), and
   confirm the sweeper actually credited the stray duplicate charge this
   session left queued on the sandbox (`docs/guardrails/money-moving-actions-no-blanket-retry.md`'s
   account) once its billing date passes.

**Rejected for v4**: staging this across `v0.5.0`/`v0.6.0`/`v0.7.0` by
sensitivity — considered specifically for the customer-PII item, rejected
because none of these six items carries `v0.4.0`'s beta-verification
asymmetry (no money moves, nothing is irreversible), so there's no
technical reason forcing a split; the PII item's risk is addressed by
documentation, not by isolating its release. One version, matching v3's
own precedent.

## Why

Derived directly from the product review's findings (see this session's
review, not separately filed — the review itself was requested and
delivered as a chat response, not a durable artifact; this decision *is*
the durable record of what came out of it). The core throughline: v3
shipped five actions that are correctly designed but only really usable
if you already have IDs from outside Terraform. v4 closes that gap rather
than adding new action surface — it's entirely read-only, lower-risk than
v3's category of change by construction.

## Related

- [[0010-v3-scope-lifecycle-actions]]
- [[0006-subscription-transaction-events-notifications-api-shapes]]
- `docs/guardrails/catalog-resources-need-data-source.md`
- `docs/plans/paddle-provider-v4.md`
