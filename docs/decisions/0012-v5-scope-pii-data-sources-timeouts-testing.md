---
title: v5 scope is a PII-in-state audit, four plural lookup data sources, a notification replay action, a testing-infrastructure investment, and configurable timeouts
status: accepted
date: 2026-08-12
tags: [paddle, provider, scope, v5, pii, data-sources, testing, timeouts]
---

## Decision

Scoped via `/kms:roadmap` immediately after v0.5.0 shipped, following the
same interview-then-plan convention v1-v4 used. Five workstreams, one
`v0.6.0` release (see "Why" for the staging call):

1. **PII-in-state audit and fix.** `paddle_events`' `data` field can carry
   customer PII (e.g. a `customer.created` event's payload includes
   email/name) but never got the loud state-security warning
   [[pii-bearing-data-sources-need-state-security-warning]] gave
   `paddle_customer` — a real gap in what v0.5.0 shipped, found the night
   it shipped, not by design. Fixed with the same documentation-as-
   mitigation posture, extended to `paddle_events`. Alongside that fix, a
   full audit pass over every existing resource, data source, and action
   schema for any other overlooked PII vector (including whether
   `Sensitive: true` is missing anywhere it should be present — a
   different protection than the state-file warning, hides values from
   CLI/log output but doesn't stop them writing to state, worth checking
   while already auditing). `paddle_customers` (plural, see item 3) gets
   explicit extra scrutiny in this audit: listing multiple customers at
   once compounds the PII-in-state concern beyond what the singular
   lookup carries.

2. **Configurable timeouts.** A real `terraform-plugin-framework-timeouts`
   integration — see [[0013-configurable-timeouts-architecture]] for the
   full technical design (this was seriously considered for *not* doing
   at all, given it wouldn't have fixed the sweeper bugs that prompted the
   conversation; decided to proceed anyway because the underlying gap —
   this provider's HTTP timeout is a single hardcoded 60s constant with no
   user-facing override at all — is real and independent of that night's
   actual bug).

3. **Four new plural/list lookup data sources**: `paddle_subscriptions`,
   `paddle_transactions`, `paddle_notifications`, `paddle_customers` —
   complementing the singular "exactly one match" data sources v0.5.0
   shipped with a "give me everything matching these filters" shape.
   `paddle_customers` was included despite its PII-compounding concern
   (item 1) rather than deferred — the discovery-gap justification v0.5.0
   was built on (finding IDs from something a user actually has on hand)
   applies here too, and the concern is addressed by documentation +
   audit, not by leaving the gap unfilled.

4. **`paddle_notification_replay` action** — `POST
   /notifications/{id}/replay`, confirmed real
   ([[0007-replay-endpoint-and-timeouts-module-confirmed]]). Fits
   [[actions-map-to-single-api-endpoint]] cleanly (one endpoint, one
   action). Deliberately does **not** get the money-moving actions'
   search-before-invoke/no-blind-retry treatment
   ([[money-moving-actions-no-blanket-retry]]) — replaying a notification
   isn't financial or irreversible; the worst case of a duplicate replay
   is one extra webhook delivery attempt, not a real-world harm like a
   duplicate charge. A plain action matching the endpoint 1:1.

5. **Testing-infrastructure investment**: a general, reusable
   `httptest`-based mock-server pattern for resource CRUD logic (not
   scoped narrowly to just proving out the timeouts feature), *and* a
   retrofit of all five existing resources
   (`product`/`price`/`discount`/`discount_group`/`notification_setting`)
   with mock-based unit tests for their CRUD logic. This is additive,
   never a replacement — [[0003-acceptance-tests-against-live-sandbox]]
   stays the foundational verification standard; mock tests give a
   faster, cheaper local/CI signal underneath it, not instead of it. See
   [[mock-tests-supplement-not-replace-acceptance-tests]].

6. **Docs**: `examples/lookup-then-act/main.tf` demonstrating the actual
   v0.5.0 payoff end-to-end (look up a subscription/transaction, feed its
   ID straight into `paddle_subscription_cancel`/`paddle_adjustment`) —
   the exact discovery-gap story v0.5.0 was built around, not yet shown
   as a worked example anywhere. A README.md pointer to it from the
   Actions section. Hand-written usage documentation for the new
   `timeouts{}` block (schema reference itself is `tfplugindocs`-
   generated as always, but the *why*/*when* isn't).

**Single release, `v0.6.0`**, not staged across multiple — considered and
rejected staging (e.g. PII audit + replay action as one release, timeouts
+ testing infrastructure as a separate one to isolate the riskier
client-behavior change) in favor of matching this project's established
precedent: v1-v4 each shipped as one release unless a real asymmetric-
risk reason forced a split ([[0011-v4-scope-data-sources-and-regression-guard]]'s
explicit reasoning, itself following the same pattern). Nothing in this
scope is irreversible or money-moving, so there's no clear line to split
on the way v3's actions genuinely had one.

## Why

Derived directly from the closing conversation of the v0.5.0 release
session: a live investigation into leftover sweep failures surfaced a
real, previously-undiscussed architecture gap (the client's hardcoded
60s HTTP timeout, no override path), and a step back to ask "what else
improves plugin maturity and UX" surfaced the rest — including catching,
in the course of answering that question, a real PII-warning gap in what
had just shipped hours earlier. The throughline: v0.5.0 closed the
action-input discovery gap for singular lookups; this plan extends that
same value to plural lookups and a new action, closes a real PII-warning
gap found immediately after shipping, and invests in verification
infrastructure and configurability that this session's own debugging
made obvious were missing.

## Related

- [[0011-v4-scope-data-sources-and-regression-guard]]
- [[0013-configurable-timeouts-architecture]]
- [[0007-replay-endpoint-and-timeouts-module-confirmed]]
- `docs/guardrails/pii-bearing-data-sources-need-state-security-warning.md`
- `docs/guardrails/mock-tests-supplement-not-replace-acceptance-tests.md`
- `docs/guardrails/configurable-timeouts-need-a-hard-ceiling.md`
- `docs/plans/paddle-provider-v5.md`
