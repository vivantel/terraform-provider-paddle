---
title: Implementation plan — terraform-provider-paddle v3
status: shipped as v0.4.0-beta.1 — 2026-08-10. Merged via PR #14, CI
  acceptance job passed against the real sandbox (one real finding fixed:
  the adjustment fixture now skips cleanly on a missing default-payment-
  link account precondition, docs/plans/paddle-provider-v3.md Step 5).
  Tagged and published to the Terraform Registry; Registry Smoke Test
  passed a real apply/destroy through the published binary, and a manual
  real-Registry terraform init/validate/plan (no dev_overrides) confirmed
  all five actions' schemas work through the actual published artifact.
  Beta, not stable: subscription actions' real success paths
  (pause/resume, charge) still need a human to provision a real sandbox
  subscription via checkout before they can be exercised — see Step 5.
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
  lower-severity gap in already-shipped `Create*` calls, investigated as
  this plan's Step 7 and resolved by analysis, not a code change — see
  that step's status for why the obvious fix would have introduced a
  worse risk than it solved.

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
- **Steps 0-6 are implemented as of 2026-08-10, Step 7 resolved by
  analysis** (see each step's own Status — read them before assuming
  anything below is still todo). Remaining real work: run the acceptance
  suite against the actual sandbox (blocked locally on no
  `PADDLE_API_KEY`; CI has the secret), which may surface bugs the way
  every prior resource's first real sandbox run in this repo's history
  did — don't assume "implemented" means "verified." `provider.go`
  now implements `provider.ProviderWithActions`/`Actions()` (returns all
  five actions from `internal/provider/actions/`), alongside the
  pre-existing `Resources()`/`DataSources()`. The package layout mirrors
  Stripe's `internal/provider/actions/` (one file per action,
  `action_<name>.go`) — reference implementation fetched and read
  2026-08-10 from `github.com/stripe/terraform-provider-stripe` tag
  `v0.3.0-beta.3` (**not** `main`, which lags behind and doesn't have
  actions at all — confirmed by diffing the two) — but this provider's
  actions are hand-designed against real Paddle field lists, not
  spec-generated; Stripe's generated-code patterns (e.g. its
  `assignStringToNamedFieldOrMethod` reflection helpers) weren't copied,
  they solve a code-generation problem this provider doesn't have.
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

Status: done — 2026-08-10. `internal/client/client.go`'s single-attempt
logic (request build, send, decode) extracted from `do()`'s retry loop
into a shared `doOnce()` helper (no behavior change to `do()` — every
existing `TestDo_*` test still passes unmodified, confirming the
refactor). New `doNoRetry(ctx, method, path, body, out) error` calls
`doOnce()` exactly once. A transport-level failure or a retryable-status
response (429/5xx — ambiguous, Paddle may have processed the request
before failing to respond) is wrapped in a new `*NonRetryableError`
(`Unwrap()`-compatible, so `errors.As` against the underlying `*APIError`
still works through it); a clean non-retryable 4xx (Paddle definitively
rejected the request, nothing ambiguous) is returned as a plain `*APIError`
unchanged, not wrapped — deliberately narrower than "any non-2xx" so a
validation error doesn't get a misleading "verify before retrying"
message. Unit tests in `internal/client/no_retry_test.go` confirm exactly
one HTTP attempt on 5xx/429/transport failure (vs. `do()`'s
multi-attempt behavior on the same inputs), the plain-vs-wrapped error
distinction, and successful decode-into-`out`. `go build`/`go vet`/
`gofmt -l .`/`go test ./...` all clean.

Not yet wired to any caller — `paddle_adjustment`/subscription actions
(Steps 1-2) are what actually calls `doNoRetry`; nothing does yet, so
this step alone has no user-visible effect.

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
`CreatePrice`, etc.) — see `docs/guardrails/catalog-creates-lack-dedup-check.md`
and this plan's Step 7, which investigated it and resolved it by analysis
rather than a code change (the obvious fix would have introduced a worse
risk than it solved) — see Step 7's own status for the full account.

## Step 1: `paddle_adjustment` action

Status: implemented, sandbox-verification pending — 2026-08-10. Real field
list confirmed against the live API reference (not the plan's original
guess): `action` has 7 enum values, not just refund/credit
(`credit`/`refund`/`chargeback`/`chargeback_reverse`/`chargeback_warning`/
`chargeback_warning_reverse`/`credit_reverse`) — schema validator uses the
full set. No dedicated GET-by-id exists; `list-adjustments` (with an `id`
filter) is the closest equivalent, used here for search-before-invoke
instead. `internal/client/client.go` gained `Adjustment`/`AdjustmentItem`
(request fields + `ID`/`Status` only, not the full response shape — see
its own comment), `CreateAdjustment` (via `doNoRetry`), `ListAdjustments`
(paginated, filtered by `transaction_id`, via regular `do()`).
`internal/provider/actions/action_paddle_adjustment.go`: schema mirrors
the confirmed fields; `findMatchingAdjustment` (pure, unit-tested)
implements search-before-invoke matching on `action`+`reason`(+`type` if
set); `Invoke()` distinguishes `*client.NonRetryableError` (ambiguous
outcome) from a plain rejection in its diagnostics message. Unit tests:
wire-shape tests in `internal/client/client_test.go`
(`TestAdjustmentJSON_*`), pagination/filter test in `list_test.go`,
matching-logic and conversion tests in
`internal/provider/actions/action_paddle_adjustment_test.go`. Acceptance
test written (Step 5:
`TestAccPaddleAdjustment_basic`, invoke-twice-confirm-once against a real
fixture transaction) but not yet run against the sandbox this session —
see Step 5's status. Docs generated (`docs/actions/adjustment.md`, via
`tfplugindocs generate` — see Step 1 item 6).

What *was* verified this session despite no `PADDLE_API_KEY` being
available: `go build`/`go vet`/`gofmt -l .`/`go test ./...` all clean;
and — going beyond what the plan itself asked for — a real dev-override
build (`go build`, `~/.terraformrc` `dev_overrides`) was exercised against
a real Terraform 1.15.8 binary: `terraform validate` and `terraform plan`
both succeed against valid config (plan correctly previews the action
invocation with all config values, confirming the "does `terraform plan`
show what an action will do before apply" question this plan's own
predecessor conversation raised as a safety concern), and a deliberately
invalid config (`action = "not_a_real_value"`, a missing required
`effective_from` on another action, `quantity = 0`) is correctly rejected
by the schema validators with the expected error messages. This confirms
the schema and provider-server wiring are correct; it does not confirm
`Invoke()`'s actual HTTP calls against Paddle, which needs a real
`PADDLE_API_KEY` and is what "sandbox-verification pending" refers to.

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
6. Docs: resolved — `tfplugindocs generate` supports actions natively
   (confirmed by running it, 2026-08-10: produced `docs/actions/*.md` for
   all five actions unprompted, correctly rendering each schema
   including the nested `items` block). No hand-writing needed.

## Step 2: subscription lifecycle actions (cancel, pause, resume, charge)

Status: implemented, sandbox-verification pending — 2026-08-10. Real field
lists confirmed against the live API reference for all four endpoints
(cancel/pause/resume/charge), correcting several of this plan's original
guesses:

- **cancel/pause**: `effective_from` enum is `immediately`/
  `next_billing_period` (not `at_end_of_period` as an earlier draft
  guessed) — made **Required** in both actions' schemas with no
  client-side default, even though Paddle defaults to
  `next_billing_period` server-side if omitted, so an irreversible
  immediate cancellation/pause is never an implicit choice.
- **pause**: also takes `resume_at` (RFC 3339, optional — omit for
  indefinite) and `on_resume` (optional enum).
- **resume**: `effective_from` is required by Paddle's own API (not just
  optional-with-a-default) — modeled as Required here too, plus optional
  `on_resume`.
- **charge**: real request shape is far more involved than this plan
  originally assumed — `items` supports three variants (a catalog price,
  a non-catalog price against an existing product, or a fully inline
  product+price), and **`custom_data` is only available on the two
  non-catalog variants, not on the catalog-price variant or at the
  top level** — the plan's original assumption that a deterministic
  `custom_data` key would work universally for search-before-invoke was
  wrong for the dominant real-world case. **Scope narrowed deliberately**:
  `paddle_subscription_charge` supports catalog prices only
  (`price_id`+`quantity`) — the other two variants are a materially
  bigger schema-design task, deferred rather than half-modeled (see the
  action's own schema description). Search-before-invoke instead lists
  this subscription's `origin=subscription_charge` transactions
  (`ListSubscriptionChargeTransactions`, new narrowly-scoped read-only
  client method — not the start of broader Transaction support, see its
  own comment) and matches on an exact `price_id`+`quantity` item-set
  comparison (`sameChargeItems`/`findMatchingCharge`, pure, unit-tested).
  This is a known-weaker check than a synthetic key would have been: two
  deliberately separate charges for identical items look identical to it
  — documented explicitly in the action's schema description, not
  overstated as airtight.

`internal/client/client.go` gained `Subscription` (`ID`/`Status` only),
`GetSubscription` (via regular `do()` — a read is retry-safe),
`SubscriptionCancelRequest`/`PauseRequest`/`ResumeRequest`/`ChargeRequest`
+ `SubscriptionChargeItem`, and
`CancelSubscription`/`PauseSubscription`/`ResumeSubscription`/`ChargeSubscription`
(all via `doNoRetry`), plus `Transaction`/`TransactionItem` (read-only,
narrowly scoped) and `ListSubscriptionChargeTransactions`.
`internal/provider/actions/subscription_status.go`'s
`checkAlreadyInTargetState` (shared by cancel/pause/resume, unit-tested
including the exact "canceled is not active" case
`docs/guardrails/money-moving-actions-no-blanket-retry.md` calls out) —
each action still owns its own mutating call and progress/error
messaging. Four action files, each following
`action_paddle_adjustment.go`'s `NonRetryableError`-vs-plain-rejection
diagnostics pattern. Unit tests: wire-shape tests in
`client_test.go` (`TestSubscription*RequestJSON_*`), a filter/pagination
test in `list_test.go`
(`TestListSubscriptionChargeTransactions_FiltersByOriginAndSubscriptionID`),
status-check tests in `subscription_status_test.go`, and matching/
conversion tests in `action_paddle_subscription_charge_test.go`.
Acceptance tests written (Step 5:
`internal/provider/action_paddle_subscription_acc_test.go`, four tests
covering the invalid-ID error path, the already-canceled short-circuit,
the pause/resume round trip, and the charge invoke-twice proof) but not
yet run against the sandbox this session. Docs generated
(`docs/actions/subscription_{cancel,pause,resume,charge}.md`).

Same dev-override `terraform validate`/`plan` verification described in
Step 1's status was run against all four of these actions together (one
config exercising all five actions via a shared `action_trigger`) — plan
correctly previewed every action's config, and deliberately-invalid input
(a missing required `effective_from` on resume, `quantity = 0` on a
charge item) was correctly rejected with clear errors. See Step 1's
status for the full verification narrative; not repeated here.

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
   - `charge`: this item's original plan (a `custom_data`-embedded
     deterministic key) turned out not to fit the shipped, catalog-only
     item scope — `custom_data` only exists on the two non-catalog item
     variants this action deliberately doesn't support, found during
     implementation. **Correction 1 (2026-08-10)**: shipped as an exact
     `price_id`+`quantity` item-set match against
     `ListSubscriptionChargeTransactions` (`origin=subscription_charge`)
     instead — weaker than a synthetic key (two deliberately separate
     charges for identical items look identical to it), documented as
     such. **Correction 2 (2026-08-11, found running the real sandbox
     acceptance test)**: that transaction search only works for
     `effective_from="immediately"` — Paddle creates no queryable
     transaction at all for `"next_billing_period"` until the
     subscription renews, so the check silently never fired for that
     input. Fixed: `effective_from="next_billing_period"` now checks
     `GetSubscriptionNextTransaction` (`?include=next_transaction`)
     instead. See `docs/guardrails/money-moving-actions-no-blanket-retry.md`
     for the full account — this shipped broken in `v0.4.0-beta.1` for
     one release before the real-sandbox acceptance test standard this
     plan itself requires (item 5 below) caught it.
     **Correction 3 (2026-08-11, found running the follow-up acceptance
     test added to close out correction 2)**: correction 2's fix was
     itself non-functional, for two compounding reasons —
     `NextTransactionPreview.Items` was decoded from a top-level
     `"items"` key that doesn't exist on the real response (the real
     items are under `details.line_items`), and even once that's fixed,
     the matching logic required an exact item-set match against a
     preview that always mixes the subscription's own recurring items in
     with any queued charge, so it could never match. This let a real
     duplicate charge through into the sandbox subscription's next
     renewal, confirmed by a diagnostic raw-JSON read against the actual
     account. Fixed: `NextTransactionPreview` gained a custom
     `UnmarshalJSON` reading `details.line_items`, and
     `findMatchingScheduledCharge` now uses a new subset-containment
     matcher (`containsChargeItems`) instead of the exact-match one
     `findMatchingCharge` correctly uses for `"immediately"`. Full
     account, including why the resulting sandbox duplicate was left to
     bill and be swept rather than force-removed (no API to cancel a
     single queued charge exists), in
     `docs/guardrails/money-moving-actions-no-blanket-retry.md`.
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
6. Docs: resolved — see Step 1 item 6. `tfplugindocs generate` produced
   `docs/actions/subscription_{cancel,pause,resume,charge}.md`
   unprompted, no hand-writing needed.

## Step 3: `Actions()` on the provider

Status: done, verified beyond what a fresh session could assume — 2026-08-10.
`provider.go`: added `var _ provider.ProviderWithActions = &PaddleProvider{}`,
`resp.ActionData = c` in `Configure()` alongside the existing
`ResourceData`/`DataSourceData`, and `Actions()` returning all five new
actions. Top-level provider `Description` updated to mention actions.
Verified with a real protocol-level test
(`internal/provider/actions_wiring_test.go`,
`TestProviderServer_ExposesAllFiveActionSchemas`) that builds the actual
proto server via the existing `testAccProtoV6ProviderFactories` and calls
`GetProviderSchema` — no network call, so it runs without
`PADDLE_API_KEY`/`TF_ACC` — confirming all five action type names are
registered with no diagnostics, and that `ResourceSchemas`/
`DataSourceSchemas` counts are unaffected (5 and 6 respectively). Also
confirmed via the real `terraform validate`/`plan` dev-override run
described in Step 1's status — the actions actually show up correctly in
`terraform plan`'s "Actions to be invoked" preview through the real proto
wire format, not just via this provider's own Go-level tests.

Implements: [[0010-v3-scope-lifecycle-actions]].

1. `internal/provider/provider.go`: add `var _
   provider.ProviderWithActions = &PaddleProvider{}` and implement
   `Actions(_ context.Context) []func() action.Action`, returning the
   five actions from Steps 1-2. Model directly on the existing
   `Resources()`/`DataSources()` methods for style consistency. (Done —
   line numbers shift as the file grows, check current line numbers
   rather than trusting any cited here.)
2. `Schema()`: no attribute changes needed for actions
   themselves, but confirm nothing about the provider-level schema
   assumes only resources/data sources exist.

## Step 4: `required_version` floor

Status: done — 2026-08-10. `required_version = ">= 1.14.0"` added to the
`terraform {}` block in `examples/provider/provider.tf`, `README.md`'s
Usage section, and `examples/full-stack/main.tf` — the same three
locations `docs/guardrails/example-version-constraints-track-latest-minor.md`
already tracked for `required_providers`; that guardrail extended to
cover `required_version` too. `terraform fmt -check -diff` clean on both
`.tf` files. The real dev-override `terraform validate`/`plan` run (Step
1's status) used `required_version = ">= 1.14.0"` itself and succeeded
against the actual installed Terraform 1.15.8 — confirms the constraint
string is syntactically valid and not overly strict for the version this
was tested against.

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

Status: implemented and run for real against the sandbox, 2026-08-11 —
found and fixed several real bugs in the process (see this file's own
git history for the full sequence: fixture customer needs a name, address
needs full fields, `ListAdjustments`' `per_page` cap, a Config-built-
before-PreCheck timing bug, and — most seriously — `paddle_subscription_charge`'s
search-before-invoke failing to prevent a real duplicate charge due to
Paddle read-after-write lag, now mitigated with a retry loop). **Real,
permanent invoices are generated by this test suite** — Paddle's manual
-collection "billed" transactions are legal invoice records with no
delete API, and every real run of `TestAccPaddleAdjustment_basic`/
`TestAccPaddleSubscriptionCharge_roundTrip` creates one. Sweeper cleanup
added: `sweepTestFixtureCustomers` now also cancels/credits each swept
customer's transactions before archiving the customer; a new
`sweepTestSubscriptionCharges` sweeper (gated on
`PADDLE_TEST_SUBSCRIPTION_ID`) sweeps every `subscription_charge`-origin
transaction against the pinned test subscription unconditionally (it's a
dedicated fixture by definition, no naming filter needed). Both wired
into `.github/workflows/sweep.yaml`.

Original status text below, kept for the record — 2026-08-10. Simulations API researched and **rejected**:
confirmed it only produces webhook test events, not real queryable
transaction/subscription objects — not a usable fixture source. Two
different real constraints found, requiring two different strategies:

- **`paddle_adjustment` CAN self-provision**: a manually-collected
  transaction can be created via a direct `POST /transactions` call and
  set to `billed` status immediately, with no real checkout/payment
  needed (confirmed against the real API reference) — satisfies
  create-adjustment's "completed, or billed/past_due" requirement. Needs
  a Customer + Address first (`customer_id`/`address_id` are required in
  practice to reach a billable status, even though the schema marks them
  nullable). `internal/client/client.go` gained a new "Test fixture
  support only" section: `Customer`/`Address`/`TransactionCreate` +
  `CreateCustomer`/`CreateAddress`/`CreateTransaction`, explicitly
  documented as not managed resources and not a reopening of
  [[0010-v3-scope-lifecycle-actions]]'s deferred Customer/Address
  decision (the PII-in-state concern that decision raised doesn't apply
  to disposable fixture data that never touches Terraform state).
  `ListTestFixtureCustomers`/`ArchiveTestFixtureCustomer` +
  a new `paddle_test_fixture_customer` sweeper
  (`internal/provider/sweep_test.go`) clean these up, matching on
  `"acctest"` in the email (not `isAccTestName`'s `"acc test"` — an email
  local-part can't contain a space).
  `internal/provider/action_paddle_adjustment_acc_test.go`:
  `TestAccPaddleAdjustment_basic` provisions customer→address→product→
  price→transaction, then invokes the action twice (via a
  `terraform_data` `after_create`+`after_update` trigger, forcing a
  second invocation by changing `input` between steps — the same
  mechanism used for Step 1/2's invoke-twice-confirm-once requirement)
  and confirms exactly one matching adjustment exists after each.
- **Subscription actions CANNOT self-provision**: confirmed (web search,
  2026-08-10) — Paddle subscriptions can only be created via a real
  checkout + test card, no pure-API path exists at all, even in sandbox.
  Same constraint `docs/plans/paddle-provider-v2.md` Step 6 hit for
  `paddle_checkout_domain`, and the same fix: `ListSubscriptions` (new,
  test-only purpose) looks up whatever already exists, tests skip
  cleanly if nothing matches. Given `cancel`'s irreversibility
  ("can't be undone"), acceptance tests deliberately **never invoke
  cancel against a real active subscription** — that would permanently
  consume a scarce, manually-provisioned sandbox fixture for every future
  test run. Split into what's actually safe to automate:
  - `TestAccPaddleSubscriptionCancel_invalidSubscriptionID` — always
    runs, no fixture needed, confirms the real error path.
  - `TestAccPaddleSubscriptionCancel_alreadyCanceledShortCircuits` — the
    safe half of cancel's coverage: finds an already-canceled
    subscription (if any) and confirms invoking cancel against it
    succeeds via the short-circuit, not by erroring.
  - `TestAccPaddleSubscriptionPauseResume_roundTrip` — pause/resume are
    reversible, so this is real success-path coverage: pauses a found
    active subscription, confirms status, resumes it, confirms status
    again.
  - `TestAccPaddleSubscriptionCharge_roundTrip` — invokes charge twice
    against a found active subscription (self-provisioning its own
    product/price fixture, so "matching items" is unambiguous), confirms
    exactly one matching charge transaction exists after each.
  All four in `internal/provider/action_paddle_subscription_acc_test.go`.

**Manual precondition, not yet done**: none of the subscription tests'
success paths have ever actually run against a real sandbox subscription,
because this repo's sandbox account may not have one — same situation
`paddle_checkout_domain` was in until a human manually added a checkout
domain via the dashboard
(`docs/plans/paddle-provider-v2.md` Step 6). Provisioning at least one
active sandbox subscription via a real checkout (test card) is a
manual, human step outside this plan's ability to automate — needed
before `TestAccPaddleSubscriptionPauseResume_roundTrip`/
`TestAccPaddleSubscriptionCharge_roundTrip` can confirm anything beyond
"skips cleanly." Documented in Step 6's README section.

**Second manual precondition, found the hard way**: the PR's real CI
`acceptance` run (2026-08-10, `feat/v3-lifecycle-actions`) confirmed
every subscription test skips cleanly as designed — and surfaced a real,
unanticipated blocker for `TestAccPaddleAdjustment_basic`: Paddle
rejects *any* transaction creation via the API — even the fully manual,
non-checkout fixture this test builds — with
`transaction_default_checkout_url_not_set` until the sandbox account has
a default payment link configured (dashboard → Checkout → default pay
link). Not documented anywhere in the API reference research this plan
did beforehand; only surfaced by actually running against the real
sandbox, exactly the class of finding `docs/decisions/0003-acceptance-tests-against-live-sandbox.md`
exists to catch. Fixed by having the fixture helper skip cleanly with a
clear message on this specific error code, same treatment as the
subscription tests' missing-fixture case, rather than failing CI for an
account-configuration reason unrelated to the code. Documented in
`README.md`'s Actions section, right after the subscription-fixture
note.

`go build`/`go vet`/`gofmt -l .`/`go test -c` all clean; every new
acceptance test confirmed to skip correctly (via `TF_ACC` or
`PADDLE_API_KEY` gating, or its own dynamic lookup) when run locally
without credentials — see this step's own commit for the actual output.

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

Status: done — 2026-08-10. New `README.md` "Actions — refunds, credits,
and subscription lifecycle operations" section (right after "Checkout
domains"): lists all five actions, explains no-`paddle_subscription`
-resource reasoning, the no-idempotency-key risk and this provider's
no-blanket-retry + search-before-invoke mitigations (with the known
`paddle_subscription_charge` false-positive case called out explicitly,
not glossed over), the `-auto-approve`/plan-review recommendation (citing
this repo's own `e2e.yaml` as the pattern *not* to copy for
action-containing configs), the separate-scoped-API-key recommendation, a
usage example, and the subscription-fixture manual-precondition note tying
back to Step 5. `.github/workflows/e2e.yaml` itself untouched — it
doesn't exercise any action, so Step 6 item 2's "don't add a new example
under `-auto-approve` without a comment" doesn't apply yet; noted for
whenever that changes.

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

Status: **resolved by analysis — no code change** — 2026-08-10.

Implements: `docs/guardrails/catalog-creates-lack-dedup-check.md` (see
that file's own 2026-08-10 revision for the full account — summarized
here).

Started implementing this step's original plan (a `custom_data`-embedded
deterministic key for Product/Price/Discount, a `name`/`destination`
list-and-match for Discount Group/Notification Setting, mirroring Steps
1-2's search-before-invoke) and stopped partway through once two real
problems surfaced — this is a case of the roadmap-phase plan not
surviving contact with implementation, caught before shipping rather than
after:

1. **Injecting a synthetic key into `custom_data` pollutes a field users
   already control** (the v2 `custom_data` retrofit exposed it as a real,
   user-facing attribute). A provider-injected value would show up in
   Paddle's own dashboard/API for that object — a real, visible side
   effect, not an implementation detail.
2. **"Adopt a found match into Terraform state" is a materially worse
   risk for a resource than "skip, no-op" is for an action.** An action
   has no state to corrupt; a resource's `Create()` populating state from
   a *coincidentally* name-matching but actually unrelated pre-existing
   object means a later `terraform destroy` could archive something this
   config never created. Names/destinations are user-chosen strings, not
   scoped identifiers — this isn't a theoretical risk.
3. **No server-side filter exists** for any of the candidate search
   fields (checked both list endpoints' real query parameters) — a
   search means an unbounded client-side list-and-scan, tolerable for
   low-cardinality Discount Groups/Notification Settings but a real
   performance/cost regression for Products/Prices/Discounts on any
   catalog with more than a handful of objects.

Re-examining the actual failure mode this guardrail was written to
prevent closed the gap without any of that: **for Discount Groups, Paddle
already enforces server-side name uniqueness** — the "silent duplicate"
scenario cannot happen for this entity at all. A retried `Create` after
an ambiguous failure surfaces a clear `409
discount_group_name_conflict` (the exact error this project already
diagnosed and fixed once, in `.github/workflows/e2e.yaml`'s own history)
— a confusing-but-self-diagnosing error, not a silent problem, and
`FriendlyErrorMessage` already surfaces it clearly. Product/Price/
Discount/Notification Setting have no such constraint and can genuinely
duplicate on a retried `Create` — accepted as a real but low-severity,
not-worth-the-new-risk-to-fix gap (an extra catalog object, never money
movement).

No unit tests, no acceptance tests, no docs regeneration needed for this
step — nothing in `internal/provider/*_resource.go` or
`internal/client/client.go`'s existing `Create*` methods changed.

## Definition of done for this plan

- Steps 0-7 marked `done` (or, for Step 7, resolved with a documented
  reason not requiring code), not silently skipped.
- `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...` all
  clean with no `TF_ACC` set.
- `TF_ACC=1 go test ./... -run TestAcc` passes against the sandbox for
  every resource and every new action, old and new.
- Step 0's no-retry path confirmed via unit test to make exactly one
  attempt against a simulated 5xx/timeout.
- Search-before-invoke confirmed both by unit test (short-circuit logic,
  Steps 1-2) and by real-sandbox proof: `paddle_adjustment` and
  `paddle_subscription_charge` each get an explicit invoke-twice-within-
  one-test proof; `paddle_subscription_cancel`/`pause`/`resume` get
  real-sandbox coverage of the short-circuit/round-trip paths instead
  (see Step 5's status for why an invoke-twice test isn't safe to write
  for `cancel` specifically).
- `required_version` present and consistent across all three tracked
  locations (Step 4).
- `tfplugindocs generate` produces no diff (or the plan's Step 1/2 item 6
  question about action-doc generation support is resolved one way or
  the other, not left ambiguous).
- README safety section (Step 6) present.
- Step 7 resolved (documentation only, per its own status) — not treated
  as a missing code change.
- Every commit carries `Refs:` trailers per
  `docs/skills/commit-with-kms-attribute.md`.
