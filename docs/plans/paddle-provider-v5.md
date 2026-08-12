---
title: Implementation plan — terraform-provider-paddle v5
status: not started — 2026-08-12. Interviewed and scoped via /kms:roadmap
  at the close of the v0.5.0 release session; every decision below was
  made with the user before any code was written. Ships as v0.6.0 (see
  the version-numbering note below).
date: 2026-08-12
tags: [paddle, provider, plan, v5, pii, timeouts, data-sources, testing]
---

# Implementation plan: terraform-provider-paddle v5

**Read this whole file before doing anything.** Written to be
self-sufficient for a completely fresh session with zero prior context —
same convention as `docs/plans/paddle-provider-v1.md` through
`paddle-provider-v4.md`, which this plan follows on from.

Repo: `/home/ubuntu/projects/vivantel/terraform-provider-paddle`
(GitHub: `vivantel/terraform-provider-paddle`).

**Version-numbering note**: `paddle-provider-v5.md` is this plan's name in
the informal per-generation sequence (`v1`...`v5`) `docs/plans/` files
use — it does **not** mean the release tag is `v5.0.0`. The actual semver
tag this plan ships as is `v0.6.0` (the repo is still pre-1.0; v4's plan
shipped as `v0.5.0`, not `v4.0.0` — same pattern here).

## Why this plan exists

Scoped at the close of the session that shipped `v0.5.0`
(`docs/plans/paddle-provider-v4.md`, all steps done, real-sandbox-verified,
tagged and Registry-confirmed). Two things prompted it directly:

1. A live debugging session investigating leftover `sweep.yaml` failures
   (real leaked sandbox transactions) surfaced that this provider's HTTP
   client has a single hardcoded 60s timeout with zero user-facing
   override anywhere — a real architecture gap, independent of the
   sweeper bugs that prompted finding it (the sweeper doesn't go through
   Terraform's resource lifecycle at all, so a `timeouts{}` block
   wouldn't have fixed *that* night's bug, but the underlying gap is real
   for actual `terraform apply`/`destroy` users).
2. A closing "what else improves plugin maturity and UX" review — done
   *right after* `v0.5.0` shipped, the same review-immediately-after-
   release pattern v3→v4 and v4→v5 both used — surfaced, among other
   ideas, a real PII-warning gap in what had just shipped hours earlier:
   `paddle_events`' `data` field can carry customer PII exactly like
   `paddle_customer` does, but never got the same warning treatment.

A `/kms:roadmap` interview (2026-08-12) turned both into one decision
record, plus a separate technical decision for the one piece that changes
existing client behavior:

- `docs/decisions/0012-v5-scope-pii-data-sources-timeouts-testing.md` —
  read it in full before writing any code. Records every scope decision
  below, including the ones argued through (whether `timeouts{}` was
  worth doing at all; whether to stage this across multiple releases;
  whether `paddle_customers` plural was worth the PII-compounding
  concern) and why they landed where they did.
- `docs/decisions/0013-configurable-timeouts-architecture.md` — the
  `timeouts{}` feature's actual technical design: caller-wins precedence,
  the 30m ceiling, all four CRUD ops, the 60s default, and why real-sandbox
  verification doesn't work for this one feature specifically.

Supporting artifacts:

- `docs/facts/0007-replay-endpoint-and-timeouts-module-confirmed.md` —
  the real, confirmed API/module shapes Steps 2 and 5 are designed
  against (the notification replay endpoint, the
  `terraform-plugin-framework-timeouts` module's recommended pattern).
  Don't re-derive these from scratch; they're already verified.
- `docs/guardrails/pii-bearing-data-sources-need-state-security-warning.md`
  (updated this session) — governs Step 1's `paddle_events` fix and the
  audit's PII-vector checklist exactly. Read the whole file, not just the
  new paragraphs — the original guardrail's reasoning still applies.
- `docs/guardrails/configurable-timeouts-need-a-hard-ceiling.md` —
  governs Step 2's 30m cap.
- `docs/guardrails/mock-tests-supplement-not-replace-acceptance-tests.md`
  — governs Step 3's testing-infrastructure work; read this before
  writing a single mock test, since its whole point is preventing a
  category of mistake (treating mock coverage as sufficient on its own)
  that's easy to fall into by default.

Read all of these before making any judgment call not spelled out below.

## Ground truth before you start

- `master` is the default branch, currently at `v0.5.0` (tag pushed,
  Registry-ingested, verified — see `docs/plans/paddle-provider-v4.md`'s
  final status and its own Definition of Done checklist for the evidence
  trail). Branch from `master`.
- Existing resources (`internal/provider/*_resource.go` +
  `*_data_source.go`): `paddle_product`, `paddle_price`, `paddle_discount`,
  `paddle_discount_group`, `paddle_notification_setting` (each with a data
  source, each with real-sandbox-verified CRUD+import). Existing lookup-
  only data sources: `paddle_checkout_domain`, `paddle_subscription`,
  `paddle_transaction`, `paddle_customer`, `paddle_events`,
  `paddle_notification` — all singular/"exactly one match" shaped except
  `paddle_events`, which already returns a list (`events` attribute) since
  "exactly one event" isn't a meaningful concept the way "exactly one
  subscription" is; **no `paddle_events` plural variant needed in this
  plan for that reason.** Existing actions
  (`internal/provider/actions/*.go`): `paddle_adjustment`,
  `paddle_subscription_cancel`/`pause`/`resume`/`charge`.
- `internal/client/client.go` already has `ListSubscriptionsFiltered`,
  `ListTransactionsFiltered`, `ListNotificationsFiltered` — each already
  supports a `Limit` field (added in v4, see
  `docs/plans/paddle-provider-v4.md`'s post-review-fixes section) used by
  the *singular* data sources to detect 0/1/many matches cheaply. Step 4's
  new *plural* data sources should call these same methods with
  `Limit: 0` (unlimited) instead of introducing new list methods — check
  what's already there before writing a duplicate.
- `internal/client/client.go` does **not** yet have a customer-listing
  method beyond `ListCustomersByEmail` (single-filter, used by the
  singular `paddle_customer` data source). Step 4's `paddle_customers`
  plural data source needs a more general `ListCustomersFiltered`-shaped
  method (mirroring the other three) — check the real `GET /customers`
  filter support (confirmed in `docs/facts/0006-...`: `email` exact-match
  comma-separated, `status`) before assuming it supports the same filter
  shape as the others; don't guess from adjacency.
- `internal/client/client.go`'s `do()`/`doNoRetry()` (~line 167 as of
  `v0.5.0`) unconditionally wrap every call in
  `context.WithTimeout(ctx, retryOverallBudget)` where
  `retryOverallBudget = 60 * time.Second` is a package-level `var`. Step 2
  changes this to be conditional — see
  `docs/decisions/0013-configurable-timeouts-architecture.md`'s
  Consequences section for exactly what "conditional" means here (only
  impose the default if the incoming context has no deadline of its own).
- Every established pattern from v1-v4 still applies:
  `IsNull()`+`IsUnknown()` before `Value*()`, `stringvalidator.OneOf` on
  enums, unit tests for pure `toAPI*`/`fromAPI*` functions, acceptance
  tests confirmed against the real sandbox before calling anything done
  (`docs/decisions/0003-acceptance-tests-against-live-sandbox.md`),
  `tfplugindocs generate` + `git diff --exit-code -- docs/` clean before
  any push (`docs/guardrails/docs-must-be-regenerated-before-merge.md`),
  the "no filter set" guard pattern every v4 singular data source has
  (`internal/provider/lookup_guard.go`) — Step 4's plural data sources
  need the analogous guard too (empty filter set = list-everything,
  which is a legitimate use case for a *plural* data source unlike the
  singular ones, so this guard's shape differs: a plural data source
  should probably just warn in its schema description about the cost of
  an unfiltered list, not hard-error the way the singular ones do — a
  judgment call to make explicitly when writing it, not silently copy the
  singular pattern).
- **This session had `PADDLE_API_KEY` available via CI** (unlike the v4
  session's start) — real-sandbox verification via CI's `acceptance` job
  is expected to actually happen for each step, not deferred. Don't
  repeat the "implement + unit test only" pattern v4 started with unless
  a fresh session genuinely has no sandbox access.

## How to update this file as you work

Same convention as v1-v4: each step has a `Status:` line, update in place
as you go. Commit via `docs/skills/commit-with-kms-attribute.md`, with
`Refs:` trailers to the decision/fact/guardrail files each step
implements. Regenerate docs via `tfplugindocs generate` after any schema
change and confirm `git diff --exit-code -- docs/` locally before
pushing. Open each step as its own PR against `master` and confirm CI
(`build`/`lint`/`docs`/`acceptance`) green before merging — the pattern
that caught every real bug during the v0.5.0 session was small PRs with
real CI runs, not one giant branch.

---

## Step 1: PII fix + full audit

Status: not started. Depends on: none.

**Do this first** — it's the fastest step and closes a real gap in
already-shipped code, not just new work.

1. `internal/provider/events_data_source.go`: add the same PII/state-
   security warning `paddle_customer` has to `paddle_events`' schema
   `MarkdownDescription`, worded for an opaque/variable-shape field per
   the updated guardrail (`data` *can* carry PII depending on event type,
   not "this field is PII"). Don't just copy `paddle_customer`'s wording
   verbatim — read the updated guardrail's new paragraphs first, they
   spell out exactly how the wording should differ.
2. `README.md`: extend the PII section (added for `paddle_customer` in
   v4) to also cover `paddle_events`, or add a clearly-linked second
   subsection — match whichever the guardrail's own "Applies to" section
   implies is cleaner once you're looking at the real current README
   structure.
3. **Full audit pass**: read every schema in `internal/provider/
   *_resource.go` and `*_data_source.go`, plus every action in
   `internal/provider/actions/*.go`, checking two things per attribute:
   (a) could this carry customer PII (email, name, address, tax ID, or
   equivalent) that a user wouldn't expect to persist into state, and (b)
   if it's PII or a secret (API keys, webhook signing secrets), is
   `Sensitive: true` set? `notification_setting_resource.go`'s
   `endpoint_secret_key` is a known candidate to check first (a secret,
   not PII, but the same `Sensitive` question applies). Record findings
   as you go; fix `Sensitive` gaps as trivial one-line schema changes;
   for any newly-found PII-bearing field, apply the same warning pattern
   as `paddle_events` above and add it to the guardrail's "Applies to"
   list.
4. Update `docs/guardrails/pii-bearing-data-sources-need-state-security-warning.md`'s
   "Applies to" section with whatever the audit actually found (it
   currently lists `paddle_customer`, `paddle_events`, and a forward
   reference to `paddle_customers` plural from Step 4 — update those
   placeholders once Step 4's data source actually exists).

**Definition of done**: `paddle_events`' warning is real (schema +
README), the audit is documented (even a short paragraph in this step's
own `Status:` line saying what was checked and found, so a future session
doesn't have to redo it blind), any `Sensitive` gaps found are fixed,
`go test ./...` and `golangci-lint run ./...` clean, `tfplugindocs
generate` clean.

## Step 2: configurable `timeouts{}` block

Status: not started. Depends on: none, but should land before Step 3
(the mock-server harness is partly justified by this step's own
verification need).

Read `docs/decisions/0013-configurable-timeouts-architecture.md` in full
before starting — every design decision below is explained there, this
is the "what to actually change" version.

1. Add `github.com/hashicorp/terraform-plugin-framework-timeouts` to
   `go.mod` (`go get` it, don't hand-edit `go.mod`/`go.sum`).
2. `internal/client/client.go`: change `do()` and `doNoRetry()`'s
   unconditional `context.WithTimeout(ctx, retryOverallBudget)` to only
   apply when `ctx` (the incoming one) has no deadline of its own
   (`if _, ok := ctx.Deadline(); !ok { ... }`). Unit-test this directly —
   a fake context with a pre-set deadline should pass through unchanged;
   one without should get `retryOverallBudget` applied. This is the one
   piece of this step that changes existing behavior; get it right and
   tested before touching any resource.
3. New shared helper (suggest `internal/provider/timeouts.go`) —
   something like
   `func resolveTimeout(ctx context.Context, configured timeouts.Value, op string, default_ time.Duration) (context.Context, context.CancelFunc, diag.Diagnostics)`
   that reads the configured value (via the `timeouts` module's own
   `.Create()`/`.Read()`/`.Update()`/`.Delete()` accessors, each taking a
   default), applies the 30m ceiling
   (`docs/guardrails/configurable-timeouts-need-a-hard-ceiling.md`), and
   returns a derived context ready to pass to client calls. One shared
   helper, not five copy-pasted implementations — the guardrail
   explicitly calls this out.
4. Add a `timeouts` attribute (`timeouts.Attributes(ctx, timeouts.Opts{
   Create: true, Read: true, Update: true, Delete: true})`, per the
   confirmed-real pattern in
   `docs/facts/0007-replay-endpoint-and-timeouts-module-confirmed.md`) to
   all five resource schemas, and wire each resource's
   `Create`/`Read`/`Update`/`Delete` to call the Step 2.3 helper and pass
   the resulting context down to its client call(s).
5. Unit tests: the `do()`/`doNoRetry()` conditional-timeout logic (2
   above), the shared helper's ceiling-enforcement logic (a configured
   value under 30m passes through, one over gets clamped, unset uses the
   60s default) — these are pure enough to unit test without a mock
   server.
6. Mock-server test (needs Step 3's harness, or build a minimal one-off
   here if Step 3 hasn't landed yet and reorder later): a deliberately
   slow/hanging `httptest` handler, configure a short `timeouts{}` value
   in a resource's config, confirm the operation actually times out at
   roughly the configured value, not 60s. This is the verification
   `docs/decisions/0013-...`'s "Consequences" section calls out as
   impossible against the real sandbox — this test *is* the proof, not a
   supplement to a sandbox test.

**Definition of done**: all five resources have working `timeouts{}`
support, the precedence change is unit-tested, the ceiling is enforced
and tested, a mock-server test proves a configured timeout actually
fires, `tfplugindocs generate` picks up the new schema attribute
automatically, real-sandbox acceptance tests for all five resources still
pass unchanged (proving the default-60s behavior-preservation claim in
`docs/decisions/0013-...`).

## Step 3: mock-server test harness + retrofit

Status: not started. Depends on: none structurally, but naturally
overlaps Step 2 (which needs a slice of this harness regardless).

Read `docs/guardrails/mock-tests-supplement-not-replace-acceptance-tests.md`
in full first — this step's whole purpose is bounded by that guardrail.

1. Build a small, reusable `httptest.Server`-backed test helper (suggest
   `internal/provider/mockserver_test.go` or similar) that stands up a
   fake Paddle API a resource's acceptance-test-style config can run
   against via `testAccProtoV6ProviderFactories`-equivalent wiring, but
   pointed at the mock server's URL instead of the real sandbox. Look at
   `internal/provider/sweep_test.go`'s existing `httptest.NewServer`
   usage (`TestSweepMatchingProducts_LogsMatchedAndSweptCount`) as the
   closest existing precedent in this codebase, even though that one
   tests a sweeper function directly rather than a full resource through
   the Plugin Framework — this step's harness needs to go one level
   deeper (through `resource.Test`) to actually exercise a resource's
   `Create`/`Read`/`Update`/`Delete` methods, not just a client method.
2. Retrofit all five existing resources
   (`product`/`price`/`discount`/`discount_group`/`notification_setting`)
   with mock-based CRUD tests covering their basic lifecycle — not a
   replacement for their existing real-sandbox acceptance tests
   (`*_resource_acc_test.go` files stay exactly as they are), an
   additional, faster-running test file alongside them.

**Definition of done**: the harness is genuinely reusable (Step 2's
timeout test and at least one retrofit both use it, proving it's not
a one-off), all five resources have mock-based CRUD coverage, existing
real-sandbox acceptance tests are untouched and still pass, every new
test file's naming makes it obvious at a glance which kind of test it is
(mock vs. real sandbox) — don't let this be ambiguous from the filename
alone.

## Step 4: four plural/list data sources

Status: not started. Depends on: none.

Same structure as v4's singular data sources
(`internal/provider/subscription_data_source.go` etc.) but returning a
list instead of requiring exactly one match — closer to
`internal/provider/events_data_source.go`'s existing shape (a `Computed`
list attribute of nested objects) than to the singular ones.

1. `internal/provider/subscriptions_data_source.go` (plural) — filters:
   `customer_id`/`status` (same as the singular one), `Limit: 0` against
   the existing `ListSubscriptionsFiltered`.
2. `internal/provider/transactions_data_source.go` (plural) — filters:
   `subscription_id`/`customer_id`/`status`, against
   `ListTransactionsFiltered`.
3. `internal/provider/notifications_data_source.go` (plural) — filters:
   `notification_setting_id`/`status`, against
   `ListNotificationsFiltered`. Decide whether to include each
   notification's delivery logs (`paddle_notification`'s `logs` nested
   list) in the plural version too, or leave that to the singular lookup
   only — a real N+1-calls-per-result cost if included (one
   `ListNotificationLogs` call per notification returned), worth an
   explicit decision here rather than copying the singular schema
   blindly.
4. `internal/provider/customers_data_source.go` (plural) — filter:
   `email` (same exact-match semantics as the singular one, confirmed in
   `docs/facts/0006-...`). **This is the one with the PII-compounding
   concern from Step 1/the updated guardrail** — its schema
   `MarkdownDescription` needs the "returns multiple customers' PII" 
   wording the guardrail update specifies, not a copy of the singular
   `paddle_customer` warning. Needs a new `ListCustomersFiltered` client
   method (see "Ground truth" above).
5. Register all four in `internal/provider/provider.go`'s
   `DataSources()`. Update `internal/provider/actions_wiring_test.go`'s
   `len(resp.DataSourceSchemas)` expected count (currently 11 as of
   `v0.5.0` — will need to become 15).
6. Unit tests for each `fromAPI*`/list-conversion function. Acceptance
   tests against the real sandbox for each — reuse existing fixtures
   (`PADDLE_TEST_SUBSCRIPTION_ID` etc.) where possible rather than
   provisioning new ones.

**Definition of done**: all four data sources real-sandbox-verified via
CI's `acceptance` job, `tfplugindocs generate` produces the four new doc
files, `actions_wiring_test.go`'s count updated and passing.

## Step 5: `paddle_notification_replay` action

Status: not started. Depends on: none, but natural to do after Step 4
(shares context with the notification data sources).

Read `docs/decisions/0012-v5-scope-pii-data-sources-timeouts-testing.md`
item 4 and `docs/facts/0007-replay-endpoint-and-timeouts-module-confirmed.md`
first — confirms the endpoint shape and the explicit decision *not* to
add search-before-invoke protection here.

1. `internal/client/client.go`: add `ReplayNotification(ctx, id string)
   (*Notification, error)` — `POST /notifications/{id}/replay`, returns
   the *new* notification's data per the confirmed real response shape.
   Uses the regular retry-wrapped `do()`, not `doNoRetry` — unlike the
   money-moving actions, a replay isn't dangerous to retry (see decision
   0012 item 4's reasoning) — but confirm this against the real API
   reference for the endpoint's own idempotency characteristics before
   assuming, don't purely infer from the "low stakes" framing without
   checking.
2. `internal/provider/actions/action_paddle_notification_replay.go` —
   single `notification_id` config field, no search-before-invoke check
   (deliberately, per the decision). Follow the existing action file
   structure (`action_paddle_adjustment.go` or the subscription actions
   as a template for the schema/`Invoke` shape, even though this action
   is simpler than any of those).
3. Register in `provider.go`'s `Actions()`.
4. Unit tests for the request/response shape. Acceptance test against
   the real sandbox — needs an existing notification to replay; reuse
   whatever fixture Step 4's `paddle_notification`/`paddle_notifications`
   acceptance tests already establish (or the permanent
   `notification_setting` fixture documented in `README.md`'s
   `paddle_notification` precondition section, added in the v0.5.0
   session) rather than provisioning a new one.

**Definition of done**: real-sandbox-verified replay (confirm via the
Paddle dashboard or `GET /notifications` that a genuinely new
notification entity was created, not just that the action didn't error),
`tfplugindocs generate` picks up the new action doc.

## Step 6: docs

Status: not started. Depends on: Steps 2, 4, 5 (documents features those
steps build).

1. `examples/lookup-then-act/main.tf` — a real, complete example: look up
   a subscription via `paddle_subscription` (or the new plural
   `paddle_subscriptions`), feed its `id` into
   `action.paddle_subscription_cancel`; look up a transaction via
   `paddle_transaction`, feed its `line_items[0].item_id` into
   `action.paddle_adjustment` — the same pattern
   `TestAccPaddleTransactionDataSource_feedsAdjustment`
   (`internal/provider/transaction_data_source_acc_test.go`) already
   proves works, now as a real, readable example a human would actually
   look at, not test code.
2. `README.md`: a short pointer to the new example from the Actions
   section, near the existing usage example.
3. `README.md` or a new `docs/guides/timeouts.md` (check whether
   `tfplugindocs`' guide-generation convention is already used anywhere
   in this repo before picking the location — if not, README is the
   simpler, already-established choice): hand-written usage note for
   `timeouts{}` — when you'd actually want to configure one (a slow
   catalog operation under real load, mirroring the sweeper's own
   real-world motivation), and a reminder of the 30m ceiling.

**Definition of done**: the example file is real, working HCL (not just
prose) — apply it against the sandbox once by hand to confirm, don't
just eyeball it for syntax. README changes committed.

## Step 7: housekeeping and release

Status: not started. Depends on: Steps 1-6 all done and real-sandbox-
verified.

1. `CHANGELOG.md`: `[0.6.0]` entry via `/kms:changelog`
   (`docs/skills/release-with-kms-changelog.md`), generated from the real
   commit history since `v0.5.0`, not hand-written from memory.
2. Confirm `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test
   ./...`, `golangci-lint run ./...` all clean on final `master`.
3. Confirm `tfplugindocs generate` produces no diff.
4. Tag and push `v0.6.0` (annotated tag, matching `v0.4.0`/`v0.5.0`'s own
   convention — check `git tag -l -n99 v0.5.0` for the exact message
   style to match).
5. Watch `release.yaml` (triggered by the tag push) to completion, confirm
   the GitHub release is real (`gh release view v0.6.0 --json isDraft,
   isPrerelease` — both must be `false`).
6. Confirm Registry ingestion for real — `registry-smoke-test.yaml`
   triggers automatically via `workflow_run` after `release.yaml`
   completes; confirm its log shows `Installing vivantel/paddle v0.6.0`
   specifically (not a cached older version).
7. Independently confirm too, not just via CI logs — a real
   `terraform init`/`validate` in a scratch directory against
   `version = "0.6.0"`, exercising at least one schema from each new
   surface this plan added (a plural data source, the replay action, a
   `timeouts{}` block) — the same standard
   `docs/plans/paddle-provider-v4.md`'s own final steps set, don't skip
   it just because CI already did something similar.

**Definition of done**: `v0.6.0` tagged, released (non-prerelease),
Registry-confirmed, independently smoke-tested — all with real command
output as evidence, not just "should be fine."

## Definition of done for this plan

- Steps 1-7 all have their own `Status: done` with real verification
  evidence (test output, PR/CI-run URLs), not "implemented" alone.
- `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`,
  `golangci-lint run ./...` all clean.
- `tfplugindocs generate` produces no diff (`git diff --exit-code --
  docs/`).
- Every new resource change (timeouts support) and every new data
  source/action verified against the real sandbox via CI's `acceptance`
  job — not just the new mock-server tests, per
  `docs/guardrails/mock-tests-supplement-not-replace-acceptance-tests.md`.
- `CHANGELOG.md` has a `[0.6.0]` entry.
- Tagged and pushed as `v0.6.0` (not `v5.0.0`), release verified
  non-prerelease, Registry ingestion confirmed, and a real
  `terraform init`/`validate` smoke test run independently (not just
  read from CI logs) against the actual published artifact.
