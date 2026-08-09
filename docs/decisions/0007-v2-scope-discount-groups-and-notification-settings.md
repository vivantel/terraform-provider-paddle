---
title: v2 scope adds Discount Groups and Notification Settings
status: accepted
date: 2026-08-08
tags: [paddle, provider, scope, v2]
---

## Decision

The next release after v1 (`paddle_product`/`paddle_price`/`paddle_discount`)
adds two more full CRUD resources, each with a matching data source per
`docs/guardrails/catalog-resources-need-data-source.md`:

- `paddle_discount_group`
- `paddle_notification_setting`

And one lower-priority stretch addition, only if the two above land with
capacity to spare:

- `paddle_checkout_domain`

Field lists confirmed directly against Paddle's real API reference
(2026-08-08), not guessed:

- **Discount Groups** — trivially small: `name` (1-500 chars) is the only
  create/update field beyond `status` (`active`/`archived` — same
  archive-not-delete pattern as Product/Price/Discount; no separate delete
  operation). `create-discount-group` / `update-discount-group`.
- **Notification Settings** — `description`, `type` (`email`/`url`,
  **create-only** — absent from the update field list, so this needs
  `RequiresReplace` the same way Price's `product_id` does),
  `destination`, `subscribed_events` (array of event type names) at
  create; update additionally exposes `active` (bool, not settable at
  create — defaults `true`). Unlike every v1 resource, Notification
  Settings has a **real hard DELETE** endpoint
  (`delete-notification-setting`), not an archive-via-update pattern —
  don't reuse the `Archive*`/`statusPatch` client pattern here, it doesn't
  apply.
- **Checkout Domains** — not yet field-verified against the live API
  reference; do that before implementing, same standard as the two above,
  not this decision's job to guess ahead of time.

## Why

Both of the two primary additions were already flagged as deferred-not-
rejected, not newly discovered:

- [[0001-catalog-only-scope-v1]] explicitly said Notification Settings
  "were considered for inclusion... but deferred to keep v1 scope tight;
  they can be added as an independent resource later without touching
  this decision." This is that later addition.
- `paddle_discount` (`internal/provider/discount_resource.go`) already has
  a `discount_group_id` attribute referencing a group that, today, can
  only be created via the Paddle dashboard or a raw API call — there's no
  way to declare the whole relationship in Terraform. This is a genuine
  gap in v1, not scope creep: v1 shipped a discount schema that references
  an entity type it doesn't manage.

Checkout Domains is included as a stretch item because it's the same
catalog/config shape (an approved-domains allowlist, no lifecycle
character) as everything already in scope — but it's genuinely lower value
than the two primary additions, so it shouldn't block them if time is
tight.

## Consequences

- Notification Settings needs its own client archive/delete story —
  `client.DeleteNotificationSetting` (real DELETE), not
  `client.ArchiveNotificationSetting` (there's no `Archive*`-shaped
  operation for this entity at all). `Delete()` in
  `notification_setting_resource.go` should not follow the
  `client.IsNotFound(err) → tolerate` pattern blindly without checking
  whether Paddle's DELETE endpoint itself 404s on an already-deleted
  destination the same way the archive endpoints do — verify against the
  real sandbox, the same way every other resource this provider has
  shipped needed a real-sandbox check to catch something local
  tests/review couldn't.
- `type`'s `RequiresReplace` on Notification Settings is the same class of
  fix as Price's `product_id` — confirmed from the API reference before
  writing the resource this time, not discovered via a sandbox crash after
  the fact.
- This still doesn't revisit [[0001-catalog-only-scope-v1]]'s exclusion of
  Subscriptions/Transactions/Customers/Addresses/Businesses/Adjustments —
  that remains a deliberate, standing exclusion, not something this
  decision reopens.

## Related

- [[0001-catalog-only-scope-v1]]
- `docs/guardrails/catalog-resources-need-data-source.md`
- `docs/plans/paddle-provider-v2.md`
