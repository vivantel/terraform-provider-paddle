---
title: Money-moving or state-changing actions must not blindly retry, and must check current state before invoking
status: active
date: 2026-08-10
tags: [paddle, provider, actions, resilience, safety]
---

## Guardrail

Terraform actions whose underlying Paddle API call is not safe to blindly
repeat — `paddle_adjustment` (create-adjustment), and every subscription
action (`cancel`/`pause`/`resume`/`charge`) — need two independent
protections, not just one:

1. **No blanket retry.** Must not go through `internal/client.Client`'s
   standard retry-on-429/5xx path unmodified. An ambiguous failure
   (request sent, response lost to a timeout or 5xx) must surface as a
   clear, distinct error telling the user to check the Paddle
   dashboard/API directly for the actual outcome before retrying manually
   — never get silently retried by the shared wrapper. The exact
   mechanism (a `skipRetry`/no-retry variant of `client.Client.do`, a
   separate method, or something else) is an implementation decision for
   `docs/plans/paddle-provider-v3.md` to resolve — this guardrail only
   fixes the requirement.

2. **Search-before-invoke.** The no-retry protection above only stops the
   *provider* from auto-retrying — it does nothing to stop a user who
   sees the resulting error from manually re-running `terraform apply`
   and double-executing for real. Before performing the actual mutation,
   every action in scope here must check whether it's already been done:
   - **State-transition actions** (`cancel`, `pause`, `resume`) have a
     natural check available for free: `GET` the subscription's current
     status before invoking. Only treat the call as a successful no-op if
     the subscription is already in the *specific* target state that
     endpoint produces — already `canceled` for `cancel`, already
     `paused` for `pause`, already `active` for `resume` (confirm the
     exact status value Paddle uses for a resumed subscription at
     implementation time — don't assume `active` without checking).
     **Do not treat "any status other than the source state" as
     equivalent to "already done"** — e.g. `resume`'s no-op check must
     not be "not `paused`", because a `canceled` subscription is also
     "not `paused`" but resume can't reach it and silently reporting
     success there would mask a real failure (the subscription stays
     canceled, the config claims success). Any status that's neither the
     source state nor the confirmed target state must fall through to
     the normal mutating call and let Paddle's own response (success or
     a real error) decide, not get short-circuited into a false
     no-op.
   - **Object-creating actions** (`paddle_adjustment`) don't have a
     status to check, but do have a list endpoint: query existing
     adjustments for the same `transaction_id` and compare against
     `reason` (and any other correlating field the real API exposes —
     confirm at implementation time) before creating a new one. This is
     best-effort dedup, not a guarantee — document that limitation in the
     action's schema description rather than implying it's airtight.
   - **`paddle_subscription_charge`**: `custom_data` was initially thought
     to be a universal fix for this one (see this guardrail's own prior
     revision), but implementation-time research (2026-08-10) found
     Paddle's `items` field actually has three variants, and
     **`custom_data` only exists on the two non-catalog variants — not on
     the catalog-price variant, which is the dominant real-world case and
     the only one this provider's `paddle_subscription_charge` action
     supports** (the other two variants were deliberately scoped out as
     their own bigger schema-design task, not shipped half-modeled). The
     shipped mechanism instead lists this subscription's
     `origin=subscription_charge` transactions and matches on an exact
     `price_id`+`quantity` item-set comparison — a **weaker** check than
     a synthetic key would have been (two deliberately separate charges
     for identical items are indistinguishable from a retry to this
     check), documented as such in the action's own schema description.
     **Second correction, found running the real sandbox acceptance test,
     2026-08-11**: that transaction-search check only works for
     `effective_from="immediately"` — Paddle creates no queryable
     transaction at all for a `"next_billing_period"` charge until the
     subscription actually renews, so the search silently found nothing
     both before and after invoking, `paddle_subscription_charge`'s own
     duplicate-prevention **did not fire at all** for that input value.
     Fixed in code by branching on `effective_from`: `"next_billing_period"`
     checks `GetSubscriptionNextTransaction` (Paddle's own
     `?include=next_transaction` renewal preview) instead of searching
     transactions — matches the documented API shape. **Not yet confirmed
     against a real response**: running this branch for real found the
     preview not reliably reflecting a just-queued charge quickly enough
     for a test to observe (unclear from this session whether that's
     Paddle-side eventual consistency or a real bug in the matching
     logic) — this repo's own acceptance test was switched to
     `"immediately"` instead so it has *reliable* invoke-twice coverage,
     rather than asserting on a path not yet confirmed to work. The
     `"next_billing_period"` branch remains a real, documented gap:
     implemented per spec, not proven against Paddle's actual behavior.
     This shipped broken (not just unverified — silently non-functional)
     in `v0.4.0-beta.1` for one full release before being caught — the
     "invoke twice, confirm once" acceptance-test standard this guardrail
     requires (see `docs/plans/paddle-provider-v3.md` Step 1 item 5) is
     exactly what caught it, once actually run against the real sandbox
     rather than only unit-tested.
     See `docs/plans/paddle-provider-v3.md` Step 2 for the full account
     of this correction.

## Why

Derived from [[0010-v3-scope-lifecycle-actions]]. Confirmed directly
against Paddle's own docs (errors reference, rate-limiting reference) and a
general search, 2026-08-10: Paddle's API has no idempotency-key mechanism
anywhere. `meta.request_id` exists only for support/logging per the errors
page, explicitly not for deduplication.
`docs/guardrails/client-calls-must-use-retry-wrapper.md` mandates every
call go through the shared retry wrapper uniformly — safe when it was
written, because every call in this provider up to v2 is retry-idempotent
(retrying a duplicate `create-product` just makes two products, mildly
annoying, no money moves). Adjustments and subscription actions break that
assumption: retrying an ambiguous 5xx after Paddle already processed the
refund/cancellation server-side would double-execute a real financial
operation or cut off a paying customer twice, with nothing on either side
to catch it.

The no-retry protection alone was the first fix written for this guardrail
(2026-08-10) and is necessary but not sufficient — it only closes the
*automatic* retry path. A human retrying by hand after seeing the error
message it produces is just as capable of double-executing as the
provider's own retry loop was. Search-before-invoke closes that second
path. (A related, lower-severity version of this same gap exists in every
`Create*` call this provider already shipped in v1/v2 — see
`docs/guardrails/catalog-creates-lack-dedup-check.md`, a separate,
lighter-weight flag for that pre-existing backlog, not fixed by this
guardrail.)

## Applies to

- `internal/provider/actions/*.go` — any action wrapping a Paddle endpoint
  that creates a financial adjustment or changes live subscription state.

## Related

- [[0010-v3-scope-lifecycle-actions]]
- `docs/guardrails/client-calls-must-use-retry-wrapper.md`
- `docs/guardrails/catalog-creates-lack-dedup-check.md`
- `docs/plans/paddle-provider-v3.md`
