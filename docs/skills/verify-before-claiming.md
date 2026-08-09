---
title: Verify against reality before claiming, don't infer from a similar prior fact
status: active
date: 2026-08-09
tags: [paddle, provider, process, review]
---

Before stating something is true — a field exists, an endpoint behaves a
certain way, a sandbox account already has some object, a design decision's
premise is correct — check it directly against the real thing (the live API
reference, the real sandbox, the actual CI run output), not by inferring it
from how a similar-looking prior case worked. If checking isn't possible in
the moment, say so explicitly rather than stating the inference as fact.

This is the single practice that caught every real bug found across this
provider's whole build-out, and every miss happened when it was skipped:

- **Checkout Domains** (`docs/plans/paddle-provider-v2.md` Step 6): assumed,
  from `paddle_discount`/`paddle_notification_setting` both having a normal
  create/update/delete lifecycle, that Checkout Domains would too. The real
  API reference has no create or update operation for it at all — dashboard
  only. Caught by fetching the actual API docs before writing any code, not
  after.
- **`subscribed_events` shape** (Notification Settings): could have assumed
  the response mirrors the request shape, the way most of this API does.
  It doesn't — request is an array of strings, response is an array of
  event objects. Caught by checking the real API reference for both
  directions separately, not just one.
- **`api_version` defaulting**: assumed `Optional` alone was enough because
  nothing in the field's description suggested otherwise. The real sandbox
  returned a non-null default on the very first `Create`, producing
  "Provider produced inconsistent result after apply." Caught only because
  the acceptance test ran against the real sandbox instead of stopping at
  "it compiles and the unit tests pass."
- **The sandbox already having a checkout domain**: stated as fact in a
  plan-doc status line, based on nothing — no check was actually run. The
  next real CI run showed the acceptance test hitting its skip path, proving
  the claim wrong. Caught by the CI run's actual output, not by re-reading
  the earlier (unverified) claim.

None of these were caught by code review, type-checking, or "this pattern
worked before elsewhere" reasoning — only by an explicit check against the
real system, run in the moment the claim was about to be made.

## How to apply

- A claim about an external system (an API's behavior, a field's shape, a
  sandbox account's state) needs a citation to what was actually checked —
  a fetched doc, a real API response, a CI run's log output — not "this is
  usually how it works" or "the last similar resource behaved this way."
- If a check can't be run right now, say the claim is unverified rather than
  stating it as settled fact — the honest gap is more useful than a
  confident guess that turns out wrong later.
- This applies as much to status/plan-doc updates as to code — a "confirmed
  against the real sandbox" line in a plan doc is itself a claim that needs
  to have actually happened, not be written because it's what usually
  happens at that step.

## Related

- `docs/guardrails/bulk-mechanical-edits-need-per-site-review.md`
- `docs/plans/paddle-provider-v2.md`
