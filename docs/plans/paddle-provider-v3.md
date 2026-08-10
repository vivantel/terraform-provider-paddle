---
title: Implementation plan — terraform-provider-paddle v3
status: not started
date: 2026-08-10
tags: [paddle, provider, plan, v3, actions]
---

# Implementation plan: terraform-provider-paddle v3

**Read this whole file before doing anything.** Written to be
self-sufficient for a completely fresh session with zero prior context —
same convention as `docs/plans/paddle-provider-v1.md` and
`docs/plans/paddle-provider-v2.md`, which this plan follows on from.

Repo: `/home/ubuntu/projects/vivantel/terraform-provider-paddle`
(GitHub: `vivantel/terraform-provider-paddle`).

## Why this plan exists

v2 (`docs/plans/paddle-provider-v2.md`, all steps done, released as
`v0.2.0`/`v0.3.0`/`v0.3.1`) shipped `paddle_discount_group` and
`paddle_notification_setting` plus a `paddle_checkout_domain` data source.
A follow-up conversation (2026-08-10) asked "design the next version, use
Terraform actions if needed" and, after a three-persona review pass
(payment engineer / Stripe-provider-precedent check / senior architect)
that changed the scope significantly from the first draft, produced one
decision record:

- `docs/decisions/0010-v3-scope-lifecycle-actions.md` — adopts Terraform
  Plugin Framework **actions** (a mechanism this provider has never used)
  and scopes exactly five of them. Read it in full before writing any
  code — it also documents what was considered and explicitly rejected
  (a `paddle_customer`/`paddle_address`/`paddle_business` resource
  reversal that didn't survive review) so this plan doesn't have to
  re-justify staying away from that.

Supporting artifacts this plan implements directly:

- `docs/facts/0005-plugin-framework-already-satisfies-actions-version-floor.md`
- `docs/guardrails/actions-map-to-single-api-endpoint.md`
- `docs/guardrails/money-moving-actions-no-blanket-retry.md` — updated
  2026-08-10 after this plan's first draft: the no-retry carve-out
  (Step 0) alone doesn't stop a human from manually re-running `terraform
  apply` after seeing the error it produces, so this guardrail now also
  requires search-before-invoke, folded into Steps 1-2 below.
- `docs/guardrails/catalog-creates-lack-dedup-check.md` — a related,
  lower-severity gap in already-shipped `Create*` calls. Initially
  scoped as a deferred backlog item; pulled into this plan (Step 7) the
  same day once checking each entity's actual field list turned up a
  concrete, low-effort mechanism for 4 of 5 entities.

Read all four before making any judgment call not spelled out below.

## Ground truth before you start

- Go 1.25 (`go.mod`: `go 1.25.8`), `terraform-plugin-framework` v1.19.0 —
  already satisfies the actions version floor
  ([[0005-plugin-framework-already-satisfies-actions-version-floor]]), do
  not bump it as part of this work. Check `go.mod` isn't stale before
  assuming this.
- `master` is the default branch. Branch from `master`.
- Existing resources (all in `internal/provider/`, client methods in
  `internal/client/client.go`): `paddle_product`, `paddle_price`,
  `paddle_discount`, `paddle_discount_group`, `paddle_notification_setting`
  (each with a data source), plus `paddle_checkout_domain` (data source
  only). `internal/client.Client.do()` (line ~144) is the single
  chokepoint every one of those goes through, with retry/backoff on
  429/5xx and a 60s overall call budget (`retryOverallBudget`).
- **This plan adds a new top-level construct this provider has never had:
  actions.** `provider.go`'s `PaddleProvider` currently implements
  `Resources()` and `DataSources()` (lines 106, 116) but not
  `provider.ProviderWithActions`/`Actions()` — that interface conformance
  needs adding for the first time. Model the package layout on Stripe's
  `internal/provider/actions/` (one file per action,
  `action_<name>.go`) — reference implementation fetched and read
  2026-08-10 from
  `github.com/stripe/terraform-provider-stripe` tag `v0.3.0-beta.3`
  (**not** `main`, which lags behind and doesn't have actions at all —
  confirmed by diffing the two) — but this provider's actions are
  hand-designed against real Paddle field lists, not spec-generated;
  don't copy Stripe's generated-code patterns (e.g. its
  `assignStringToNamedFieldOrMethod` reflection helpers) wholesale, they
  solve a code-generation problem this provider doesn't have.
- Every established pattern from v1/v2 still applies where relevant to
  resources/data sources (not actions, which are new territory): the
  `IsNull()`+`IsUnknown()` check before `Value*()`, `stringvalidator.OneOf`
  on enums, unit tests for pure `toAPI*`/`fromAPI*` functions, acceptance
  tests confirmed against the real sandbox before calling anything done.

## How to update this file as you work

Same convention as v1/v2: each step has a `Status:` line, update in place
as you go. Commit via `docs/skills/commit-with-kms-attribute.md`, with
`Refs:` trailers to the decision/fact/guardrail files each step
implements. Regenerate docs via `tfplugindocs generate` after any schema
change (`docs/guardrails/docs-must-be-regenerated-before-merge.md`) and
confirm `git diff --exit-code -- docs/` locally before pushing.

---

## Step 0: retry-wrapper carve-out for non-idempotent calls

Status: pending

Implements: `docs/guardrails/money-moving-actions-no-blanket-retry.md`
(protection 1 of 2 — see that guardrail; protection 2, search-before-invoke,
is implemented per-action in Steps 1-2, not here, since it needs each
action's own lookup/status-check call, not a shared client-layer
mechanism).

**Do this before any action code that depends on it.** `internal/client.Client`
needs a way for a caller to opt out of the automatic retry-on-5xx path
while still going through the shared client (never a raw `http.Client` —
`docs/guardrails/client-calls-must-use-retry-wrapper.md` still applies to
*using the client*, just not its default retry behavior for these calls).

1. Design the exact mechanism — options to weigh, pick one, document why
   in this step's Status line once done:
   - A `doNoRetry(ctx, method, path, body, out any) error` sibling to
     `do()` that makes exactly one attempt (still inside the same overall
     timeout/context handling), no retry loop at all.
   - A `RetryPolicy` parameter/option threaded into `do()`.
   - Something else, if a cleaner shape presents itself once you're
     looking at the real code — this plan doesn't mandate the shape,
     only the requirement (see the guardrail).
2. On any failure from this path (network error, non-2xx, timeout), the
   error message must be distinct from the normal retryable-error path
   and must tell the caller to verify the actual outcome against Paddle
   (dashboard or a `GET` on the affected object) before trying again —
   this is what `Invoke()` in each action below surfaces as a
   `resp.Diagnostics.AddError`.
3. Unit test: confirm this path makes exactly one HTTP attempt even
   against a `5xx`/timeout response (`httptest` server, same pattern as
   existing client tests), unlike `do()` which retries.

**Reminder, not this step's job to fix:** this same double-create exposure
already exists in every `Create*` call shipped through v2 (`CreateProduct`,
`CreatePrice`, etc.) — see
`docs/guardrails/catalog-creates-lack-dedup-check.md` and this plan's
Step 7, which does fix it (in this plan's scope after all, once a
concrete per-entity mechanism was confirmed — see Step 7's own note on
why this isn't the same "out of scope" call [[0010-v3-scope-lifecycle-actions]]
made for Customer/Address/Business).

## Step 1: `paddle_adjustment` action

Status: pending

Implements: [[0010-v3-scope-lifecycle-actions]],
`docs/guardrails/actions-map-to-single-api-endpoint.md`.

1. **First**: fetch and confirm the real field list against
   `https://developer.paddle.com/api-reference/adjustments/create-adjustment`
   — this plan's research pass (2026-08-10) found `action`
   (`refund`|`credit`, required), `transaction_id` (required,
   `txn_...`), `reason` (required), `type` (`partial`|`full`, defaults
   `partial`), `items` (required for partial), `tax_mode`. Don't design
   the schema from this summary alone — verify directly, the same
   standard [[0007-v2-scope-discount-groups-and-notification-settings]]
   held Discount Groups/Notification Settings to. Also confirm: is there
   a `GET` for retrieving a single adjustment by ID for `Invoke()` to
   confirm terminal status, or only `list-adjustments`?
2. `internal/client/client.go`: `Adjustment`-shaped request/response
   structs, `CreateAdjustment(ctx, ...) (*Adjustment, error)` using
   Step 0's no-retry path. Also add `ListAdjustments(ctx, transactionID
   string) ([]Adjustment, error)` (or however the real API's filter
   parameters actually work — confirm against
   `list-adjustments`'s reference page) for item 3's search-before-invoke
   check.
3. `internal/provider/actions/action_paddle_adjustment.go` (new package —
   `internal/provider/actions/`): `action.Action` +
   `action.ActionWithConfigure`. Schema mirrors the confirmed field list.
   **Search-before-invoke** (`docs/guardrails/money-moving-actions-no-blanket-retry.md`):
   before calling `CreateAdjustment`, call `ListAdjustments` for the
   target `transaction_id` and check whether one already matches this
   invocation's `reason` (and `action`/`type`/amount, if those are
   practical to compare against the confirmed response shape). If a
   match is found, treat as already-done — `resp.SendProgress` a message
   saying so, don't create a second one. Document in the schema
   description that this check is best-effort correlation, not a
   guarantee. `Invoke()` then calls `CreateAdjustment`, checks the
   response for a terminal-success status if one exists (don't just
   treat "HTTP 2xx" as "done" if Paddle's response can carry a
   non-terminal state — confirm this against the real API response
   shape, see item 1), uses `resp.SendProgress` for a status message on
   success.
4. Register in `provider.go`'s new `Actions()` method (see Step 4).
5. Unit tests for any pure request-building logic, and for the
   search-before-invoke matching logic specifically (fabricate a
   `ListAdjustments` response containing a match and confirm `Invoke()`
   short-circuits without calling `CreateAdjustment`). Acceptance/
   integration test: needs a real completed transaction to adjust — see
   Step 5 for the fixture-strategy problem this and Step 2 share; don't
   invent a one-off fixture approach here, resolve it once in Step 5 and
   reuse. Include one acceptance test that invokes the action twice
   against the same transaction/reason and confirms only one adjustment
   exists afterward — this is the one thing this whole guardrail exists
   to prevent, so it needs its own explicit real-sandbox proof, not just
   unit-test coverage of the matching logic in isolation.
6. Docs: `docs/actions/paddle_adjustment.md` generation — check whether
   `tfplugindocs` already supports actions in this version, or whether
   action docs need hand-writing (`terraform-provider-development:provider-docs`
   skill covers this; check it before assuming either way).

## Step 2: subscription lifecycle actions (cancel, pause, resume, charge)

Status: pending

Implements: [[0010-v3-scope-lifecycle-actions]],
`docs/guardrails/actions-map-to-single-api-endpoint.md`.

1. **First**: fetch and confirm the real request/response shape for each
   of the four endpoints against
   `https://developer.paddle.com/api-reference/subscriptions/overview`
   (and each endpoint's own reference page) — this plan's research pass
   (2026-08-10) only confirmed the endpoints exist
   (`/subscriptions/{id}/{cancel,pause,resume,charge}`, all `POST`), not
   their field lists. Known shape hints worth verifying directly, not
   assuming: cancel and pause almost certainly take an `effective_from`
   (`immediately`|`next_billing_period`-shaped enum), pause likely takes
   a `resume_at` (or resumes indefinitely if omitted), charge needs
   `items` (what's being charged) and probably its own `effective_from`.
   **Do not let "immediate" be an implicit/default value for anything
   destructive here** — if the API defaults ambiguously, make the
   schema attribute required rather than guessing a default, the same
   discipline [[0007-v2-scope-discount-groups-and-notification-settings]]
   applied to `paddle_notification_setting`'s `type`.
2. `internal/client/client.go`: one method per endpoint
   (`CancelSubscription`/`PauseSubscription`/`ResumeSubscription`/
   `ChargeSubscription`), all via Step 0's no-retry path. Also add
   `GetSubscription(ctx, id) (*Subscription, error)` if it doesn't
   already exist — item 3's search-before-invoke check for
   cancel/pause/resume needs the subscription's current status, and
   nothing in this provider fetches a subscription today (no
   `paddle_subscription` resource/data source exists).
3. Four action files in `internal/provider/actions/`:
   `action_paddle_subscription_cancel.go`,
   `action_paddle_subscription_pause.go`,
   `action_paddle_subscription_resume.go`,
   `action_paddle_subscription_charge.go`. Each takes a
   `subscription_id` string attribute (no `paddle_subscription` resource
   exists or is planned — see [[0010-v3-scope-lifecycle-actions]] for
   why) plus that endpoint's own fields from item 1.
   **Search-before-invoke** (`docs/guardrails/money-moving-actions-no-blanket-retry.md`):
   - `cancel`/`pause`/`resume`: call `GetSubscription` first. Only
     short-circuit as already-done if the status is already that
     endpoint's *specific* target state — already `canceled` for
     `cancel`, already `paused` for `pause`, already `active` for
     `resume` (confirm the real "resumed" status value at
     implementation time, don't assume `active`). **Do not** treat
     "any status other than the source state" as already-done — in
     particular, `resume`'s check must not be "not `paused`": a
     `canceled` subscription is also not `paused`, but resume can't
     reach it from there, and silently reporting success would mask
     that nothing actually happened. Any status that's neither the
     source nor the confirmed target state falls through to the normal
     mutating call, so Paddle's own response (success or a real error)
     is what the user sees.
   - `charge`: resolved 2026-08-10 — `create-subscription-charge`'s
     request body has a genuine client-settable field, confirmed against
     the real API reference: per-item `custom_data` (structured
     key-value, echoed back on the resulting object) and `receipt_data`
     (free text, `immediately`-only). Generate a **deterministic** key
     (a hash of this invocation's own inputs — `subscription_id` +
     `items` + amount/description — not a random UUID, since a genuine
     retry must reproduce the identical key without anywhere durable to
     store one; actions have no persisted state between invocations,
     unlike a resource's `.tfstate` slot) and set it in `custom_data` at
     invoke time. Before calling `ChargeSubscription`, list recent
     transactions on the subscription and search for that exact key
     instead of a fuzzy amount/description/time-window heuristic. Still
     document as best-effort in the schema description (list-based
     search, not a server-side guarantee — same caveat class as every
     other search-before-invoke check here), just with a precise key to
     match on instead of guessing.
4. Register all four in `provider.go`'s `Actions()`.
5. Unit tests for each action's status-check short-circuit logic
   (`cancel`/`pause`/`resume`: fabricate a `GetSubscription` response
   already in the target state, confirm the mutating client method is
   never called). Acceptance/integration tests — same fixture-strategy
   dependency on Step 5 as `paddle_adjustment`, plus the same
   invoke-twice-confirm-once real-sandbox proof Step 1 item 5 requires,
   applied to at least `cancel` (cheapest to prove: cancel twice,
   confirm the second call short-circuits instead of erroring or
   double-processing).
6. Docs, same open question as Step 1 item 6.

## Step 3: `Actions()` on the provider

Status: pending

Implements: [[0010-v3-scope-lifecycle-actions]].

1. `internal/provider/provider.go`: add `var _
   provider.ProviderWithActions = &PaddleProvider{}` and implement
   `Actions(_ context.Context) []func() action.Action`, returning the
   five actions from Steps 1-2. Model directly on the existing
   `Resources()`/`DataSources()` methods (lines 106/116) for style
   consistency.
2. `Schema()` (line 41): no attribute changes needed for actions
   themselves, but confirm nothing about the provider-level schema
   assumes only resources/data sources exist.

## Step 4: `required_version` floor

Status: pending

Implements: [[0010-v3-scope-lifecycle-actions]],
[[0005-plugin-framework-already-satisfies-actions-version-floor]].

1. Add `required_version = ">= 1.14.0"` to the `terraform {}` block in
   `examples/provider/provider.tf` (the source `docs/index.md`'s example
   is generated from) and its hand-copied duplicates in `README.md` and
   `examples/full-stack/main.tf` — same three locations
   `docs/guardrails/example-version-constraints-track-latest-minor.md`
   already tracks for the `required_providers` version constraint; add
   this alongside it, don't introduce a fourth place to keep in sync.
2. Consider whether `docs/guardrails/example-version-constraints-track-latest-minor.md`
   itself needs a note that it now also covers `required_version`, not
   just the provider source version constraint — extend that guardrail
   file if so, rather than leaving this plan as the only place that
   connection is written down.

## Step 5: acceptance-test fixture strategy for actions

Status: pending

Implements: [[0010-v3-scope-lifecycle-actions]] (the "acceptance-testing
this action layer is structurally harder" consequence).

Every resource shipped through v2 self-provisions its own fixtures via
`terraform apply` inside the test itself. This doesn't work here:
`paddle_adjustment` needs a real completed transaction; the subscription
actions need a real active subscription; per this provider's own existing
docs, subscriptions are only created via checkout, not Terraform.

1. Research Paddle's Simulations API
   (`https://developer.paddle.com/api-reference/simulations/overview` —
   confirmed "full CRUD" in this plan's initial research pass, 2026-08-10,
   but not field-verified) as a way to script a fake completed
   transaction/subscription into the sandbox for test setup. If it can
   produce a real `txn_...`/`sub_...` ID usable by the actions above,
   this is likely the cleanest fixture path.
2. If Simulations doesn't fit, fall back to a documented manual/scripted
   setup step (comparable to how
   `docs/plans/paddle-provider-v2.md`'s Step 6 handled
   `paddle_checkout_domain`'s test needing a manually-configured sandbox
   domain) — document exactly what a fresh session needs to do before
   these tests can pass for real, not just skip cleanly.
3. Whatever's chosen, apply it consistently to both Step 1 and Step 2's
   acceptance tests rather than inventing a different approach per
   action.

## Step 6: safety documentation

Status: pending

Implements: [[0010-v3-scope-lifecycle-actions]] (the operational-warning
consequence).

1. `README.md`: a new section (or an addition to an existing "Usage"
   section) covering: these five actions move real money or change live
   billing state; Paddle has no idempotency-key support
   ([[0010-v3-scope-lifecycle-actions]]'s "Why"), so an ambiguous failure
   requires manually checking Paddle before retrying, not re-running
   `terraform apply` blindly; recommend a separate, more tightly-scoped
   API key for configs that include these actions.
2. Check `.github/workflows/e2e.yaml` and any other example/CI config
   added or touched by this plan for `-auto-approve` — don't add a new
   example that exercises these actions under `-auto-approve` without an
   explicit comment explaining that's a deliberate, reviewed choice for
   that specific context (e.g., a sandbox-only CI smoke test), not a
   pattern to copy for real configs.

## Step 7: search-before-create retrofit for existing `Create*` calls

Status: pending

Implements: `docs/guardrails/catalog-creates-lack-dedup-check.md`.

Flagged 2026-08-10 while designing Steps 1-2's search-before-invoke
requirement: the same underlying gap — a retried `POST` can silently
double-create, because Paddle has no idempotency-key mechanism anywhere —
already exists in every `Create*` call this provider shipped through v2
(`CreateProduct`, `CreatePrice`, `CreateDiscount`, `CreateDiscountGroup`,
`CreateNotificationSetting`). Initially scoped as a deferred backlog item
(lower severity than the actions layer — an extra catalog object, not
duplicated money movement), then pulled into this plan the same day once
checking each entity's real field list turned up a concrete, low-effort
mechanism for 4 of 5 — see the table in
`docs/guardrails/catalog-creates-lack-dedup-check.md` for the full
per-entity detail; summarized here:

1. **`Product`/`Price`/`Discount`**: all three have a `CustomData
   map[string]any` field, client-settable and echoed back. Generate a
   deterministic key (hash of the resource's own config inputs — same
   "deterministic, not random" reasoning as Step 2's `charge` fix, though
   these *do* have `.tfstate` to persist into if that turns out simpler
   than re-deriving the hash on every `Create()` call — worth comparing
   both before picking one), store it in `custom_data`, search for a
   match before calling `CreateProduct`/`CreatePrice`/`CreateDiscount`.
2. **`DiscountGroup`**: no `custom_data` field exists on this entity at
   all (confirmed absent). Doesn't need one — Paddle enforces global
   uniqueness on `name` for discount groups (already discovered in
   `docs/plans/paddle-provider-v2.md` Step 4). Search by `name` directly
   before calling `CreateDiscountGroup`.
3. **`NotificationSetting`**: no `custom_data` field, no known uniqueness
   constraint. Best-effort only — match on `destination`+`type` (the
   latter create-only/immutable) before calling
   `CreateNotificationSetting`. Document this as imprecise in the
   resource's schema description, same discipline
   `docs/guardrails/money-moving-actions-no-blanket-retry.md` requires
   for Adjustments' `reason`-based match.
4. Unit tests for each entity's matching logic (fabricate a list/search
   response containing a match, confirm `Create()` short-circuits without
   calling the mutating client method). Acceptance test: extend each
   resource's existing `TestAccPaddle*_basic` with an invoke-twice check,
   or add a dedicated step — confirmed against the real sandbox, not just
   asserted from the unit-tested matching logic in isolation, same
   standard Step 1 item 5 holds `paddle_adjustment` to.
5. Regenerate docs if any schema-visible behavior changes (unlikely —
   this is create-path logic, not a new attribute — but confirm rather
   than assume).

## Definition of done for this plan

- Steps 0-7 marked `done`, or explicitly deferred with a reason (not
  silently skipped).
- `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...` all
  clean with no `TF_ACC` set.
- `TF_ACC=1 go test ./... -run TestAcc` passes against the sandbox for
  every resource and every new action, old and new.
- Step 0's no-retry path confirmed via unit test to make exactly one
  attempt against a simulated 5xx/timeout.
- Search-before-invoke confirmed both by unit test (short-circuit logic,
  Steps 1-2 item 5) and by at least one real-sandbox invoke-twice test per
  action family (`paddle_adjustment`, and `paddle_subscription_cancel` at
  minimum) proving only one mutation actually happened.
- `required_version` present and consistent across all three tracked
  locations (Step 4).
- `tfplugindocs generate` produces no diff (or the plan's Step 1/2 item 6
  question about action-doc generation support is resolved one way or
  the other, not left ambiguous).
- README safety section (Step 6) present.
- Step 7's search-before-create check confirmed for all five existing
  `Create*` calls, both by unit test (matching logic) and by a real
  invoke-twice-confirm-once sandbox test per resource.
- Every commit carries `Refs:` trailers per
  `docs/skills/commit-with-kms-attribute.md`.
