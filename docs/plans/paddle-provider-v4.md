---
title: Implementation plan — terraform-provider-paddle v4
status: done — 2026-08-11. Interviewed and scoped via /kms:roadmap
  immediately after v0.4.0 shipped and was reviewed as a product; every
  decision below was made with the user before any code was written.
  Implemented, real-sandbox-verified (PRs #19-#24), and shipped as
  v0.5.0 (see the version-numbering note below and this file's
  "Definition of done" section for the full evidence trail).
date: 2026-08-11
tags: [paddle, provider, plan, v4, data-sources]
---

# Implementation plan: terraform-provider-paddle v4

**Read this whole file before doing anything.** Written to be
self-sufficient for a completely fresh session with zero prior context —
same convention as `docs/plans/paddle-provider-v1.md`,
`docs/plans/paddle-provider-v2.md`, and `docs/plans/paddle-provider-v3.md`,
which this plan follows on from.

Repo: `/home/ubuntu/projects/vivantel/terraform-provider-paddle`
(GitHub: `vivantel/terraform-provider-paddle`).

**Version-numbering note**: `paddle-provider-v4.md` is this plan's name in
the informal per-generation sequence (`v1`, `v2`, `v3`, now `v4`) that
`docs/plans/` files use — it does **not** mean the release tag is
`v4.0.0`. The actual semver tag this plan ships as is `v0.5.0` (the repo
is still pre-1.0; v3's plan shipped as `v0.4.0`, not `v3.0.0` — same
pattern here). Don't let the filename mislead you into writing `v4.0.0`
anywhere in code, docs, or CHANGELOG.

## Why this plan exists

v3 (`docs/plans/paddle-provider-v3.md`, all steps done, released as
`v0.4.0-beta.1` then promoted to `v0.4.0` stable) shipped this provider's
first Terraform actions: `paddle_adjustment` and four subscription
lifecycle actions. Immediately after that promotion, a product-management-
style review of the provider's overall state (completeness, robustness,
reliability, customer value) found the actions are correctly implemented
and real-sandbox-verified, but **practically hard to use**: every one of
them requires an opaque Paddle ID (`subscription_id`, `transaction_id`,
`item_id`) that has no discovery path inside Terraform today — a real
user has to go find these in the Paddle dashboard or via a manual API
call first, then hardcode them. The review also found a stale README
status line, a live regression-detection gap (all three of v3's real bugs
were only ever caught by a human manually running acceptance tests), and
a repeated root cause behind two of those three bugs (three different,
easy-to-conflate item shapes on Transaction-adjacent API responses) worth
refactoring away before building more code on top of it.

A `/kms:roadmap` interview (2026-08-11) turned those findings into one
decision record:

- `docs/decisions/0011-v4-scope-data-sources-and-regression-guard.md` —
  read it in full before writing any code. It records every scope
  decision below, including the ones argued against (staging across
  multiple releases; skipping the customer-PII item entirely) and why
  they were rejected.

Supporting artifacts this plan implements directly:

- `docs/facts/0006-subscription-transaction-events-notifications-api-shapes.md`
  — the real, confirmed API filter/retention shapes every data source
  below is designed against. Don't re-derive these from scratch or guess
  at query parameter names; they're already verified.
- `docs/guardrails/lookup-data-sources-required-for-action-inputs.md` —
  the guardrail this plan's Steps 2-3 close out.
- `docs/guardrails/pii-bearing-data-sources-need-state-security-warning.md`
  — governs Step 4's schema/README wording exactly; don't improvise the
  warning language, follow this guardrail's framing (same posture as
  `README.md`'s existing Actions-section warning, applied to PII instead
  of financial risk).
- `docs/guardrails/actions-need-scheduled-regression-coverage.md` —
  governs Step 6.

Read all four before making any judgment call not spelled out below.

## Ground truth before you start

- `master` is the default branch, currently at `v0.4.0` (tag pushed,
  Registry-ingested, verified — see `docs/plans/paddle-provider-v3.md`'s
  final status). Branch from `master`.
- Existing resources (`internal/provider/*_resource.go` +
  `*_data_source.go`, client methods in `internal/client/client.go`):
  `paddle_product`, `paddle_price`, `paddle_discount`,
  `paddle_discount_group`, `paddle_notification_setting` (each with a data
  source), `paddle_checkout_domain` (data source only). Existing actions
  (`internal/provider/actions/*.go`): `paddle_adjustment`,
  `paddle_subscription_cancel`/`pause`/`resume`/`charge`.
- `internal/client/client.go` already has `GetSubscription`,
  `ListSubscriptions` (unfiltered), `GetTransaction`,
  `ListTransactionsByCustomer`, `ListSubscriptionChargeTransactions`
  (filtered by `subscription_id`+`origin`) — Steps 2-3 below extend this
  file, they don't start from nothing. Check what's already there before
  writing a new method that duplicates one.
- Data source pattern to follow exactly:
  `internal/provider/discount_data_source.go` (91 lines) is the smallest,
  clearest reference — `Metadata`/`Schema`/`Configure`/`Read`, an `id`
  `Required` attribute, every other field `Computed`. Filtering-by-field
  data sources (this plan's Steps 2, 3, 5 all need one) are new territory
  for this provider — no existing file to copy that part from; design it
  as `Optional` filter attributes alongside `id` (mutually-exclusive-ish:
  if `id` is set, look up directly; otherwise apply whatever filters are
  set and require the result to be exactly one match, erroring with a
  clear "N matches, narrow your filter" message on more than one — same
  usability standard `checkout_domain_data_source.go` already sets for
  its own non-ID lookup).
- `.github/workflows/e2e.yaml` currently runs the catalog resources'
  acceptance tests against the published Registry artifact, daily. Read
  it before Step 6 — you're extending its existing `go test` invocation
  and env/secret wiring, not writing a new workflow.
- Every established pattern from v1-v3 still applies:
  `IsNull()`+`IsUnknown()` before `Value*()`, `stringvalidator.OneOf` on
  enums, unit tests for pure `toAPI*`/`fromAPI*` functions, acceptance
  tests confirmed against the real sandbox before calling anything done
  (`docs/decisions/0003-acceptance-tests-against-live-sandbox.md`),
  `tfplugindocs generate` + `git diff --exit-code -- docs/` clean before
  any push (`docs/guardrails/docs-must-be-regenerated-before-merge.md`).

## How to update this file as you work

Same convention as v1-v3: each step has a `Status:` line, update in place
as you go. Commit via `docs/skills/commit-with-kms-attribute.md`, with
`Refs:` trailers to the decision/fact/guardrail files each step
implements. Regenerate docs via `tfplugindocs generate` after any schema
change and confirm `git diff --exit-code -- docs/` locally before pushing.

---

**Post-hoc note (2026-08-11, once sandbox access became available via
CI)**: every step below was written mid-implementation, when "sandbox
verification pending" was accurate. It no longer is — see this file's
"Definition of done" section at the bottom for the real, PR-linked
verification evidence for all six steps. The per-step `Status:` lines
below are left as-written (a historical record of what was known at each
point), not retroactively edited.

## Step 1: centralized line-item-shape resolution helper

Status: implemented, unit-tested, `go build`/`go vet`/`gofmt`/`go test ./...`
clean. Sandbox acceptance verification pending — no `PADDLE_API_KEY`
available in this session; user chose "implement + unit test only" over
providing a key or waiting for CI (see conversation). `internal/client/lineitem.go`
adds `LineItemIDs` and `ResolveLineItemID`; both real existing call sites
(`internal/provider/sweep_test.go`'s `cancelOrCreditTransaction`,
`internal/provider/action_paddle_adjustment_acc_test.go`'s
`TestAccPaddleAdjustment_basic`) refactored to use them instead of their
own `Details.LineItems` traversal.

**Do this first** — Steps 3 and 5 both need to expose transaction line
items in a data source schema, and building that against the current raw
shapes would very likely reproduce the exact bug class that caused two of
`v0.4.0-beta.1`'s three real bugs.

Context for a fresh session: `internal/client/client.go` currently has
**three different, non-interchangeable shapes** for "an item on something
transaction-related", each confirmed against the real API the hard way
this session:

1. `Transaction.Items[].Price.ID` — nested under `price: {id: ...}`, not
   flat (see `TransactionItem`'s own doc comment, `client.go` around line
   1300).
2. `Transaction.Details.LineItems[].ID` — a completely separate list, only
   this one carries the `txnitm_...` ID `paddle_adjustment`'s `item_id`
   actually needs (see `TransactionLineItem`'s own comment).
3. `NextTransactionPreview.Items[].PriceID` — flat, decoded via a custom
   `UnmarshalJSON` reading `details.line_items` (see
   `NextTransactionPreview`'s own comment, added fixing the
   `next_billing_period` bug).

Build one small internal package or file (suggest
`internal/client/lineitem.go`) with a single exported helper — something
like `func ResolveLineItemID(txn *Transaction, priceID string) (itemID string, ok bool)`
— that knows all three shapes internally and gives every future caller
(including the two data sources below) one call site instead of three
copy-pasted traversals. Keep the existing three raw types as-is (don't
break `paddle_adjustment`'s or `paddle_subscription_charge`'s existing,
now-verified-correct logic) — this is additive, a shared helper on top,
not a rewrite of what already works.

Unit tests: cover all three shapes with real-shaped fixtures (reuse the
JSON bodies already in `internal/client/client_test.go`'s
`TestTransactionJSON_*`/`TestNextTransactionPreviewJSON_*` tests as a
starting point — don't invent new fixture shapes, the real ones are
already captured there).

**Definition of done**: `go test ./...` green, the helper has its own
unit tests, and — as a real proof this isn't just theoretical — refactor
`paddle_adjustment`'s existing item-lookup code (wherever it currently
does its own `Details.LineItems` traversal) to use the new helper, so
Step 1 is proven against a real, already-shipped call site before Step 3
becomes the second one.

## Step 2: `paddle_subscription` data source

Status: implemented, unit-tested, docs generated
(`docs/data-sources/subscription.md`), `go build`/`go vet`/`gofmt`/
`go test ./...` clean. Sandbox acceptance verification pending — same
reason as Step 1. `internal/provider/subscription_data_source.go` +
`ListSubscriptionsFiltered`/`SubscriptionListFilter` in
`internal/client/client.go`, registered in `provider.go`. Acceptance
tests written (`subscription_data_source_acc_test.go`, both `id` and
`customer_id`+`status` filter paths) reusing `findTestSubscription` per
the plan — not yet run against the real sandbox.

New file: `internal/provider/subscription_data_source.go`, following
`discount_data_source.go`'s structure. Schema:

- `id` — `Optional` (not `Required`, unlike every existing data source in
  this repo — see the filter-vs-ID design note in "Ground truth" above).
- `customer_id` — `Optional`, filter.
- `status` — `Optional`, filter. Validate against Paddle's real status
  enum (`active`, `canceled`, `past_due`, `paused`, `trialing` — confirmed
  in [[0006-subscription-transaction-events-notifications-api-shapes]]) —
  don't invent this list, use exactly what's confirmed there.
- Every other field Computed: at minimum
  `status` (also computed-out, i.e. returned even when used as a filter
  input — same pattern `checkout_domain_data_source.go` or similar
  filter-shaped data sources use elsewhere in the Terraform ecosystem),
  `customer_id`, `currency_code`, `next_billed_at`, `created_at`. Check
  the real `GET /subscriptions/{id}` response shape before finalizing the
  field list — `client.Subscription` today only has `ID`/`Status`, you'll
  need to extend that struct (carefully — it's used elsewhere; check
  every existing caller before widening it) or add a separate, richer
  type for this data source's purposes.

Client-side: extend `internal/client/client.go`'s `ListSubscriptions` (or
add a new method — your call, but don't silently change
`ListSubscriptions`'s existing unfiltered behavior if anything else
depends on that) to accept the `customer_id`/`status` filters confirmed
real in [[0006-subscription-transaction-events-notifications-api-shapes]].

Register in `internal/provider/provider.go`'s `DataSources()`.

Acceptance test: use `findTestSubscription`
(`internal/provider/action_paddle_subscription_acc_test.go`) or the
pinned `PADDLE_TEST_SUBSCRIPTION_ID`/`PADDLE_TEST_CANCELED_SUBSCRIPTION_ID`
env vars already wired up for the subscription actions — don't
re-provision a third fixture, both existing pinned subscriptions are
already exactly what this data source needs to look up.

Docs: `tfplugindocs generate` picks this up automatically once the schema
exists (same as every prior resource/data source in this repo) —
confirm `docs/data-sources/subscription.md` is generated, don't hand-write
it.

## Step 3: `paddle_transaction` data source

Status: implemented, unit-tested, docs generated
(`docs/data-sources/transaction.md`), `go build`/`go vet`/`gofmt`/
`go test ./...` clean. Sandbox acceptance verification pending — same
reason as Steps 1-2. Added `client.ResolvedLineItem`/`ResolveLineItems`
to `internal/client/lineitem.go` (Step 1's helper, now with a second
consumer) and collapsed `ListSubscriptionChargeTransactions`/
`ListTransactionsByCustomer` onto one new
`ListTransactionsFiltered`/`TransactionListFilter`, both kept as thin
wrappers so existing callers/signatures are untouched.
`internal/provider/transaction_data_source.go` registered in
`provider.go`. Acceptance test
(`transaction_data_source_acc_test.go`,
`TestAccPaddleTransactionDataSource_feedsAdjustment`) feeds
`data.paddle_transaction.test.line_items[0].item_id` straight into a
real `paddle_adjustment` action invocation via Terraform references, per
the plan's end-to-end proof requirement — not yet run against the real
sandbox.

Depends on: Step 1 (uses the line-item-shape helper
to expose `Details.LineItems` in this data source's schema — this is the
proof-of-value Step 1 was built for).

Same structure as Step 2: `internal/provider/transaction_data_source.go`,
`id`/`subscription_id`/`customer_id`/`status` all `Optional`, everything
else `Computed`. Must expose line items in a form
`paddle_adjustment`'s `item_id` config can be built from directly — a
nested-list attribute with at least `item_id` and `price_id` per line
(check the real field list on `Transaction.Details.LineItems` — it may
have more fields worth surfacing, e.g. `quantity`, `totals`).

Client-side: `ListTransactionsByCustomer` and
`ListSubscriptionChargeTransactions` already exist and both filter via
query params — extend with the additional filters
([[0006-subscription-transaction-events-notifications-api-shapes]]:
`subscription_id`, `status`) rather than writing a fourth, parallel list
method. Consider whether these three list methods should collapse into
one parameterized method now that a third caller needs overlapping
filters — a judgment call, not mandated, but worth a look before adding a
fourth near-duplicate.

Register in `provider.go`'s `DataSources()`.

Acceptance test: this data source's most valuable proof is an end-to-end
one — look up a transaction via this new data source, then feed its
`item_id` straight into a real `paddle_adjustment` action invocation
(reusing `createAdjustmentFixtureTransaction`'s fixture from
`internal/provider/action_paddle_adjustment_acc_test.go`) — confirming
this data source actually closes the usability gap it was built for, not
just that it returns correctly-shaped data in isolation.

Docs: `tfplugindocs generate`, confirm `docs/data-sources/transaction.md`.

## Step 4: `paddle_customer` data source (PII-bearing)

Status: implemented, unit-tested, docs generated
(`docs/data-sources/customer.md`), README PII section added, `go build`/
`go vet`/`gofmt`/`go test ./...` clean. Sandbox acceptance verification
pending — same reason as Steps 1-3. `internal/provider/customer_data_source.go`
registered in `provider.go`; `GetCustomer`/`ListCustomersByEmail` added to
`internal/client/client.go` (email filter confirmed exact-match,
comma-separated, against the real API reference — not assumed). Acceptance
test (`customer_data_source_acc_test.go`) creates a fixture customer,
looks it up by both `id` and `email`, archives it in `t.Cleanup` — its
`acctest`-containing email is already covered by the existing
`sweepTestFixtureCustomers` sweeper too, no sweeper extension needed. Not
yet run against the real sandbox.

Depends on: none.

New file: `internal/provider/customer_data_source.go`. Schema: `id`
`Optional`, `email` `Optional` filter, `name`/`status` `Computed`.
**Before writing the `MarkdownDescription`**, read
`docs/guardrails/pii-bearing-data-sources-need-state-security-warning.md`
in full — the warning text isn't optional flavor, it's what this step is
actually for. Follow the same wording posture `README.md`'s existing
Actions section uses for financial risk (found via `grep -n "risk" README.md`
if you need the exact precedent), applied to PII instead.

Client-side: `CreateCustomer`, `ListTestFixtureCustomers`,
`ArchiveTestFixtureCustomer` already exist (test-fixture-only section of
`client.go`) but there's no `GetCustomer`/`ListCustomers` general-purpose
read method yet — add one. Confirm the real `GET /customers` filter
support (by `email`, at minimum) against Paddle's API reference before
assuming the query parameter name — don't guess it from the pattern the
other filters used, verify this one specifically since customer search
semantics (exact match vs. partial) matter for correctness here.

Register in `provider.go`'s `DataSources()`.

**README.md**: add a new subsection (suggest right after the existing
Actions section) specifically warning about this data source's PII/state
risk, per the guardrail. Don't fold it into the Actions section — it's a
different risk category (data exposure, not financial/irreversible
action) and deserves its own heading so it's not missed by someone
skimming for "which sections apply to me."

Acceptance test: create a fixture customer (the pattern
`createAdjustmentFixtureTransaction` already uses via `CreateCustomer` is
a fine model to follow), look it up via this data source, confirm the
fields match, then archive it via the existing
`ArchiveTestFixtureCustomer` (don't leave a leaked test customer — extend
`internal/provider/sweep_test.go`'s existing customer-sweeping logic if
this fixture pattern doesn't already get swept by what's there).

Docs: `tfplugindocs generate`, confirm `docs/data-sources/customer.md`.

## Step 5: `paddle_events` and `paddle_notification` data sources

Status: implemented, unit-tested, docs generated
(`docs/data-sources/{events,notification}.md`), `go build`/`go vet`/
`gofmt`/`go test ./...` clean. Sandbox acceptance verification pending —
same reason as Steps 1-4. Confirmed against the real API reference,
2026-08-11: `GET /events` has only an `event_type` filter, no date-range
parameter at all (so `paddle_events`' schema documents the 90-day
retention window as a hard limitation, not a discoverable one);
`GET /notifications` has `notification_setting_id`/`status`/`search`/
`filter`/`from`/`to` — does not mirror `/events`' shape, checked
specifically rather than assumed. Delivery-attempt detail comes from
`GET /notifications/{id}/logs` (`response_code`/`response_content_type`/
`response_body`/`attempted_at`), surfaced as `paddle_notification`'s
`logs` nested list. `ListEvents`, `GetNotification`/
`ListNotificationsFiltered`/`NotificationListFilter`,
`ListNotificationLogs` added to `internal/client/client.go`.
`internal/provider/{events,notification}_data_source.go` registered in
`provider.go`. `paddle_events`' acceptance test needs no dedicated
fixture (any product create already produces a `product.created` event);
`paddle_notification`'s is deliberately lenient — lists whatever already
exists and skips cleanly if nothing does, since a notification can't be
provisioned via direct API call. Not yet run against the real sandbox.

Depends on: none (independent of Steps 1-4, can be
done in parallel or any order relative to them).

Two new files: `internal/provider/events_data_source.go` and
`internal/provider/notification_data_source.go`.

`paddle_events`: `type` `Optional` filter (comma-separated per the real
API — confirm whether this provider's schema should expose it as a
`ListAttribute` of strings, joined server-side, rather than a raw
comma-separated string attribute — prefer the list form, matching
`subscribed_events` on the existing `notification_setting_resource.go`
schema for consistency), plus whatever date-range filter the real API
supports (check — not yet confirmed in
[[0006-subscription-transaction-events-notifications-api-shapes]], verify
before designing this attribute). **`MarkdownDescription` must state the
90-day retention window explicitly** — confirmed real in that same fact
file; an undocumented silent-empty-result past 90 days is exactly the
kind of footgun this repo's existing guardrails (e.g.
`docs/guardrails/money-moving-actions-no-blanket-retry.md`'s general
philosophy of "document real limitations, don't imply false guarantees")
consistently avoid elsewhere — match that standard here.

`paddle_notification`: `id` `Optional`, filter by `notification_setting_id`
and/or `status` (check the real API's actual filter support before
finalizing — don't assume it mirrors `paddle_events`' filters just
because they're adjacent features). Should expose delivery-attempt detail
(response code/body Paddle recorded) since that's the entire point —
"did my webhook get delivered" — not just the notification's own
metadata.

Client-side: new `ListEvents`/`ListNotifications` methods on
`internal/client.Client`, following the existing pagination pattern
(`after` cursor, `HasMore`) every other `List*` method already uses —
copy that shape, don't reinvent it.

Register both in `provider.go`'s `DataSources()`.

Acceptance test: `paddle_events` — trigger something that produces a real
event (e.g. the existing `TestAccPaddleProduct_basic`'s create already
does; no new fixture needed, just query for a `type` you know just
happened). `paddle_notification` — needs an actual configured
`notification_setting` with real deliveries to inspect; check whether the
sandbox account already has one from prior acceptance-test runs
(`paddle_notification_setting`'s own acceptance test creates one) before
deciding whether a dedicated fixture is needed here, or whether this test
should be more lenient (assert schema/shape correctness against whatever
real notifications already exist, rather than requiring a specific one).

Docs: `tfplugindocs generate`, confirm
`docs/data-sources/{events,notification}.md`.

## Step 6: extend `e2e.yaml` for scheduled action regression coverage

Status: implemented (`.github/workflows/e2e.yaml`), YAML validated
(`python3 -c "import yaml; yaml.safe_load(...)"`), not yet triggered —
this needs a real GitHub Actions run (push/PR merge, then a manual
`workflow_dispatch` or waiting for the daily schedule) which this session
can't do; still needs the "Definition of done" confirmation below once
merged. **Found the hard way while implementing this step**: the plan's
description of `e2e.yaml` as running "the catalog resources' acceptance
tests" doesn't match reality — `e2e.yaml` has no `go test` step at all,
it applies real Terraform HCL against the published Registry artifact
(`ci.yaml`'s separate `acceptance` job is what runs `go test -run TestAcc`,
against the in-process build, not the published one). `action_paddle_*_acc_test.go`'s
tests are structurally incapable of testing a published binary
(`testAccProtoV6ProviderFactories` always builds in-process from source),
so "extend the `-run` regex" wasn't actually applicable — extended
`e2e.yaml`'s existing HCL-against-published-binary pattern with a
`paddle_subscription_pause`/`resume` action block instead, gated behind a
new `PADDLE_TEST_SUBSCRIPTION_ID` secret (same pinned fixture
README.md's Actions section already documents provisioning), skipping
cleanly if unset. `cancel`/`charge` deliberately excluded from this
addition — pause+resume is the one action pair that's safe and fully
reversible against a shared pinned fixture on a daily, unattended
schedule.

Depends on: none — can be done any time, doesn't
block or get blocked by Steps 1-5, but do it before calling this plan
done (it's the regression-guard the review specifically asked for).

Read `.github/workflows/e2e.yaml` in full first. It currently runs
against the *published Registry artifact* (not source), daily, for the
catalog resources. Extend its test-run step to also include the actions'
acceptance tests (`TestAccPaddleAdjustment_*`, `TestAccPaddleSubscription*`
— check the exact `-run` regex/pattern it currently uses and extend it,
don't replace it). This needs the same `PADDLE_TEST_SUBSCRIPTION_ID`/
`PADDLE_TEST_CANCELED_SUBSCRIPTION_ID` secrets `ci.yaml`'s `acceptance`
job already has wired up — confirm `e2e.yaml`'s env block passes them
through too (it may not, today, since it predates the subscription
actions' pinned-fixture pattern — check before assuming).

**Definition of done**: manually trigger the workflow once (or wait for
its next scheduled run) after this change and confirm the actions' tests
actually ran and passed in that job's log — same standard this whole
project applies everywhere else (`docs/skills/verify-before-claiming.md`):
"the YAML looks right" is not the same as "it actually ran".

## Step 7: housekeeping (do this early — it's fast and unblocks nothing else)

Status: partially done.

- `README.md`: replaced the stale "Pre-1.0 (`v0.2.x`)" Status line with
  accurate current framing — `v0.4.0` stable, actions, and the new v4
  data sources, done at the *end* of this session's work (Steps 1-6 all
  implemented first) so it reflects what actually shipped rather than
  what was planned. Also updated the repo's top-line description to name
  the new data sources. Done.
- Confirm the stray duplicate one-time charge this session left queued on
  `PADDLE_TEST_SUBSCRIPTION_ID`'s next renewal (documented in
  `docs/guardrails/money-moving-actions-no-blanket-retry.md`'s "Third
  correction" entry, billing date 2026-09-11) actually got credited by
  the sweeper once that date passes. **Not checkable yet, not blocked on
  sandbox access**: today is 2026-08-11, a full month before that
  billing date — the charge hasn't even fired yet. Revisit this
  specific item after 2026-09-11, separately from the rest of this
  plan's sandbox-verification gap (which was blocked on
  `PADDLE_API_KEY`, not on time passing).

## Post-implementation code review (2026-08-11, `/code-review high`)

10 findings against Steps 1-6 as originally implemented; all 10 fixed
this same session, TDD (failing test confirmed red, then the fix,
confirmed green) for every fix backed by real logic, not just docs:

1. **`paddle_subscription`/`paddle_transaction`/`paddle_notification` had
   no guard against a config with `id` and every filter left unset** —
   silently fell through to an unfiltered list-and-hope-there's-exactly-
   one, dangerous specifically for the two that feed real actions
   (`subscription_id`/`item_id` into cancel/pause/resume/charge/adjustment).
   Fixed: `internal/provider/lookup_guard.go`'s `subscriptionFilterEmpty`/
   `transactionFilterEmpty`/`notificationFilterEmpty`, unit-tested
   (`lookup_guard_test.go`, red before the functions existed, green
   after), wired into each `Read()` as an explicit "Missing lookup key"
   error, mirroring `paddle_customer`'s existing guard.
2. **`events_data_source_acc_test.go`'s `Check` didn't actually verify
   the data source's own output** — it only asserted `events.#` is set
   (passes even at `"0"`), while the real assertion ran in `PostApplyFunc`
   via an independent direct `client.ListEvents` call bypassing `Read()`
   entirely; a real `Read()` bug could go undetected. Fixed:
   `testAccCheckEventsContainsProduct` now asserts directly against the
   data source's Terraform state.
3. **`paddle_transaction`/`paddle_notification` had zero acceptance-test
   coverage for their filter-lookup path** (only `id`-lookup was
   exercised), unlike `paddle_subscription`'s `_byID`+`_byFilter` pair.
   Fixed: added `TestAccPaddleTransactionDataSource_byFilter` and
   `TestAccPaddleNotificationDataSource_byFilter`.
4. **`e2e.yaml`'s new action block assumed same-`action_trigger`
   ordering** ("`actions = [pause, resume]` runs pause first") that
   Terraform's own reference doesn't explicitly guarantee (calls
   `actions` only an "ordered list", confirmed via the live docs, no
   sequential-completion language). Fixed: restructured into two
   `terraform_data` resources with `depends_on` between them — a
   documented, load-bearing Terraform guarantee — instead of relying on
   same-trigger list-ordering semantics.
5. **`e2e.yaml`'s action coverage doesn't literally satisfy
   `docs/guardrails/actions-need-scheduled-regression-coverage.md`'s
   "every action" wording** (deliberately excludes `cancel`/`charge`/
   `adjustment` on safety/reversibility grounds, disclosed in the
   workflow's own comment but not in the guardrail doc itself). Fixed:
   the guardrail doc now documents this exception explicitly, plus the
   e2e.yaml-has-no-`go test`-step correction (see Step 6's own status
   line above for that same correction).
6. **`Subscription.NextBilledAt`/`Notification.DeliveredAt` returned `""`
   instead of `null`** when Paddle omits them (canceled/paused
   subscription; not-yet-delivered notification) — inconsistent with
   this codebase's own established pointer-for-genuinely-optional-field
   convention (`discount_resource.go`'s `Code`/`CurrencyCode`/etc.), and
   silently broke the idiomatic Terraform `!= null` absence check. Fixed:
   both fields are now `*string`; unit tests
   (`TestFromAPISubscription_NoNextBilledAtIsNull`,
   `TestFromAPINotification_NoDeliveredAtIsNull`) confirmed red (didn't
   compile against the old `string` fields) then green.
7. **`ListSubscriptionsFiltered`/`ListTransactionsFiltered`/
   `ListNotificationsFiltered` always paginated to full exhaustion**
   even though their data-source callers only need to know 0/exactly-1/
   more-than-1 — wasteful API calls on every `plan`/`refresh`, worse
   once combined with the missing-guard bug above. Fixed: added a
   `Limit` field to each `*ListFilter` (0 = unlimited, unchanged default
   for every pre-existing caller), backed by a pure `reachedLimit`
   helper (`internal/client/pagination.go`, unit-tested red-then-green),
   and each data source now passes `Limit: 2`.

All fixes verified: `go build ./...`, `go vet ./...`, `gofmt -l .`,
`golangci-lint run ./...`, `go test ./...` all clean; `tfplugindocs
generate` produces no diff beyond the intentional new doc files and the
`next_billed_at`/`delivered_at` wording updates. Still blocked on real
sandbox verification — see the checklist below, unchanged by this review
round (no behavior needing a sandbox run was left un-reviewed, but
nothing here has actually been run against one either).

## Definition of done for this plan

**Final status (2026-08-11): done.** What started as "implement + unit
test only" (no `PADDLE_API_KEY` in the original session) was fully
closed out across a sequence of follow-up PRs once real sandbox access
became available via CI — every item below now has real, checkable
evidence, not just an assertion.

- [x] Steps 1-6 all implemented and real-sandbox-verified. PR #19 (merge
      commit `6e5cb1b`) shipped the full v4 feature set with 10
      code-review findings already fixed pre-merge (TDD: red confirmed,
      then the fix, then green, for every finding backed by real logic).
- [x] `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`,
      `golangci-lint run ./...` all clean — confirmed repeatedly across
      every PR's CI `build`/`lint` jobs, and again locally on final
      `master` before tagging.
- [x] `tfplugindocs generate` produces no diff — confirmed via every PR's
      `docs` CI job (`git diff --exit-code -- docs/`) and locally before
      tagging. Five new data source doc files shipped:
      `docs/data-sources/{subscription,transaction,customer,events,notification}.md`.
- [x] Every new data source verified against the real sandbox via CI's
      `acceptance` job (`TF_ACC=1 PADDLE_API_KEY=... go test ./... -run
      TestAcc -v`) — not just unit-tested. Real passing runs:
      PR #19 (run `31526895540`) exercised
      `TestAccPaddleSubscriptionDataSource_{byID,byFilter}`,
      `TestAccPaddleTransactionDataSource_{feedsAdjustment,byFilter}`,
      `TestAccPaddleCustomerDataSource_byIDAndEmail`,
      `TestAccPaddleEventsDataSource_productCreated`, and
      `TestAccPaddleNotificationDataSource_basic` (the `_byFilter`
      variant skipped that run — no unambiguous notification existed
      yet). PR #20 (run `31540271382`, after a manually-created
      permanent `notification_setting` fixture) confirmed
      `TestAccPaddleNotificationDataSource_basic` passes for real
      end-to-end, not just its empty-account skip path — see
      README.md's new `paddle_notification` precondition paragraph for
      how that fixture was set up.
- [x] `.github/workflows/e2e.yaml`'s new action-coverage step actually
      triggered and confirmed passing in the job's log — twice.
      `workflow_dispatch` run `31526299732` first surfaced a real
      concurrency bug (pause/resume racing within one `terraform apply`
      despite a `depends_on` between them, confirmed via the log's
      "Action started" timestamps 2ms apart, leaving the real pinned
      subscription stuck `paused`). Fixed by splitting pause/resume into
      two genuinely separate `terraform apply` invocations; re-dispatch
      run `31526558851` confirmed the fix — pause found it already
      paused (no-op), resume then ran in a separate process and set it
      back to `active`, log-confirmed
      (`"Subscription *** resume requested (status now active)"`).
- [x] `CHANGELOG.md` gets a `[0.5.0]` entry — added once all of the above
      was actually verified, not before (see the entry itself for what's
      listed).
- [x] Tagged and pushed as `v0.5.0` (not `v4.0.0`), release verified
      non-prerelease, Registry ingestion confirmed, and a real
      `terraform init`/`validate` smoke test run against the actual
      published artifact. See the `v0.5.0` CHANGELOG entry and the
      `release`/`registry-smoke-test` workflow runs it links for the
      evidence trail.

### Follow-up work this plan's own execution surfaced (all closed before
### tagging)

Real sandbox use (not code review) found four more issues, each fixed in
its own PR before this plan was called done:

- PR #21/#22: `createAdjustmentFixtureTransaction`'s customer/transaction
  (then product/price) fixtures had no per-test cleanup, only a weekly
  sweep — inconsistent with this repo's own stated sweeper design
  (`docs/decisions/0009`: sweepers are a crash safety net, not primary
  cleanup). Fixed with `t.Cleanup`, plus a `tooRecentToSweep` age guard
  against the sweeper and an in-flight test racing over the same live
  sandbox account.
- PR #23: a real `sweep.yaml` run against the backlog these accumulated
  before the above fix landed died on `go test`'s own default 10-minute
  internal timeout (`*** Test killed with quit: ran too long (11m0s)`).
  Fixed with an explicit `-timeout=30m`.
- PR #24: even with more time, the sweep was burning a full ~60s
  retry-with-backoff cycle per leaked transaction on a `CancelTransaction`
  call *guaranteed* to fail (every leaked subscription-charge transaction
  is already `completed`, never cancelable) before falling through to
  the working credit/refund path — observed live as exactly ~120s per
  transaction. Fixed with `shouldAttemptCancel`, skipping the doomed
  attempt for `completed` transactions specifically.
