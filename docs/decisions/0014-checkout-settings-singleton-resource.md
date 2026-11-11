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

Written from the API reference (read 2026-11-11), then confirmed against the live sandbox on 2026-11-11: `TestAccPaddleCheckoutSettings_basic` (create, update, no-op plan, import, destroy-clears-to-null) passed in CI's `acceptance` job, PR #51. The API key needs both `checkout_settings.read` and `checkout_settings.write`; with read only, the test failed with a 403 before reaching the resource. The acceptance test records the account's existing value and restores it, because other acceptance tests need a default payment link set.

## Cleanup and sweepers

- No sweeper: nothing is created, so nothing can be archived or deleted. A leaked state is only the sandbox pointing at a test URL, and a sweeper cannot know the original value; clearing it would break the tests that need a link set.
- `e2e.yaml` saves the sandbox's value before apply and restores it in an `always()` step after destroy. It only exercises the resource when the latest published provider contains it, so the daily run does not fail before the first release that includes it.
