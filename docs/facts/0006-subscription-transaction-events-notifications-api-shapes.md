---
title: Real API shapes confirmed for subscriptions/transactions/events/notifications filtering
status: current
date: 2026-08-11
tags: [paddle, provider, data-sources, v4]
---

## Fact

Confirmed against Paddle's own API reference, 2026-08-11, while scoping
[[0011-v4-scope-data-sources-and-regression-guard]]:

- `GET /subscriptions` supports server-side filters on `customer_id`,
  `status` (comma-separated, multiple values), `address_id`,
  `collection_mode`, `id`, `price_id`, `scheduled_change_action` — not
  just an unbounded list. `internal/client.Client` already exercises this
  shape indirectly (`ListSubscriptions` currently lists everything
  unfiltered; the filters exist server-side and are documented, just not
  yet wired into a client method).
- `GET /transactions` supports server-side filters on `customer_id`,
  `subscription_id`, `status`, `origin`, among others — already exercised
  by this client today (`ListTransactionsByCustomer`,
  `ListSubscriptionChargeTransactions`), so a `paddle_transaction` data
  source's filtered lookup is extending an already-proven pattern, not a
  new one.
- `GET /events` exists: a paginated list of account events, filterable by
  `type` (comma-separated), **retained for 90 days only** — events older
  than that are gone, not just paginated-away. Any data source built on
  this needs to document the retention window explicitly (a lookup that
  silently returns nothing for anything older than 90 days is a footgun
  if undocumented).
- `GET /notifications` (list) plus per-notification delivery logs exist:
  a notification record represents one delivery attempt to a webhook
  endpoint or email destination, with the response Paddle received
  recorded against it — the natural read-side companion to the
  `paddle_notification_setting` resource this provider already manages
  (which only configures *where* to deliver, not what was actually
  delivered or whether it succeeded).

None of these required an API version bump or a beta/undocumented
endpoint — all four are stable, published API surface, confirmed via
Paddle's official docs (`developer.paddle.com/api-reference/...`), same
sourcing standard [[0001-catalog-only-scope-v1]] set for schema fields.

## Related

- [[0011-v4-scope-data-sources-and-regression-guard]]
- `docs/plans/paddle-provider-v4.md`
