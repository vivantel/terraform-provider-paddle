---
title: v3 scope is Terraform actions for Paddle lifecycle operations (adjustments, subscription ops)
status: accepted
date: 2026-08-10
tags: [paddle, provider, scope, v3, actions]
---

## Decision

The next release after v2 adopts Terraform Plugin Framework **actions** — a
mechanism this provider has never used before — and ships five of them:

- `paddle_adjustment` — a single action with an `action` attribute
  (`refund`/`credit`), matching Paddle's real `create-adjustment` endpoint
  1:1. No split into separate refund/credit actions.
- `paddle_subscription_cancel`
- `paddle_subscription_pause`
- `paddle_subscription_resume`
- `paddle_subscription_charge`

Each subscription action matches one real Paddle endpoint
(`/subscriptions/{id}/{cancel,pause,resume,charge}`) 1:1. No
`paddle_subscription` resource is added — subscriptions remain excluded as
a full resource, unrevisited, per [[0001-catalog-only-scope-v1]] (they're
checkout-created, not declared upfront). Actions were chosen specifically
because they don't require owning the resource lifecycle to act on it — a
`subscription_id` string attribute is enough, the same way Stripe's own
`refund` action takes a bare `charge`/`payment_intent` string rather than
requiring a managed Charge resource.

**Explicitly deferred, not rejected:**

- `paddle_customer`/`paddle_address`/`paddle_business` as full CRUD
  resources. Two things briefly seemed to justify reversing part of
  [[0001-catalog-only-scope-v1]]'s exclusion (which explicitly named
  Customers/Addresses/Businesses, grouped with Subscriptions/Transactions,
  as "normally created at runtime... not declared upfront") — neither
  held up under review:
  - *"The API supports full CRUD"* — true (`GET POST PATCH`, confirmed
    against the real API reference 2026-08-10), but Transactions also has
    `POST`/`PATCH` and stays excluded; API shape was never the actual test
    0001 used, creation-time pattern was. Whether Paddle customers are, in
    practice, checkout-created (like Transactions) or commonly
    API-provisioned directly by merchants (which would justify treating
    them differently from Transactions) was never verified.
  - *"Stripe's official provider has a full `resource_stripe_customer`"* —
    checked directly: Stripe's entire provider repo is headed
    `// File generated from our OpenAPI spec`, and its README states "This
    repository is automatically generated from Stripe's internal
    tooling." It reflects exhaustive spec coverage, not a curated
    judgment that Customers deserve IaC treatment. Not usable as
    precedent.
  
  A PII-in-state-file concern was also raised and never designed for —
  customer email/name/address/tax-ID data would sit in Terraform state
  (plaintext by default) and, if treated like Product/Price config, risk
  ending up committed to source control alongside `.tf` files.
  Revisiting this needs actual verification of Paddle's real
  customer-creation pattern plus a PII/state-security story, not a redo of
  either argument above.
- Subscription `activate` (trialing → active) — no clear operator use case
  surfaced; trials normally activate on schedule or via checkout, not an
  IaC-triggered action.
- Subscription `preview-update`/`preview-charge` — non-destructive reads,
  not "actions" in the Stripe/this-provider sense (no side effect). Better
  fit as data sources if ever wanted, not this action batch.
- Payment Methods (Paddle: read + delete only, no create) and Transactions
  stay excluded per [[0001-catalog-only-scope-v1]], unrevisited.

## Why

Real Paddle API shape, confirmed against the live reference 2026-08-10
(not assumed): Adjustments is create-and-retrieve only — no update, no
delete, nothing resembling a resource lifecycle. Subscription
cancel/pause/resume/charge are each an independent `POST` endpoint with no
corresponding `GET`/read-back-as-the-same-object semantics. Both are
exactly the shape Stripe's own actions layer (`internal/provider/actions/`,
first shipped in the `v0.3.0-beta.3` tag — `main` lags behind and doesn't
have it yet, confirmed by comparing the tag's tree against `main`'s) covers
with actions and deliberately keeps out of its `resource_*` set: a one-time,
irreversible "verb," never a "noun" with a lifecycle to reconcile on a
later plan.

**Confirmed no idempotency-key support exists anywhere in Paddle's API**
(checked 2026-08-10: the errors reference page, the rate-limiting page, and
a general search all independently turn up nothing — `meta.request_id`
exists only for support/logging per the errors page, explicitly not for
deduplication). This provider's existing
`docs/guardrails/client-calls-must-use-retry-wrapper.md` mandates every API
call go through the shared client, which retries automatically on
`429`/`5xx` ([[0005-http-client-retry-backoff]]) — safe so far because
every existing call has been retry-idempotent (retrying a duplicate
`create-product` just makes two products; retrying a duplicate
`create-adjustment` after an ambiguous failure would double-refund real
money, with nothing on either side to catch it). This decision requires a
carve-out — see `docs/guardrails/money-moving-actions-no-blanket-retry.md`.

Terraform Plugin Framework actions require Terraform ≥1.14 and
`terraform-plugin-framework` ≥1.15. `go.mod` already pins v1.19.0 — no
dependency bump needed (see [[0005-plugin-framework-already-satisfies-actions-version-floor]]),
but no `required_version` constraint is declared anywhere in this repo
today, so this is a provider-wide floor bump for every user, accepted
explicitly during the interview that produced this decision (the
alternative — dropping actions from this version and forcing
refund/subscription ops into an awkward resource shape with no real
update/delete — was rejected).

## Consequences

- New guardrails required (not optional), written alongside this decision:
  - `docs/guardrails/actions-map-to-single-api-endpoint.md`
  - `docs/guardrails/money-moving-actions-no-blanket-retry.md` — two
    protections, not one: no blanket retry (closes the provider's own
    auto-retry path), *and* search-before-invoke (closes the
    just-as-real path of a human manually re-running `terraform apply`
    after seeing the no-retry error — the first draft of this decision
    only covered the former, extended 2026-08-10 once that gap was
    pointed out).
  - `docs/guardrails/catalog-creates-lack-dedup-check.md` — a related,
    lower-severity version of the same underlying gap (no
    idempotency-key support means any retried `POST` can double-create),
    found already present in every `Create*` call shipped through v2.
    Initially flagged as a deferred backlog item; pulled into this
    version's scope the same day (`docs/plans/paddle-provider-v3.md`
    Step 7) once checking each entity's real field list turned up a
    concrete, low-effort mechanism for 4 of 5 entities — cheap enough
    not to defer.
- `docs/guardrails/client-calls-must-use-retry-wrapper.md` gets a short
  exception note pointing at the new retry guardrail — the blanket
  "every call" language predates this decision and no longer holds
  without qualification.
- `required_version = ">= 1.14.0"` needs declaring in
  `examples/provider/provider.tf` (and wherever else
  `docs/guardrails/example-version-constraints-track-latest-minor.md`
  already tracks) — doc-only change, no dependency work.
- Acceptance-testing this action layer is structurally harder than
  anything shipped so far: refunding needs a real completed transaction;
  subscription ops need a real active subscription; per existing docs,
  subscriptions can only be created via checkout, not Terraform. Every
  resource shipped through v2 self-provisions its own fixtures via
  `terraform apply` — this layer can't. Fixture strategy (Paddle's
  Simulations API, confirmed full-CRUD in the same API-reference pass
  that surfaced everything else in this decision, or a scripted setup
  step) is `docs/plans/paddle-provider-v3.md`'s problem to resolve, not
  assumed here.
- Docs must carry an explicit operational warning: these actions move
  money or change live billing state, with no server-side dedup.
  Recommend a separate, more tightly-scoped API key for
  action-containing configs, and flag that this repo's own
  `.github/workflows/e2e.yaml` uses `terraform apply -auto-approve`
  throughout — any future example or CI config exercising these actions
  should not follow that pattern unmodified, or should sit behind an
  explicit human-approval gate.
- This does not reopen [[0001-catalog-only-scope-v1]]'s exclusion of
  Customers/Addresses/Businesses/Transactions/Payment Methods as
  resources — that remains standing, same as [[0007-v2-scope-discount-groups-and-notification-settings]]
  didn't reopen Subscriptions/Transactions. A future proposal to add them
  needs its own verification pass (real customer-creation pattern,
  PII/state-security design), not a rerun of either debunked argument
  above.

## Related

- [[0001-catalog-only-scope-v1]]
- [[0005-http-client-retry-backoff]]
- [[0005-plugin-framework-already-satisfies-actions-version-floor]] (fact,
  same number, different directory — `docs/facts/`)
- [[0007-v2-scope-discount-groups-and-notification-settings]]
- `docs/guardrails/client-calls-must-use-retry-wrapper.md`
- `docs/guardrails/actions-map-to-single-api-endpoint.md`
- `docs/guardrails/money-moving-actions-no-blanket-retry.md`
- `docs/guardrails/catalog-creates-lack-dedup-check.md`
- `docs/plans/paddle-provider-v3.md`
