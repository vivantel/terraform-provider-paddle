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
   - **`paddle_subscription_charge`**: resolved 2026-08-10 —
     `create-subscription-charge`'s request body has a genuine
     client-settable field, confirmed against the real API reference:
     per-item `custom_data` (structured, echoed back) and `receipt_data`
     (free text, `immediately`-only). Generate a deterministic key (a
     hash of the invocation's own inputs, not a random UUID — actions
     have no persisted state between invocations to store a random one
     into), set it in `custom_data`, search recent transactions on the
     subscription for that key before invoking. See
     `docs/plans/paddle-provider-v3.md` Step 2 item 3 for the full
     mechanism.

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
