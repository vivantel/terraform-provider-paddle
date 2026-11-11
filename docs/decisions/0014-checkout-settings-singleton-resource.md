---
title: paddle_checkout_settings is a singleton resource over the checkout URL settings
status: accepted
date: 2026-11-11
tags: [paddle, provider, scope, checkout]
---

## Decision

Add `paddle_checkout_settings`, managing Paddle's `GET`/`PATCH /settings/checkout-url` (one field, `default_url`, the account's default payment link). It is a singleton: fixed ID `checkout-url`, Create and Update are the same PATCH, destroy PATCHes an explicit `null`, and a setting found cleared by Read removes the resource from state so the next plan re-applies it.

## Why

Paddle has no payment-link entity: a link is either the account-wide default URL or a single transaction's `checkout.url`. The default URL is the one real, stable piece, and the API refuses to create any transaction until it is set (see the `paddle_adjustment` acceptance precondition in `README.md`). Managing it as code removes that manual dashboard step. A resource that created a transaction to mint a link was rejected: it would be a new billable-flow object per apply, and the link expires with the transaction.

Destroy clears rather than leaving the value because Terraform's contract is that a destroyed resource stops being managed; the doc says so, since clearing it blocks API transactions on that account.

## Verification status

Written from the API reference (read 2026-11-11), not yet confirmed against the live sandbox. Unverified assumptions: PATCH with `null` clears the field; the GET returns `default_url: null` rather than omitting it when unset; the sandbox applies no domain check. The acceptance test records and restores the account's existing value because other acceptance tests need one set. This decision is not done until `TestAccPaddleCheckoutSettings_basic` passes in CI's `acceptance` job.
