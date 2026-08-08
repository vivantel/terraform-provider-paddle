---
title: Implementation plan — terraform-provider-paddle v2
status: not started
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

Status: done, pending real-sandbox confirmation (next CI push) —
2026-08-08. Modeled as a JSON-encoded `types.String` (confirmed against
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

Status: not started

Implements: [[0009-tflog-observability-and-acceptance-test-sweepers]],
`docs/guardrails/log-client-requests-with-tflog.md`.

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

Status: not started

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

Status: not started

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

Status: not started

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

Status: not started

Implements: [[0007-v2-scope-discount-groups-and-notification-settings]].

1. **First**: fetch and verify the real field list against
   `https://developer.paddle.com/api-reference/checkout-domains/overview`
   (or whatever the actual current path is — confirm it exists before
   planning further; it wasn't verified when this plan was written,
   unlike Discount Groups and Notification Settings). Don't design the
   schema from assumption.
2. Same guardrail set and process as Steps 4-5 once fields are confirmed.

## Step 7: Release polish

Status: not started

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
