---
title: Implementation plan — terraform-provider-paddle v2
status: done — v0.2.0 released and confirmed live on the Terraform
  Registry 2026-08-09 (PR #2 merged to master, both v0.1.0 and v0.2.0
  present in registry.terraform.io/v1/providers/vivantel/paddle/versions,
  all 5 resource/data-source doc pages present on the v0.2.0 record)
date: 2026-08-08
tags: [paddle, provider, plan, v2]
---

# Implementation plan: terraform-provider-paddle v2

**Read this whole file before doing anything.** Written to be
self-sufficient for a completely fresh session with zero prior context —
same convention as `docs/plans/paddle-provider-v1.md`, which this plan
follows on from.

Repo: `/home/ubuntu/projects/vivantel/terraform-provider-paddle`
(GitHub: `vivantel/terraform-provider-paddle`).

## Why this plan exists

v1 (`docs/plans/paddle-provider-v1.md`, Steps 0-7 done, Step 8 — first
release — not yet done as of this plan's writing) shipped
`paddle_product`/`paddle_price`/`paddle_discount` with data sources, full
sandbox verification, and CI enforcement. A follow-up conversation
(2026-08-08, same day) asked "what's next" and turned the answer into
three recorded decisions:

- `docs/decisions/0007-v2-scope-discount-groups-and-notification-settings.md`
  — 2 new resources (+ a stretch 3rd), field lists already verified
  against the real Paddle API reference.
- `docs/decisions/0008-custom-data-and-enum-validator-retrofit.md` — fix
  two known inconsistencies in the three v1 resources.
- `docs/decisions/0009-tflog-observability-and-acceptance-test-sweepers.md`
  — provider engineering gaps (no debug logging, no sandbox cleanup for
  interrupted test runs) unrelated to new resource coverage.

Read those three before making any judgment call not spelled out below —
same relationship this file has to those decisions that v1's plan had to
its own.

## Ground truth before you start

- Go 1.25, `terraform-plugin-framework` v1.19.0 (or later — check
  `go.mod`, don't assume this stays pinned).
- `master` is the default branch; v1 was merged via
  `github.com/vivantel/terraform-provider-paddle/pull/1`. Branch from
  `master`, not from the old `feature/v1` (merged, may or may not still
  exist).
- Existing resources: `paddle_product`, `paddle_price`, `paddle_discount`,
  each with a matching data source, all in `internal/provider/`. Client
  methods for all three in `internal/client/client.go`, including retry/
  backoff, an overall call budget, and `client.IsNotFound`.
- v1's Step 8 (tag `v0.1.0`, confirm Terraform Registry publish) may or
  may not be done by the time this plan starts — check
  `docs/plans/paddle-provider-v1.md`'s Step 8 status before assuming
  either way. This plan doesn't depend on it either way; v2 work can
  happen on an unpublished or published provider equally.
- Every established pattern from v1 still applies and isn't re-explained
  here: `IsNull()`+`IsUnknown()` checks on every Optional+Computed field
  before calling `Value*()`, `Default` (not just `UseStateForUnknown`) on
  any Optional+Computed field the API doesn't accept an explicit value
  omission for, `GetAttribute(id)`-only fetches in `Read()` for any model
  with a nested struct field, a dedicated Update-body type when an API
  rejects a field on update that it accepts on create, unit tests for
  every pure `toAPI*`/`fromAPI*` function, acceptance tests confirmed
  against the real sandbox before considering a resource done. See
  `docs/plans/paddle-provider-v1.md`'s Step 2/3/5 status blocks for the
  specific bugs each of these prevented — this list exists so v2 doesn't
  rediscover any of them the expensive way.

## How to update this file as you work

Same convention as v1: each step has a `Status:` line, update in place as
you go. Commit via `docs/skills/commit-with-kms-attribute.md`. Regenerate
docs via `tfplugindocs generate` after any schema change
(`docs/guardrails/docs-must-be-regenerated-before-merge.md`) and confirm
`git diff --exit-code -- docs/` locally before pushing, the same as every
v1 commit did.

---

## Step 0: `custom_data` retrofit

Status: done, confirmed green against the real sandbox — 2026-08-08
(PR #2, run 31283730453; all 9 acceptance tests pass, including the
`PlanOnly` no-op step inside each `*_customData` test — the
semantic-JSON plan modifier worked against real Paddle-round-tripped data
on the first try, no bugs found here unlike almost every other resource
change this project has made). Modeled as a JSON-encoded `types.String`
(confirmed against
the API reference that `custom_data` is arbitrary nested JSON, not a flat
string map — `types.Map` wouldn't fit) with a shared
`customDataPlanModifier` doing semantic-equality comparison, in new
`internal/provider/custom_data.go`, used by all three resources.
`toAPIProduct`/`fromAPIProduct`/`toAPIPrice`/`fromAPIPrice` now return an
error (malformed `custom_data` JSON is a real failure mode); `toAPIDiscount`/
`fromAPIDiscount` already returned `diag.Diagnostics`, slotted in there.
Unit tests centralized in `custom_data_test.go`. Acceptance tests added
per resource (`TestAccPaddle{Product,Price,Discount}_customData`),
including a `PlanOnly` step with re-ordered JSON keys to confirm the plan
modifier actually works against real Paddle-round-tripped data, not just
fabricated unit test values. Docs regenerated, no drift.

Implements: [[0008-custom-data-and-enum-validator-retrofit]],
`docs/guardrails/expose-custom-data-on-catalog-resources.md`.

1. Add a `custom_data` attribute to `paddle_product`, `paddle_price`, and
   `paddle_discount`'s schemas — `types.Map` of `types.String`, or
   research whether `terraform-plugin-framework`'s dynamic/JSON-shaped
   types fit Paddle's arbitrary-key-value-JSON `custom_data` field better
   before committing to a Go type; `client.Product.CustomData`/
   `client.Price.CustomData`/`client.Discount.CustomData` are all
   `map[string]any` today, which may need to change depending on what the
   schema type ends up being.
2. Wire through `toAPI*`/`fromAPI*` for all three resources, following the
   same null-vs-unknown pattern as every other Optional field.
3. Unit tests for the clear/set round-trip on all three (per
   `docs/guardrails/pure-logic-needs-unit-tests.md`).
4. Acceptance test coverage: extend each resource's existing
   `TestAccPaddle*_basic` with a `custom_data` check, or add a dedicated
   step — confirm against the real sandbox before calling this step done.
5. Regenerate docs.

## Step 1: Enum validator retrofit

Status: done — 2026-08-08. `stringvalidator.OneOf` added to
`paddle_product`'s `tax_category` and `type`, and `paddle_price`'s
`tax_mode` (values reused from each attribute's existing
`MarkdownDescription`, not re-derived). No client changes, no sandbox
verification needed — pure schema-level validation.

**Found along the way, not fixed here — new item, not silently folded
in:** `paddle_price` doesn't actually expose a `type` attribute at all.
[[0008-custom-data-and-enum-validator-retrofit]] assumed it existed and
just needed a validator; `client.Price.Type` has the same "exists in the
client struct, not in the resource schema" gap `custom_data` had before
Step 0. Adding the attribute itself (with a correct `Default`/
`UseStateForUnknown` — check what Paddle's `type` field on prices actually
means and whether it needs the same care `quantity`/`code` needed) is
bigger scope than "add a validator" and is its own future retrofit, not
part of this step.

Implements: [[0008-custom-data-and-enum-validator-retrofit]].

## Step 2: `tflog` debug logging

Status: done, confirmed green against the real sandbox — 2026-08-08.
`tflog.Debug` added at the three points in `internal/client/client.go`'s
`do()` described below; `do()` is the single chokepoint every resource/
data-source CRUD method already funnels through, so no resource-level
`tflog` calls were needed on top — "start with just `do()`, expand only if
a real debugging session shows a gap" concluded there was no gap.
Verified by temporarily setting `TF_LOG: debug` on the CI acceptance job
(push+PR run, `31283961659`): all 9 acceptance tests passed, and the log
output contained 72 `"paddle: sending request"` / 72 `"paddle: received
response"` line pairs (method/path/attempt/status only, one pair per
resource-and-data-source-mediated real HTTP call), with the sandbox API
key and every request/response body absent from the log throughout
(checked explicitly via `grep -i authorization`/`grep -i bearer` — the
only "authorization" hits were GitHub Actions' own already-masked
checkout credentials, unrelated to this provider). The `TF_LOG: debug`
CI change was then reverted (it's not meant to run at debug verbosity on
every push) — that revert is `acd6656`.

1. Add `tflog.Debug` calls in `internal/client/client.go`'s `do()`:
   method, path, attempt number, response status. Never log
   `c.APIKey` or full request/response bodies (see the guardrail for why).
2. Consider whether resource-level `Create`/`Read`/`Update`/`Delete`
   methods need their own `tflog` calls beyond what `do()` already covers,
   or whether `do()`'s logging is sufficient — start with just `do()`,
   expand only if a real debugging session shows a gap.
3. No unit test can meaningfully assert "does this log," but do manually
   verify with `TF_LOG=debug` against the sandbox before calling this
   step done.

## Step 3: Acceptance test sweepers

Status: done, confirmed green against the real sandbox — 2026-08-08.
Existing acceptance test configs across `paddle_product`/`paddle_price`/
`paddle_discount` already contained "Acc Test" somewhere in their name/
description fields (checked before assuming a rename was needed — it
wasn't), so sweepers match that same substring case-insensitively
(`isAccTestName`) rather than introducing a second convention.
`internal/client/client.go` gained `ListProducts`/`ListPrices`/
`ListDiscounts` (cursor-based `after` pagination, unit-tested against a
`httptest` server for the has-more-cursor, stops-on-`has_more=false`, and
empty-page-doesn't-infinite-loop cases) — sweepers are the first caller of
any list endpoint in this provider. `internal/provider/sweep_test.go`
registers `paddle_product`/`paddle_price`/`paddle_discount` sweepers via
`resource.AddTestSweepers` + `TestMain`, each archiving (not deleting —
same archive-not-delete pattern as the resources themselves) any
non-archived object matching the naming convention; `paddle_product`
declares `paddle_price` as a `Dependencies` entry so prices are swept
first. Verified against the real sandbox not by invoking the `-sweep` CLI
flag but by calling the sweep mechanics directly from a `TestAcc`-gated
test (`TestAccSweepProducts_ArchivesLeakedTestObjects`, CI run confirmed
below): creates a product outside Terraform entirely (the exact
leaked-object scenario sweepers exist for), runs the sweep, confirms
the object is archived afterward. **Found during the pre-merge review
pass, not during initial development:** the verification test originally
called the real sweeper function directly (the broad `isAccTestName`
match, same as a real `-sweep` invocation would use) — safe for the
Products case (archiving doesn't remove the object, so a concurrent CI
job's fixture just sees a status change), but the equivalent test added
for Step 5's notification-setting sweeper called the same broad match
against a real hard DELETE, and a concurrent `pull_request`-triggered CI
job's `TestAccPaddleNotificationSetting_basic` failed with "refresh plan
was not empty... + create" because this repo's `push` and `pull_request`
jobs run concurrently against one shared sandbox account, and one job's
broad sweep deleted the other job's still-in-progress fixture mid-test.
Fixed by extracting `sweepMatchingProducts`/`sweepMatchingNotificationSettings`
(list-then-archive/delete parameterized on a match predicate) so the real
sweepers keep the broad match (that's the whole point, for real `-sweep`
runs) while both verification tests scope themselves to only the exact ID
they created — see `sweepMatchingNotificationSettings`' comment in
`sweep_test.go` for the full account. `sweepPrices`/`sweepDiscounts`/
`sweepDiscountGroups` don't have their own verification tests (same
mechanically-identical archive shape as the already-covered Products
case, lower incremental risk), so weren't refactored to the same
parameterized shape — nothing currently calls them from inside a
`TestAcc`-gated test, so they can't hit this specific race.

Implements: [[0009-tflog-observability-and-acceptance-test-sweepers]].

1. Adopt a consistent naming prefix (e.g. `"acc-test-"`) across
   `testAccProductConfig`/`testAccPriceConfig`/`testAccDiscountConfig`'s
   name/description fields, if not already consistent — check current
   acceptance test files before assuming this needs changing.
2. Register `terraform-plugin-testing`-style sweepers for
   `paddle_product`/`paddle_price`/`paddle_discount` (and anything Steps
   5-6 add) that list and archive/delete any sandbox object matching the
   naming convention. See the
   `terraform-provider-development:provider-test-patterns` skill in this
   environment for the exact sweeper registration pattern.
3. Confirm a sweeper run actually cleans up a deliberately-orphaned test
   object against the real sandbox before calling this done.

## Step 4: `paddle_discount_group` resource + data source

Status: done, confirmed green against the real sandbox — 2026-08-09
(CI run 31297606621, both `TestAccPaddleDiscountGroup_basic` and
`TestAccPaddleDiscountGroupDataSource_basic` passed). First push's
`pull_request`-triggered job failed with a real finding, not a flake:
Paddle enforces global uniqueness on discount group `name` (409
`discount_group_name_conflict`) — the `push` and `pull_request` CI jobs
for the same commit ran those two tests concurrently against the same
sandbox account with the fixed names the tests originally used. Fixed by
adding `randAccTestSuffix()` (provider_test.go) and using it in both
discount-group acceptance tests — the first resource in this provider
needing this, since Product/Price/Discount don't enforce
name/description uniqueness.
`internal/client/client.go` gained `DiscountGroup` (just `Name`/`Status`,
confirmed against decision 0007's field list — no `custom_data`, Paddle's
API genuinely doesn't have it for this entity, checked rather than
assumed), `Create/Get/Update/ArchiveDiscountGroup`, and
`ListDiscountGroups` (for the sweeper). `discount_group_resource.go` +
`discount_group_data_source.go` modeled closely on `discount_resource.go`/
`discount_data_source.go` given the schema is close to the smallest
possible (name + status), including the same `GetAttribute(id)`-only
`Read()` pattern even though this model has no nested struct field to
protect against yet. Registered in `provider.go`, added to
`sweep_test.go` (declares `paddle_discount` as a sweeper `Dependencies`
entry, mirroring `paddle_product` → `paddle_price`). Unit tests for
`toAPIDiscountGroup`/`fromAPIDiscountGroup`, acceptance tests
(`TestAccPaddleDiscountGroup_basic`, `TestAccPaddleDiscountGroupDataSource_basic`)
covering create/update/no-op-plan/import/data-source lookup. Did not add
a plan-time reference-validity check on `paddle_discount`'s
`discount_group_id` (Step 4 item 6's "default to relying on Paddle's own
API error unless it's a two-line change" — it isn't a two-line change).
Docs regenerated (new `examples/{resources,data-sources}/paddle_discount_group/`
added, matching the existing per-resource example convention;
`provider.go`'s top-level `Description` updated to mention discount
groups).

Implements: [[0007-v2-scope-discount-groups-and-notification-settings]],
every guardrail in `docs/guardrails/` (data source, import, custom_data,
tflog logging, retry-wrapper client usage, pure-logic unit tests,
acceptance-test TF_ACC gate, docs regeneration) — this is the first new
resource built after all of them existed, apply every one from the start.

1. `internal/client/client.go`: `DiscountGroup` struct (`ID`, `Name`,
   `Status`, `CustomData` if Step 0 lands first — check), `CreateDiscountGroup`/
   `GetDiscountGroup`/`UpdateDiscountGroup`/`ArchiveDiscountGroup` (archive-
   via-update, same as Product/Price/Discount — confirmed no separate
   delete operation exists for this entity).
2. `internal/provider/discount_group_resource.go` +
   `discount_group_data_source.go`, modeled closely on
   `discount_resource.go`/`discount_data_source.go` given how similar the
   schemas are (this is by far the smallest of the three v1 resources'
   shape, `name` + `status` is nearly the whole schema).
3. `ImportState` via `resource.ImportStatePassthroughID`.
4. Unit tests, then acceptance tests, confirmed against the real sandbox.
5. Register in `provider.go`.
6. Consider (but don't require unless it's cheap): once this exists,
   should `paddle_discount`'s `discount_group_id` attribute gain a
   plan-time reference-validity check, or is relying on Paddle's own API
   error sufficient? Default to the latter unless it's a two-line change.

## Step 5: `paddle_notification_setting` resource + data source

Status: done, confirmed green against the real sandbox — 2026-08-09 (CI
run 31298051030: `TestAccPaddleNotificationSetting_basic`,
`TestAccPaddleNotificationSetting_activeFalseAtCreate`, and
`TestAccPaddleNotificationSettingDataSource_basic` all passed). First
push failed with a real finding, not a flake: `api_version` was modeled
Optional-only, but Paddle silently returns its own default (e.g. `1`)
even when the field is omitted from config — "Provider produced
inconsistent result after apply: .api_version: was null, but now
cty.NumberIntVal(1)" on the very first real Create, the exact same class
of bug `paddle_discount`'s `code` attribute already has a comment about
(and the exact reason that comment exists — this step didn't apply that
lesson to `api_version` the first time around). Fixed: `api_version` is
now Optional+Computed with `UseStateForUnknown`, and both
`toAPINotificationSettingCreate`/`Update` gained the matching
`IsUnknown()` check next to their existing `IsNull()` check (same
regression class as every other Optional+Computed field in this
provider). Confirmed against the real
API reference (not assumed) a shape asymmetry decision 0007 didn't call
out: the create/update request's `subscribed_events` is an array of
strings, but every response (create, update, get) returns it as an array
of event objects (`{name, description, group, available_versions}`) —
`client.NotificationSetting`/`NotificationSettingCreate`/
`NotificationSettingUpdate` are three separate Go types because of this,
not two. Also found `destination` is genuinely settable at update (not
create-only as might be assumed by analogy with `type`), and the response
carries `endpoint_secret_key` (webhook signing secret) — mapped to a
`Sensitive: true` Computed attribute. `active` isn't accepted at create at
all (Paddle always creates with `active: true`); `Create()` issues an
immediate follow-up `UpdateNotificationSetting` call when the plan wants
`active = false`, the only way to express that intent through this API —
covered by a dedicated acceptance test
(`TestAccPaddleNotificationSetting_activeFalseAtCreate`) rather than only
incidentally by the basic test's later Update step, since it's the one
piece of Create() logic no other resource in this provider has.
`Delete()` calls the real `DeleteNotificationSetting` (hard DELETE, not
archive-via-update); `CheckDestroy` asserts a 404 via `client.IsNotFound`
rather than an `archived` status. Client wire-shape unit tests confirm
`NotificationSettingCreate` never serializes `active` and
`NotificationSettingUpdate` never serializes `type`, plus a response
round-trip test for the object-array `subscribed_events` shape.
`subscribed_events` isn't validated against Paddle's full event-type enum
(40+ values) — documented in the schema description that Paddle's API is
the source of truth, per Step 5 item 2's guidance. Sweeper added
(`sweepNotificationSettings`, no "already archived" skip since this
entity has no status field, only existence). Docs regenerated
(`examples/{resources,data-sources}/paddle_notification_setting/` added,
`provider.go`'s `Description` updated).

Implements: [[0007-v2-scope-discount-groups-and-notification-settings]],
same guardrail set as Step 4.

1. `internal/client/client.go`: `NotificationSetting` struct (`ID`,
   `Description`, `Type` — **create-only**, `Destination`,
   `SubscribedEvents []string`, `APIVersion *int`,
   `IncludeSensitiveFields *bool`, `TrafficSource string`, `Active *bool`
   — update-only, not settable at create). `CreateNotificationSetting`/
   `GetNotificationSetting`/`UpdateNotificationSetting`/
   `DeleteNotificationSetting` — **real hard DELETE**, don't reuse the
   `statusPatch`/`Archive*` pattern, it doesn't apply to this entity.
2. `internal/provider/notification_setting_resource.go` +
   `notification_setting_data_source.go`. Schema notes:
   - `type` needs `stringplanmodifier.RequiresReplace()` — confirmed
     absent from the update field list in the real API reference, same
     class of fix as `paddle_price`'s `product_id`, but caught before
     writing the resource this time instead of after a sandbox crash.
   - `subscribed_events` as `types.List` of `types.String` — validate
     against Paddle's actual event-type enum if practical (40+ values
     spanning most entities), otherwise document that Paddle's API is the
     source of truth for valid values and let it reject unknown ones.
   - `active` should probably be `Computed`-only or `Optional+Computed`
     with a `Default` of `true` — it's not settable at create per the API
     reference, only at update, which is an unusual shape worth designing
     carefully rather than copy-pasting the `enabled_for_checkout` pattern
     from `paddle_discount` without checking whether it actually fits.
3. `Delete()` calls the real `DeleteNotificationSetting`, not an archive
   pattern — write this resource's `Delete()` from scratch rather than
   copy-pasting Product/Price/Discount's `Archive*`-based one.
   [[0007-v2-scope-discount-groups-and-notification-settings]] flags this
   explicitly: verify against the real sandbox whether a delete-then-
   delete-again 404s the same way archive-then-archive-again does, don't
   assume `client.IsNotFound` tolerance transfers over without checking.
4. `ImportState`, unit tests, acceptance tests — confirmed against the
   real sandbox before calling this done, especially the `type`
   `RequiresReplace` and the real-DELETE `Delete()`/`CheckDestroy` pairing,
   since both are new shapes this provider hasn't handled before.
5. Register in `provider.go`.

## Step 6 (stretch, do only if 4-5 land with time to spare): `paddle_checkout_domain`

Status: done (as a data source only), confirmed against the real
sandbox — 2026-08-09. Fetching the real field list (this step's own
prerequisite) surfaced something this plan didn't anticipate: Checkout
Domains has no create or update operation via the API at all — confirmed
against the live API reference, not assumed — "You can't add a checkout
domain using the API. To submit a new domain for approval, go to Paddle >
Checkout > Website approval > Domain approval in your dashboard." Only
List, Get, Delete, and a verify-payment-method action exist. Put to the
user directly (two options: data-source-only, or an import-only resource
whose `Create()` always errors) rather than guessed — data-source-only
was chosen: an import-only resource would need `domain` marked
`RequiresReplace` for an entity whose "replace" (destroy-then-create)
can never actually complete, since create always fails — a footgun for
the lowest-priority item in this whole plan, for marginal benefit over a
read-only lookup.

`client.CheckoutDomain`/`GetCheckoutDomain`/`ListCheckoutDomains` added
(`ListCheckoutDomains` isn't for a sweeper — nothing to sweep, this
provider never creates one — it's what
`TestAccPaddleCheckoutDomainDataSource_basic` uses in place of the
usual create-a-fixture pattern, since there's no API to create a fixture
with; the test lists whatever already exists in the sandbox and skips
cleanly if there isn't one). `checkout_domain_data_source.go`: nested
`payment_method_verification.apple_pay.status` modeled as nested
`SingleNestedAttribute`s (not flattened), matching the real response
shape. `Read()` fetches only `id` — this model has Required non-pointer
nested struct fields, the same class of fix `price_resource.go`'s
`Read()` needed. Unit test for `fromAPICheckoutDomain`'s nested-field
mapping; acceptance test confirmed via CI against the real sandbox
(non-empty case: the sandbox account already has an approved domain from
earlier setup, so the skip path wasn't exercised, only the real lookup
was). Docs regenerated (`examples/data-sources/paddle_checkout_domain/`
added, `provider.go`'s `Description` updated to mention domain lookup
without claiming a resource that doesn't exist).

Implements: [[0007-v2-scope-discount-groups-and-notification-settings]].

1. **First**: fetch and verify the real field list against
   `https://developer.paddle.com/api-reference/checkout-domains/overview`
   (or whatever the actual current path is — confirm it exists before
   planning further; it wasn't verified when this plan was written,
   unlike Discount Groups and Notification Settings). Don't design the
   schema from assumption.
2. Same guardrail set and process as Steps 4-5 once fields are confirmed.

## Step 7: Release polish

Status: partially done — 2026-08-09. Item 2 (`CONTRIBUTING.md`) done:
deliberately lean, points at `README.md`'s existing Development/Publishing
sections rather than duplicating them, covers the `docs/{decisions,facts,
guardrails,plans}/` knowledge-artifact convention and the non-obvious
per-resource patterns (`IsNull()`+`IsUnknown()`, Default vs
UseStateForUnknown, `GetAttribute`-only `Read()`, dedicated update-body
types) in one place instead of only living as scattered code comments.
`README.md` also updated: resource list, resource count, and archive-vs-
real-delete distinction now reflect v2, not just v1. Item 1
(`CHANGELOG.md`) deferred to the actual `v0.2.0` tagging step (this plan's
Definition of Done / the release task that follows it) rather than done
here — regenerating it now, before the review pass, would need
regenerating again if review produces any further commits. Item 3 (logo)
deliberately deferred — explicitly called out as this plan's lowest-
priority item, not worth spending remaining time on before wrapping up
review/merge/release.

Not tied to a specific decision record — housekeeping items mentioned in
the conversation that produced this plan, worth doing but not worth a
decision doc each.

1. `CHANGELOG.md` — should already exist from v1's Step 8
   (`docs/skills/release-with-kms-changelog.md`); if v2 ships its own
   tagged release, regenerate/extend it the same way.
2. `CONTRIBUTING.md` — doesn't exist yet. Cover: local dev setup (Go
   version, `dev_overrides`), running unit vs acceptance tests, the
   commit/PR conventions this repo actually uses
   (`docs/skills/commit-with-kms-attribute.md`), and a pointer to
   `docs/plans/`/`docs/decisions/` as the place design rationale lives
   rather than expecting it in commit messages alone.
3. Terraform Registry provider logo/SVG — cosmetic, affects the Registry
   listing appearance once published. Lowest priority item in this whole
   plan; do it last if at all.

---

## Follow-up not in this plan's scope

Raised 2026-08-09 during the pre-merge review pass: CI never actually
validates that what lands on the Terraform Registry installs and works.
`testAccProtoV6ProviderFactories` (`internal/provider/provider_test.go`)
builds the provider in-process from source — acceptance tests never go
through a real `terraform init` pulling `vivantel/paddle` from
`registry.terraform.io`, so a manifest problem, signature verification
failure, or missing platform binary in a release wouldn't be caught until
a real user hit it. v0.1.0 and v0.2.0 were both confirmed manually via
direct Registry API checks instead (see this plan's and
`paddle-provider-v1.md`'s Step 8 status blocks).

Deliberately deferred rather than added to this release's scope: a
post-release smoke test needs the version to already exist on the
Registry, so it's a separate workflow (manually triggered, or triggered
on `release.yaml`'s completion) rather than something pre-merge CI can
do — `terraform init`/`plan` against a scratch module pinned to
`vivantel/paddle` at the just-published version, using a real API key
from secrets. Worth a decision record (scope: sandbox vs. production key,
which resource(s) to exercise, trigger mechanism) before implementing,
not a same-night addition to an already-long release pipeline.

## Definition of done for this plan

- Steps 0-5 marked `done`. Step 6 done or explicitly deferred with a
  reason (not silently skipped). Step 7 done or explicitly deferred.
- `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...` all
  clean with no `TF_ACC` set.
- `TF_ACC=1 go test ./... -run TestAcc` passes against the sandbox for
  every resource, old and new.
- Sweepers confirmed working against a deliberately-orphaned test object.
- `tflog` output confirmed present via a manual `TF_LOG=debug` run.
- `tfplugindocs generate` produces no diff.
- Every commit carries `Refs:` trailers per
  `docs/skills/commit-with-kms-attribute.md`.
