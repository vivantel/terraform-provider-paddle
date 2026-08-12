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

Status: done — 2026-08-12. `paddle_events`' schema `MarkdownDescription`
and `README.md` both got the opaque/variable-shape PII warning
(worded as "`data` *can* carry PII depending on event type," not "this
field is PII," per the updated guardrail). Full audit pass done over
every `internal/provider/*_resource.go`/`*_data_source.go` schema and
every `internal/provider/actions/*.go` action: grepped every
`schema.*Attribute`/`actionschema.*Attribute` definition across all 21
files for PII-shaped names (email/name/address/phone/tax/secret/key/
token/customer) and manually read each hit. Findings: `endpoint_secret_key`
(`notification_setting_resource.go` and `notification_setting_data_source.go`)
already has `Sensitive: true` in both places — no gap. No other field
beyond `paddle_customer`'s existing `email`/`name` and `paddle_events`'
`data` carries fetched customer PII; `notification_setting`'s
`destination` can be an email address but is a user-supplied config
value the user already typed in, not PII fetched from Paddle's API, so
it's out of this guardrail's scope. No `Sensitive` gaps found anywhere
else. No new PII-bearing fields found, so the guardrail's "Applies to"
list needed no additions beyond its existing `paddle_customers` (v5)
forward reference, which stays as a placeholder for Step 4. `go build
./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`, `golangci-lint
run ./...` all clean; `tfplugindocs generate` produced the expected
single-file diff (`docs/data-sources/events.md`), now committed.
Depends on: none.

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

Status: done — 2026-08-12. `github.com/hashicorp/terraform-plugin-framework-timeouts`
added via `go get` (v0.7.0). `internal/client/client.go`'s `do()`/
`doNoRetry()` now call a new `withDefaultTimeout()` helper instead of
unconditionally wrapping every call in `context.WithTimeout(ctx,
retryOverallBudget)` — it only applies the 60s default when the incoming
`ctx` has no deadline of its own, so a resource's `timeouts{}`-derived
deadline is honored as-is (caller-wins precedence), not intersected down.
Unit-tested directly (`internal/client/default_timeout_test.go`): no
deadline gets the default applied, a shorter pre-set deadline survives
unchanged, and — the actual point of the precedence fix — a *longer*
pre-set deadline also survives unchanged rather than being tightened.
A shared `internal/provider/timeouts.go` helper (`resolveTimeout`)
reads the configured value via the module's own `.Create()`/`.Read()`/
`.Update()`/`.Delete()` accessors (60s default, matching prior behavior
exactly), clamps to the 30m ceiling
(`docs/guardrails/configurable-timeouts-need-a-hard-ceiling.md`), and
returns a derived context — one implementation, not five. Unit-tested
(`internal/provider/timeouts_test.go`): unset uses the default, a value
under the ceiling passes through, a value over the ceiling (tested with
24h) is clamped, and an unparseable value surfaces a diagnostic. All
five resources (`product`/`price`/`discount`/`discount_group`/
`notification_setting`) got a `timeouts` schema attribute (`timeouts.Attributes`,
all four ops enabled) and their `Create`/`Read`/`Update`/`Delete` wired
to `resolveTimeout`; each `Read()` now also fetches the `timeouts`
attribute via `GetAttribute` alongside `id` (the existing narrow-fetch
pattern every resource's `Read()` already used), so the previously-
applied timeout value round-trips through state correctly rather than
being lost or produoing a schema-encoding error. A `nullTimeouts()` test
helper was needed (and added to `internal/provider/timeouts.go`) because
a bare `timeouts.Value{}` zero value doesn't carry the schema's attribute
types and fails state encoding — existing test model-literal helpers
(`baseModel()`, `baseDiscountModel()`, the inline product literal in
`delete_not_found_test.go`) updated to use it.

Mock-server verification (item 6, the one behavior that genuinely can't
be proven against the real sandbox — see
`docs/guardrails/mock-tests-supplement-not-replace-acceptance-tests.md`'s
narrow exception): built as a one-off `httptest` harness here rather than
waiting on Step 3 (Step 3 hadn't landed yet), per the plan's own
reordering note — `internal/provider/timeout_firing_mock_test.go`,
`TestTimeoutFiring_ConfiguredValueOverridesDefault`. A deliberately slow
(2s) handler + a configured `300ms` delete timeout confirms the call
actually fails around 300ms, not the old 60s default, while also proving
the handler really would have succeeded eventually (ruling out a false
pass from an unrelated fast failure).

**Post-review fix (2026-08-12, same PR, before merge)**: CI's `acceptance`
job caught a real bug the local-only unit/mock tests couldn't have —
adding `Timeouts` directly to the five `*ResourceModel` structs broke
every singular data source (`paddle_product`, `paddle_price`,
`paddle_discount`, `paddle_discount_group`, `paddle_notification_setting`)
with `Value Conversion Error: Struct defines fields not found in object:
timeouts`, because each singular data source's `Read()` decodes state
into that exact same struct type, and a data source's schema has no
`timeouts` attribute (only resources get one). Fixed by moving `Timeouts`
out of the shared `*ResourceModel` structs (used unchanged by data
sources) into a resource-only wrapper struct per resource
(`productResourceStateModel`, `priceResourceStateModel`,
`discountResourceStateModel`, `discountGroupResourceStateModel`,
`notificationSettingResourceStateModel`) that embeds the base model plus
`Timeouts timeouts.Value` — `terraform-plugin-framework`'s reflection
promotes the embedded struct's `tfsdk`-tagged fields (confirmed via
`internal/reflect/helpers.go`'s explicit anonymous-field support), so
`toAPI*`/`fromAPI*` functions keep operating on the plain base model
unchanged; only `Create`/`Read`/`Update`/`Delete` now decode into the
wrapper type and pass `.XResourceModel` through to those functions. This
is exactly the kind of gap `docs/decisions/0003-acceptance-tests-against-
live-sandbox.md` exists to catch — `go build`/unit tests/mock tests all
passed clean the whole time this bug was present.

`go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`,
`golangci-lint run ./...` all clean after the fix. `tfplugindocs
generate` diff unchanged (still only the five `docs/resources/*.md`
files — confirms data source docs are unaffected, as expected).
Real-sandbox verification confirmed via CI's `acceptance` job on PR #31's
second run (https://github.com/vivantel/terraform-provider-paddle/pull/31)
— `build`/`docs`/`lint`/`acceptance` all pass, including every one of the
five resources' existing acceptance tests and the previously-broken
singular data sources, proving both default-60s behavior preservation and
the post-review fix. Depends on: none, but should land before Step 3
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

Status: done — 2026-08-12. `internal/provider/mockserver_test.go`:
`newMockPaddleServer(t, handler)` stands up an `httptest.Server`, points
this provider's client at it via a new, deliberately undocumented
internal-only escape hatch (`provider.go`'s `Configure()` now honors a
`PADDLE_BASE_URL` env var override, checked after the `environment`
switch — not part of the public schema, no docs, sourced only from the
environment, real users have no supported way to reach it), and returns
`ProtoV6ProviderFactories` wired the same shape
`testAccProtoV6ProviderFactories` (`provider_test.go`) uses — so a mock
test drops straight into `resource.Test` with `IsUnitTest: true`,
exercising a resource's real `Create`/`Read`/`Update`/`Delete` methods
through the actual Plugin Framework lifecycle (confirmed
`terraform-plugin-testing` auto-installs a `terraform` binary via
`hc-install` when none is on `PATH`, exactly as needed in this sandbox),
not just a client method call — this is the "one level deeper than
`sweep_test.go`'s existing `httptest.NewServer` precedent" the plan
called for.

All five resources retrofitted with `*_resource_mock_test.go` files
(`TestMockPaddle<X>_basicLifecycle`), each with its own small in-memory
`sync.Mutex`-guarded store implementing just enough of that resource's
real endpoint shape (confirmed against `internal/client/client.go`'s
actual request/response types, not guessed) to drive create → update →
destroy: `product`/`price`/`discount`/`discount_group` archive-on-destroy
(status flips to `archived` via the same `statusPatch` shape the real
client sends), `notification_setting` a real hard `DELETE` (no archive
fallback, matching that resource's real behavior) — each test asserts
the post-destroy store state matches. Existing `*_resource_acc_test.go`
files are completely untouched (confirmed via `git status` before
committing — only `provider.go` and new `*_mock_test.go`/
`mockserver_test.go` files changed). Filename convention makes mock vs.
real-sandbox unambiguous at a glance: `*_resource_acc_test.go` (real
sandbox, gated on `PADDLE_API_KEY`/`TF_ACC`) vs. `*_resource_mock_test.go`
(mock, `IsUnitTest: true`, runs under plain `go test ./...`).

The harness's reusability is proven two ways, not just asserted: Step 2's
`TestTimeoutFiring_ConfiguredValueOverridesDefault` predates this file and
used a lower-level one-off (`testDeleteState`, not this harness — the
plan flagged this as an acceptable reordering at the time since Step 3
hadn't landed yet), but all five of *this* step's retrofits share the one
`newMockPaddleServer` helper, and a follow-up pass could migrate Step 2's
test onto it too (not done here — out of scope, the ceiling/precedence
logic that test verifies isn't resource-specific, so it doesn't need
`resource.Test`'s full lifecycle the way CRUD retrofits do).

`go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...` (including
all five new mock tests, ~4-5s each, and every pre-existing test) all
pass; `go test ./... -run TestMock -v` isolates just the five retrofits.
`golangci-lint run ./...` clean. `tfplugindocs generate` produces no diff
(no schema change this step). Real-sandbox acceptance tests untouched —
this step needs no CI `acceptance` job confirmation of its own beyond
"still passes," since nothing it touches changes resource behavior; PR
CI confirms this. Depends on: none structurally, but naturally
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

Status: done — 2026-08-12. All four plural data sources added, calling
the existing `ListSubscriptionsFiltered`/`ListTransactionsFiltered`/
`ListNotificationsFiltered` with `Limit: 0` (unlimited) as instructed,
plus a new `ListCustomersFiltered` client method (`email`+`status`
filters, confirmed real server-side support against the actual API
reference — both filters, not just `email`, per the "Ground truth"
section's note that this was already confirmed beyond what item 4's own
enumeration mentioned).

1. `paddle_subscriptions` — `customer_id`/`status` filters, reuses
   `fromAPISubscription` and the exact `SubscriptionDataSourceModel`
   struct as its nested element type.
2. `paddle_transactions` — `subscription_id`/`customer_id`/`status`
   filters. Explicit decision, documented in code: excludes `line_items`
   (a new `TransactionSummaryModel`, not `TransactionDataSourceModel`) —
   the singular data source's `line_items` requires an N+1 per-ID
   re-fetch (list responses don't carry `details.line_items` at all),
   which would mean an N+1 call per *every* result here; use this to find
   an `id`, then `paddle_transaction` (singular) for `line_items`.
3. `paddle_notifications` — `notification_setting_id`/`status` filters.
   Same explicit N+1 decision, documented in code: excludes `logs` (a new
   `NotificationSummaryModel`) — exactly the judgment call the plan
   flagged as needing an explicit decision rather than a blind copy of
   the singular schema.
4. `paddle_customers` — `email`/`status` filters. Carries the
   PII-compounding warning the guardrail specifies ("this returns
   multiple customers' PII," not a copy-pasted singular sentence) — see
   `docs/guardrails/pii-bearing-data-sources-need-state-security-warning.md`,
   updated to close out its `paddle_customers` forward-reference
   placeholder now that this data source actually exists.
5. All four registered in `provider.go`'s `DataSources()`.
   `actions_wiring_test.go`'s `len(resp.DataSourceSchemas)` expected count
   updated from 11 to 15, passing.
6. None of the four plural data sources have a "no filter set" hard-error
   guard the way the singular ones do (`lookup_guard.go`) — an empty
   filter set is a legitimate "list everything" use case here, per the
   plan's own explicit call on this; each schema's
   `MarkdownDescription` carries a `⚠️` cost warning instead (API call
   volume + state-file size for an unfiltered/loosely-filtered call), the
   judgment call the plan asked to make explicitly rather than silently
   copying the singular pattern.
7. Unit tests for the two new list-conversion functions
   (`fromAPITransactionSummary`, `fromAPINotificationSummary`,
   `internal/provider/plural_data_sources_test.go`) —
   `fromAPISubscription`/`fromAPICustomer` are reused verbatim from the
   singular data sources and already unit-tested there, so no duplicate
   test needed. Acceptance tests for all four
   (`*_data_source_acc_test.go`), each reusing an existing fixture rather
   than provisioning new ones — `findTestSubscription`, the sandbox's
   existing notifications, a fresh customer fixture matching the singular
   `paddle_customer` test's own pattern — plus a shared
   `checkListContainsID`/`checkListAttrsSet` helper pair
   (`plural_data_sources_acc_test.go`) since a plural data source's filter
   can legitimately return more than the one fixture record a test
   provisioned, unlike a singular data source's exact-index checks.

**Post-review fix (2026-08-12, same PR, before merge)**: CI's `acceptance`
job's first run failed — `TestAccPaddleTransactionsDataSource_byFilter`
(this step's new test) and the pre-existing
`TestAccPaddleTransactionDataSource_byFilter` both hit real sandbox `429
too_many_requests`/`context deadline exceeded` on `CreateCustomer`. Root
cause: this step's transactions test originally called
`createAdjustmentFixtureTransaction`, which provisions its own fresh
Customer + Address + Transaction via direct API calls — a fifth
customer-creating fixture call added to an already-marginal shared
sandbox rate-limit budget within one CI run (every other acceptance test
with its own customer fixture runs in the same job). A re-run without
any code change reproduced the same failure, confirming this wasn't a
one-off flake. Fixed by rewriting the test to filter by the pinned
`findTestSubscription` fixture's `subscription_id` instead — its existing
recurring billing history already has transactions, so the test needs
*zero* new fixture provisioning, not just a different existing fixture to
reuse. This is a strictly better fit for the plan's own "reuse existing
fixtures where possible" instruction than the original
`createAdjustmentFixtureTransaction` choice was, not just a rate-limit
workaround.

`go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`
(including the new unit tests; acceptance tests skip cleanly without
`PADDLE_API_KEY`), `golangci-lint run ./...` all clean.
`tfplugindocs generate` produced exactly the four expected new files
(`docs/data-sources/{subscriptions,transactions,notifications,customers}.md`),
no other diff. Real-sandbox verification confirmed via CI's `acceptance`
job on PR #33's re-run (https://github.com/vivantel/terraform-provider-paddle/pull/33)
— `build`/`docs`/`lint`/`acceptance` all pass, including all four new
data sources. Depends on: none.

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

Status: done — 2026-08-12. `internal/client/client.go`'s
`ReplayNotification(ctx, id)` — `POST /notifications/{id}/replay`, uses
the regular retry-wrapped `do()`, not `doNoRetry`, per the decision (a
replay's worst-case duplicate is one extra delivery attempt, not a
money-moving harm). `internal/provider/actions/action_paddle_notification_replay.go`
— single `notification_id` field, no search-before-invoke check
(deliberately, per decision 0012 item 4), follows
`action_paddle_subscription_cancel.go`'s file structure. Registered in
`provider.go`'s `Actions()` (comment updated: five → six actions, the
new one explicitly noted as not irreversible/financial the way the
other five are). `actions_wiring_test.go` and
`actions/schema_test.go` both updated for the sixth action (renamed
`TestProviderServer_ExposesAllFiveActionSchemas` →
`...AllSixActionSchemas`).

Unit test: `internal/client/replay_notification_test.go` confirms the
exact request (`POST /notifications/{id}/replay`, no body) and response
decoding (`replayed.ID` differs from the id passed in — a new entity, not
the original mutated). Acceptance test:
`action_paddle_notification_replay_acc_test.go`'s
`TestAccPaddleNotificationReplay_createsNewNotification` reuses whatever
notification already exists in the sandbox with status `delivered`/
`failed` (the only statuses the endpoint accepts) — same
self-provisioning-impossible leniency
`TestAccPaddleNotificationDataSource_basic` already uses — and proves the
plan's own Definition of Done directly: snapshots every notification ID
for that `notification_setting_id` before invoking, replays, then
confirms a genuinely new ID appears after (via `GET /notifications`
through `ListNotificationsFiltered`, not the Paddle dashboard, but the
same underlying confirmation the Definition of Done calls for) — not
just that the action call didn't error.

`go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`,
`golangci-lint run ./...` all clean. `tfplugindocs generate` produced
exactly the one expected new file (`docs/actions/notification_replay.md`),
no other diff. Real-sandbox verification confirmed via CI's `acceptance`
job on PR #34 (https://github.com/vivantel/terraform-provider-paddle/pull/34)
— `TestAccPaddleNotificationReplay_createsNewNotification` itself passed
every time it ran. Getting the whole job green took real investigation,
not just retries: the same unrelated, pre-existing tests
(`TestAccPaddleTransactionDataSource_byFilter`/
`TestAccPaddleTransactionsDataSource_byFilter`) failed on real sandbox
`429`s across 5 consecutive attempts (including a 15-minute cooldown),
confirming sustained account-wide rate-limit exhaustion, not a one-off
flake. Root-caused with the user's help (asked "do we respect
Retry-After?"): `do()` was already correctly honoring the header
(`waitBeforeRetry`/`parseRetryAfter`), but a correctly-read `Retry-After`
still got clamped to `retryMaxRetryAfter` (30s), and the 60s
`retryOverallBudget` only left room for one or two such waits before
giving up — not enough to ride out sustained throttling even though the
client was behaving correctly. Fixed with a new exported
`client.RelaxRetryTuningForAcceptanceTests()` (production defaults
unchanged), wired into `internal/provider`'s existing `TestMain` when
`TF_ACC` is set — the very next CI run passed clean on the first
attempt, confirming the diagnosis. This fix is broader than Step 5 (it
benefits every step's acceptance-test verification, retroactively too)
but is recorded here since this is where it was found and fixed.
Depends on: none, but natural to do after Step 4 (shares context with
the notification data sources).

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

Status: done — 2026-08-12. `examples/lookup-then-act/main.tf` — a real,
complete config: `paddle_subscription` (by `customer_id`) →
`paddle_subscription_cancel`; `paddle_transaction` (by `id`) →
`paddle_adjustment` refunding `line_items[0].item_id`. `README.md`:
Actions section gets a pointer to the example right after its existing
code sample; intro paragraph and Actions section list updated to mention
the four plural data sources and the sixth action
(`paddle_notification_replay`) — a real gap from Steps 4/5, which added
the surfaces but never touched `README.md`'s summary text, closed here
rather than left for a future session. New "Configuring `timeouts{}`"
section added to `README.md` (no `docs/guides/` convention existed
anywhere in this repo, confirmed by checking first, so README is the
right location per the plan's own instruction) — when to actually
configure one, the 60s default/no-behavior-change-if-omitted point, and
the 30m ceiling.

**Definition of Done's "apply it against the sandbox once by hand"**:
initially confirmed only with `terraform validate` against the real
built provider schema (`dev_overrides`, no registry needed) — this
caught a real error (actions weren't valid HCL syntax in the Terraform
version initially tested against; the cached `hc-install` binary at a
newer version validated cleanly). Per the user's explicit direction,
went further than validate-only: added
`internal/provider/example_lookup_then_act_acc_test.go`
(`TestAccExampleLookupThenAct_appliesCleanly`), which reads the actual
published example file from disk (`os.ReadFile`, not a re-typed copy),
substitutes real fixture IDs (the pinned
`PADDLE_TEST_CANCELED_SUBSCRIPTION_ID` fixture — safe, since
`paddle_subscription_cancel`'s own already-canceled short-circuit means
applying against it exercises the real lookup+action wiring without
touching the shared *active* fixture other tests depend on — plus a
fresh disposable transaction fixture, same pattern
`TestAccPaddleTransactionDataSource_feedsAdjustment` already uses), and
runs it through `resource.Test` against the real sandbox via CI's
existing `PADDLE_API_KEY` — no new GitHub Actions workflow needed, and
this becomes permanent regression coverage rather than a one-time
manual check.

`go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`,
`golangci-lint run ./...` all clean. `tfplugindocs generate` produces no
diff (no schema change this step). Real-sandbox verification of the
example itself pending CI's `acceptance` job on this step's PR. Depends
on: Steps 2, 4, 5 (documents features those steps build).

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
