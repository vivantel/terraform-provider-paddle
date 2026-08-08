---
title: v1 scope is Paddle catalog resources only (Products, Prices, Discounts)
status: accepted
date: 2026-08-08
tags: [paddle, provider, scope]
---

## Decision

The v1 "mature" release of `terraform-provider-paddle` manages only Paddle's
**catalog/config** surface as full CRUD resources:

- `paddle_product` (already scaffolded)
- `paddle_price` (already scaffolded)
- `paddle_discount` (new — see [[0001-catalog-only-scope-v1]] plan in
  `docs/plans/paddle-provider-v1.md`)

Explicitly **out of scope** for v1: Subscriptions, Transactions, Customers,
Addresses, Businesses, Adjustments, and Notification Settings / webhook
destinations.

## Why

Paddle Billing's full API surface splits into two kinds of objects:

1. **Catalog/config objects** (Products, Prices, Discounts) — declared
   upfront, safe to own as infrastructure-as-code, rarely mutated outside of
   deploys.
2. **Lifecycle objects** (Subscriptions, Transactions, Customers, Addresses,
   Businesses) — normally created at runtime via checkout, signup flows, or
   webhooks, not declared upfront. Modeling these as Terraform resources
   fights the grain of the domain: Terraform expects to own the full
   create/update/destroy lifecycle of a resource, but a subscription's
   lifecycle is actually owned by the customer's checkout session and
   Paddle's billing engine, not by a `terraform apply`.

This mirrors how other billing-platform Terraform providers scope their v1s:
manage the config that supports the business, not the transactional data the
business produces.

Notification Settings were considered for inclusion (see conversation) but
deferred to keep v1 scope tight; they can be added as an independent
resource later without touching this decision.

## Consequences

- `paddle_discount` needs to be designed and implemented from scratch —
  no existing scaffold, unlike Product/Price.
- Nothing in this decision blocks adding Subscriptions/Customers/Notifications
  as resources or data sources in a later major-ish bump; it only sets what
  v1 ships with.
- Any future proposal to manage Subscriptions as a full resource should
  revisit this decision explicitly rather than being added ad hoc, since it
  reverses the core rationale above.

## Related

- [[0001-existing-provider-baseline]] — what already exists before this work
- `docs/plans/paddle-provider-v1.md` — implementation plan
