# Changelog

## [0.6.1] - 2026-09-03

A patch release: no schema, attribute, or behavior changes for existing configurations. Provider docs (descriptions and examples) got a full rewrite for readability and completeness, a dependency CVE got patched, CI's broken lint job got fixed, and post-release verification left open after `v0.6.0` got closed out.

### Added

- Every resource, data source, and action's `MarkdownDescription` rewritten for a human reader — a one-sentence intro, real per-attribute descriptions (no more bare `(Type)` attributes), and internal `docs/guardrails/...`/`docs/decisions/...` links converted to prose since they 404 on the Registry. Added the missing `## Example Usage` section for all 6 actions and the 9 data sources that lacked one. A review pass before this shipped caught and fixed several real regressions the rewrite itself had introduced along the way — including 13 files left `gofmt`-invalid (would have failed CI outright), two safety-critical `**bold**` guardrail warnings silently stripped, and all 6 action example directories missing the `paddle_` prefix `tfplugindocs` actually requires, which had left every action doc silently rendering with no examples at all.
- A shared `describedTimeouts()` helper so all five resources' `timeouts` attribute documents its 60-second default and 30-minute hard ceiling from one place instead of five separate inline calls.

### Fixed

- `google.golang.org/grpc`, a transitive dependency, patched from `1.83.0` to `1.83.1` — fixes CVE-2026-84304 (GHSA-vp52-pcj8-j9qc), a high-severity denial-of-service via HTTP/2 DATA-frame fragmentation.
- CI's `lint` job had silently started failing on every push: `golangci-lint@latest` had drifted to a release requiring Go 1.26, while CI pins Go 1.25. Pinned the install to `v2.12.2`, the latest release still on the 1.25 floor.
- README's own `timeouts` configuration example used block syntax (`timeouts { ... }`) instead of the nested-attribute syntax (`timeouts = { ... }`) the schema actually expects — caught by validating the README's example against the real published `v0.6.0` binary, which rejected it outright. Fixed everywhere the wrong syntax appeared: `README.md`, `CHANGELOG.md`'s `[0.6.0]` entry, and the `configurable-timeouts-need-a-hard-ceiling` guardrail's own example.

### Documentation

- Closed out the `paddle-provider-v5` plan with real post-release verification evidence: `v0.6.0` confirmed tagged, released, Terraform Registry-ingested, and independently smoke-tested via a real `terraform init`/`validate` against the published binary.

## [0.6.0] - 2026-08-13

Closes a real PII-warning gap found the night `0.5.0` shipped (`paddle_events`' `data` field never got the same state-security warning `paddle_customer` carries), adds four plural/list data sources complementing `0.5.0`'s singular lookups, a `paddle_notification_replay` action, a configurable `timeouts` attribute replacing the client's previously-hardcoded 60s HTTP timeout, and a reusable mock-server testing harness retrofitted onto all five resources. Real-sandbox verification surfaced (and fixed) several genuine bugs along the way, including two real Paddle API validation nuances no prior release had documented and a sandbox-wide rate-limit exhaustion issue in this project's own CI retry tuning.

### Added

- `paddle_subscriptions`, `paddle_transactions`, `paddle_notifications`, `paddle_customers` — four new plural/list lookup data sources complementing the existing singular "exactly one match" data sources with a "give me everything matching these filters" shape. `paddle_transactions`/`paddle_notifications` deliberately exclude `line_items`/`logs` (both require a per-ID re-fetch on the singular data sources; doing that for every plural result would be a real N+1 cost). `paddle_customers` carries the PII-compounding warning this release's audit guardrail specifies for plural PII-bearing data sources. None of the four have a hard "no filter set" guard the way the singular lookups do — a cost warning in the schema description instead, since "list everything" is a legitimate use case here.
- `paddle_notification_replay` action — `POST /notifications/{id}/replay`, resending a `delivered`/`failed` notification. Unlike this provider's other five actions, it isn't financial or irreversible, so it deliberately skips the search-before-invoke/no-blind-retry treatment those get.
- A `timeouts` attribute on all five resources (`create`/`read`/`update`/`delete`), each a duration string. Defaults to 60 seconds (matching prior fixed behavior exactly) if omitted, with a hard 30-minute ceiling regardless of what's configured. The client's `do()`/`doNoRetry()` now only impose their own default when the caller's context has no deadline of its own — previously an unconditional `context.WithTimeout` meant a longer caller-configured timeout could never actually take effect, only ever fail faster.
- A reusable `httptest`-based mock-server test harness (`internal/provider/mockserver_test.go`) driving a resource through its real `Create`/`Read`/`Update`/`Delete` methods via `resource.Test`, one level deeper than the existing sweeper-test precedent. All five resources retrofitted with mock-based CRUD coverage — additive to, never a replacement for, real-sandbox acceptance tests.
- `examples/lookup-then-act/main.tf` — a real, complete config demonstrating the actual payoff of this provider's lookup data sources: look up a subscription/transaction, feed the result straight into an action, nothing hardcoded. Verified both via `terraform validate` against the real provider schema and a new acceptance test that applies the published example file itself against the real sandbox.
- A hand-written "Configuring `timeouts`" section in `README.md` — when you'd actually want to configure one, the no-behavior-change-if-omitted default, and the 30-minute ceiling.

### Fixed

- `paddle_events`' schema `MarkdownDescription` and `README.md` now carry the same state-security PII warning `paddle_customer` has, worded for an opaque/variable-shape field (`data` *can* carry PII depending on event type) rather than a copy of the dedicated-field wording.
- Adding `Timeouts` directly to the five resources' shared `*ResourceModel` structs broke every singular data source (each decodes state into that same struct type, whose schema has no `timeouts` attribute) — caught by CI's real-sandbox acceptance job, not by `go build` or any unit/mock test. Fixed by moving `Timeouts` into a resource-only wrapper struct per resource that embeds the base model.
- The example transactions acceptance test provisioned a fifth rate-limit-hungry fixture customer on top of every other acceptance test's own fixture in the same CI run, reproducibly hitting real sandbox `429`s. Rewritten to filter by the pinned subscription fixture's existing recurring billing history instead, needing zero new fixture provisioning.
- This project's acceptance-test suite was hitting sustained real sandbox rate-limit exhaustion across multiple CI runs — confirmed the client's retry logic already correctly honored a `429` response's `Retry-After` header, but a correctly-read `Retry-After` still got clamped to a 30-second cap, and the 60-second overall retry budget only left room for one or two such waits before giving up. Production defaults are unchanged; a new test-only hook widens both for the whole acceptance-test binary run.
- A new acceptance test misused an existing test helper (`findTestSubscription` with a `"canceled"` status argument), which always checks the pinned *active* subscription fixture regardless of the status requested — switched to the dedicated `findCanceledTestSubscription` helper that already existed for this exact case.
- `examples/lookup-then-act/main.tf`'s `paddle_adjustment` action used `action = "refund"` combined with `type = "full"` and an item-level `items` array — confirmed against the real sandbox that Paddle rejects this specific combination outright (`"items are not allowed when the adjustment type is full"`), while `action = "credit"` accepts the identical shape. Switched the example to `credit`, documented in-line.

### Changed

- The v5 PII/state-security guardrail's "Applies to" list updated to close out its `paddle_customers` forward-reference placeholder now that the data source actually exists, and to record a full audit pass over every existing resource/data source/action schema that found no other overlooked PII vector or missing `Sensitive: true` marking.
- `internal/client/client.go` gained `ListCustomersFiltered` (email+status filters, both confirmed real server-side support), `ReplayNotification`, and a new `PADDLE_BASE_URL` env var override — deliberately undocumented and absent from the public schema, existing solely so this release's mock-server harness can point the provider's client at a fake server instead of Paddle's real sandbox/production URLs.

## [0.5.0] - 2026-08-11

Closes the usability gap a product review found right after `0.4.0` shipped: v3's five actions are correctly implemented and real-sandbox-verified, but every one requires an opaque Paddle ID with no discovery path inside Terraform. Adds five read-only lookup data sources so a real user can find those IDs from something they actually have on hand instead of hardcoding values found via the dashboard or a manual API call. Getting a clean release out surfaced five more real bugs — three in CI workflow logic, two in the test sweeper — all found by actually running things against the live sandbox, not by code review, and all fixed below.

### Added

- `paddle_subscription`, `paddle_transaction`, `paddle_customer`, `paddle_events`, and `paddle_notification` — five new read-only lookup data sources. `paddle_transaction` in particular closes `paddle_adjustment`'s sharpest edge: its `item_id` lives three different, easy-to-conflate JSON shapes deep in Paddle's raw API, now resolved through one centralized helper (`internal/client/lineitem.go`) proven against both a pre-existing call site and the new data source. `paddle_customer` carries real PII (email, name) and ships with an explicit, loud state-security warning in both its schema and a new README section — Terraform persists data source reads into state on every refresh, not just once, so "read-only" doesn't make that concern go away.
- `.github/workflows/e2e.yaml` now also exercises `paddle_subscription_pause`/`resume` against the real published Registry binary, closing the gap where all three of `0.4.0-beta.1`'s real action bugs were only ever caught by a human manually running acceptance tests.

### Fixed

- `e2e.yaml`'s new action-coverage step used `secrets` in a step `if:` condition, which GitHub Actions rejects outright (a schema restriction invisible to a plain YAML parser) — the workflow file didn't even parse.
- `e2e.yaml`'s pause/resume actions raced concurrently despite a `depends_on` between their two resources, leaving the pinned sandbox subscription stuck `paused` — `depends_on` serializes resource apply order, not the actions attached to different resources' `action_trigger`s. Fixed by splitting pause and resume into two genuinely separate `terraform apply` invocations, which do serialize.
- `TestAccPaddleEventsDataSource_productCreated` flaked on Paddle's own event-log read-after-write lag — a product's `product.created` event wasn't always visible on the very next query. Added a retry-with-backoff wait, the same mitigation already used for `paddle_subscription_charge`'s duplicate-charge search.
- `sweep.yaml`'s `go test` invocation hit its own default 10-minute internal timeout partway through a real backlog, dying mid-sweep. Added an explicit `-timeout=30m`.
- The sweeper burned a full retry-with-backoff cycle (~60s) on every leaked subscription-charge transaction attempting a `CancelTransaction` call *guaranteed* to fail (these are always already `completed`, never cancelable) before falling through to the working credit/refund path — observed live as ~120s per transaction, roughly doubling real-world sweep time under sandbox rate limiting. Now skips the doomed attempt for `completed` transactions specifically.

### Changed

- `createAdjustmentFixtureTransaction`'s customer, address, transaction, product, and price fixtures now clean up immediately after each test (`t.Cleanup`) instead of relying solely on `sweep.yaml`'s weekly run — consistent with this repo's own stated sweeper design (sweepers as a crash safety net, not primary cleanup for a normal successful run). Previously left the sandbox account's customer/product/price counts growing by several on every CI push.
- Added a `tooRecentToSweep` age guard (15-minute threshold) to the customer sweeper, protecting an in-flight acceptance test's fixture from a sweep that happens to fire concurrently against the same live sandbox account.

### Documentation

- Documented the one-time manual sandbox precondition (a permanent `notification_setting` fixture) `paddle_notification`'s acceptance tests need to exercise their real filter/lookup logic instead of only their empty-account skip path — same posture as the existing default-payment-link and pinned-subscription preconditions.

## [0.4.0] - 2026-08-11

Stable graduation of the actions layer introduced in `0.4.0-beta.1` — every action now has a real-sandbox-verified round trip, closing out that release's "Note". Getting there surfaced three real bugs the sandbox alone could catch, all fixed below.

### Fixed

- `Transaction.Items[].price` is a nested object (`price.id`), not a flat `price_id` field, confirmed against the official Paddle SDKs — this had silently broken `paddle_subscription_charge`'s entire search-before-invoke item-matching the whole time (always comparing against an empty string). A separate field path, `Transaction.Details.LineItems[]`, carries the transaction-item `id` (`txnitm_...`) `paddle_adjustment`'s `item_id` actually references.
- `paddle_subscription_charge`'s search-before-invoke for `effective_from = "next_billing_period"` never worked at all, in any release: `NextTransactionPreview.Items` was decoded from a top-level `"items"` key that doesn't exist on the real response (the real items are under `details.line_items`, a third distinct item shape again), and even with that fixed, an exact-set match against a preview that always mixes the subscription's own recurring items in with any queued charge could never succeed. Confirmed the hard way: a real duplicate charge got queued against a live sandbox subscription's next renewal before both bugs were found and fixed. The resulting sandbox duplicate was left in place rather than force-removed — Paddle has no API to cancel a single queued one-time charge — and will bill normally next cycle for the sweeper to credit like any other test transaction.
- `paddle_adjustment`'s acceptance test built its Terraform config before `PreCheck` had set the fixture transaction ID (`resource.TestCase.Steps` is evaluated immediately, before `PreCheck` runs), sending an empty `transaction_id` on every apply.
- `ListAdjustments` sent `per_page=200`, exceeding the endpoint's actual max of 50.
- The sweeper now picks refund vs. credit by the target transaction's actual status (paid transactions need a refund, unpaid ones a credit) and treats an already-fully-adjusted transaction as success rather than a spurious warning on repeat sweep runs.
- `sweep.yaml` was scoped to `./internal/provider/...`, which included the `actions` subpackage that has no sweeper support at all — narrowed to `./internal/provider`.
- Paddle API error messages now include field-level detail (e.g. which field was invalid) instead of silently dropping it.

### Added

- `PADDLE_TEST_SUBSCRIPTION_ID` and `PADDLE_TEST_CANCELED_SUBSCRIPTION_ID` — two dedicated, pinned sandbox subscriptions (one kept `active`, one kept `canceled`) the subscription action tests target directly, replacing "whatever this account happens to have" and giving `pause`/`resume`/`charge`/the cancel short-circuit real, repeatable coverage instead of a permanent skip.
- Retry-with-backoff on both search-before-invoke checks in `paddle_subscription_charge`, mitigating read-after-write lag between a charge and Paddle's own search/preview endpoints catching up with it.
- A sweeper for the real invoices/transactions this test suite generates, so repeated CI runs don't accumulate sandbox billing noise.

## [0.4.0-beta.1] - 2026-08-10

### Added

- Terraform actions — this provider's first — for imperative Paddle operations with no resource lifecycle of their own: `paddle_adjustment` (refund/credit) and `paddle_subscription_cancel`/`paddle_subscription_pause`/`paddle_subscription_resume`/`paddle_subscription_charge`. Requires Terraform `>= 1.14.0`. Since Paddle has no idempotency-key mechanism anywhere in its API, every action goes through a no-blind-retry client path and a search-before-invoke check before mutating, rather than the automatic retry every other client call gets — see the new "Actions" section in `README.md` for the operational risks (real money movement, no `-auto-approve` without review, a separately-scoped API key) before using these in an automated pipeline.
- Daily `E2E (Published)` health-check workflow testing whatever's currently latest on the Terraform Registry, independent of any release.
- Sweepers now log matched/swept counts, not just failures — a clean run previously proved nothing failed, not whether anything was found and cleaned.

### Fixed

- The `E2E (Published)` workflow's discount group naming collided with itself on a retry of the same run (`github.run_id` alone doesn't change across retries, and Paddle enforces discount group name uniqueness even against archived groups).
- The `E2E (Published)` workflow's "latest version" resolution now excludes prerelease versions (like this release itself) — it previously crashed outright the moment a prerelease existed in the Registry's version list.
- `paddle_adjustment`'s acceptance test now skips cleanly, rather than failing, when the sandbox account has no default payment link configured — a real Paddle-side precondition for creating any transaction via the API at all, found by running against the live sandbox.

### Documentation

- Captured the v3 roadmap (actions scope, idempotency guardrails, implementation plan) as durable project knowledge before writing any v3 code, continuing the same convention v1/v2 used. Includes a resolved investigation into retrofitting search-before-create onto existing catalog resources — concluded, after prototyping, that the obvious fix would trade a low-severity issue for a worse one (state corruption from misadopting an unrelated object), so no code change was made there.

### Note

This is a beta pre-release: the actions above are verified against the real Paddle sandbox for their core paths (adjustments' full invoke-twice-confirm-once proof; the always-available subscription error path), but the subscription actions' real success paths (pause/resume round-trip, charge) have not yet been exercised against a real sandbox subscription — Paddle subscriptions can only be created via an actual checkout with a test card, which this release's automation couldn't provision. Expect a stable `0.4.0` once that manual verification is done.

## [0.3.1] - 2026-08-09

### Fixed

- Paddle API errors now surface a readable message (`error.detail`, with `error.code` in parentheses) in Terraform diagnostics instead of the raw JSON error envelope, across every resource and data source.
- The `~> 0.1` version constraint shown in `README.md`, `examples/provider/provider.tf` (the source of `docs/index.md`'s example), and `examples/full-stack/main.tf` was still pinning copy-pasted configs to `0.1.x` after both `0.2.0` and `0.3.0` had shipped — bumped to `~> 0.3`.
- `paddle_discount_group` and `paddle_notification_setting` were missing the `import.sh` example (and generated docs "Import" section) that `paddle_product`/`paddle_price`/`paddle_discount` already had.
- An unchecked `resp.Body.Close()` error return in the API client, caught by a new lint pass.
- A pre-tag review pass found the friendlier-error-message change above had also been applied to 8 call sites that decode a local `custom_data` field, never an actual API error — harmless (same fallback either way) but reverted for correctness.

### Added

- `examples/full-stack/main.tf` — a product, recurring price, discount group, discount, notification setting, and checkout domain lookup all wired together in one config, validated against the real provider schema, not just isolated single-resource snippets.
- `golangci-lint` CI job.

### Documentation

- Added `docs/guardrails/example-version-constraints-track-latest-minor.md` so the version-constraint staleness above doesn't recur silently on the next minor release.

## [0.3.0] - 2026-08-09

### Added

- `paddle_checkout_domain` data source — read-only lookup by ID. No matching resource: Paddle's API has no create or update operation for checkout domains at all (confirmed against the live API reference), a domain can only be added via the Paddle dashboard, so this provider only implements the lookup rather than an import-only resource whose `Create()` would always fail.

### Changed

- Post-release Terraform Registry smoke test (`.github/workflows/registry-smoke-test.yaml`) — confirms a released version actually installs and works via a real `terraform init` against `registry.terraform.io` and a real `apply`/`destroy` against the sandbox, since every other test in this repo builds the provider in-process from source and never exercises the published artifact itself.

### Documentation

- `README.md`'s Publishing section documents the new smoke-test step, and a new "Checkout domains" section explains the manual dashboard setup this data source depends on.

## [0.2.0] - 2026-08-09

### Added

- `custom_data` on `paddle_product`/`paddle_price`/`paddle_discount` — was already reachable client-side but not exposed on any resource schema; modeled as JSON-encoded string with a semantic-equality plan modifier so key-reordering from Paddle's own re-serialization doesn't cause a perpetual diff.
- `stringvalidator.OneOf` enum validation on `paddle_product`'s `tax_category`/`type` and `paddle_price`'s `tax_mode`, so a typo surfaces as a clear plan-time error instead of an opaque 400 from Paddle at apply time.
- `tflog` debug logging in the API client's single request chokepoint (method/path/attempt/status only — request/response bodies and the API key are never logged).
- Acceptance test sweepers for all five resources, backed by new cursor-paginated `List*` client methods, so sandbox objects orphaned by an interrupted test run can be cleaned up deliberately instead of accumulating.
- `paddle_discount_group` resource and data source, closing a gap where `paddle_discount`'s `discount_group_id` referenced an entity type this provider couldn't manage at all.
- `paddle_notification_setting` resource and data source — the first resource in this provider with a real hard `DELETE` instead of archive-via-update, a `RequiresReplace` attribute (`type`), and a confirmed request/response shape asymmetry on `subscribed_events` (array of strings in, array of event objects out).

### Fixed

- Discount group `name` acceptance tests now use a random suffix — Paddle enforces global uniqueness on the field, which broke when this repo's `push` and `pull_request` CI jobs ran the same fixed name concurrently against one shared sandbox account.
- `paddle_notification_setting`'s `api_version` is now `Optional+Computed` instead of purely user-set — Paddle silently returns its own account default even when the field is omitted, which produced "Provider produced inconsistent result after apply" on the very first real `Create`.
- A sweeper verification test's broad match could delete a *different* concurrently-running CI job's in-progress notification setting fixture (real hard `DELETE`, unlike the archive-based sweepers); sweeper mechanics were extracted into ID-scoped helpers so verification tests can no longer touch another job's fixtures while the real sweepers keep their intentionally broad match.

### Changed

- CI's acceptance job push-trigger branch updated from the now-merged `feature/v1` to `feature/v2`.

### Documentation

- `CONTRIBUTING.md` added — points at `README.md`'s existing dev-setup/publishing sections rather than duplicating them, and covers the `docs/{decisions,facts,guardrails,plans}/` knowledge-artifact convention plus the non-obvious per-resource patterns this provider has learned the hard way.
- `README.md` updated to list all five resources and the archive-vs-real-delete distinction, not just v1's three.
- `docs/plans/paddle-provider-v2.md` recorded a deferred follow-up: CI never validates that what actually lands on the Terraform Registry installs and works, since acceptance tests build the provider in-process from source rather than pulling it from `registry.terraform.io`.
- Checkout Domains (stretch resource) and the Registry provider logo were explicitly deferred rather than silently dropped, each with a reason recorded in the v2 plan.

## [0.1.0] - 2026-08-08

### Added

- `paddle_product` and `paddle_price` resources — initial scaffold, talking directly to Paddle's Billing API with no third party in the request path.
- Client retry/backoff for 429/5xx responses with full-jitter exponential delay, `Retry-After` support, bounded attempts, and context-cancellation awareness — plus the client-side CRUD methods for Discounts.
- `paddle_discount` resource and data source, with field list confirmed directly against Paddle's real API reference rather than guessed.
- `paddle_product` and `paddle_price` data sources (the resources existed already; read-only lookup by ID was the gap).

### Fixed

- Archive (destroy) PATCH bodies for products/prices no longer send empty-string values for required fields, and clearing an optional field (description, image URL, name) now actually clears it server-side instead of leaving it unchanged.
- `paddle_price`'s `product_id` now forces replacement instead of attempting an in-place update Paddle rejects; a stray `gofmt` issue and a missing `go.sum` (dependency versions were never actually pinned) were also fixed.
- `paddle_price`'s `quantity` attribute no longer crashes on the very first `Create` when left at its default — it now has a real schema default instead of relying on a plan modifier that only helps once state already exists.
- `paddle_price` updates no longer send `product_id` in the PATCH body — Paddle rejects the field outright if present at all, not just when its value changes.
- `paddle_price`'s `Read()` (and the post-import refresh that calls it) no longer crashes decoding a null nested `unit_price` — it now fetches only the `id` it actually needs instead of the whole model.
- A flaky acceptance test around context-cancellation timing was replaced with a deterministic unit test of the underlying wait logic.
- `paddle_discount`'s `code` attribute is now `Optional+Computed` (Paddle auto-generates a code server-side when one isn't supplied) instead of purely user-set, and no longer sends an empty string to Paddle when left unset.
- `price_data_source.go` had the same null-nested-attribute crash as `paddle_price`'s `Read()` — fixed the same way.
- Ten further issues from a full `/code-review high` pass were fixed, each with a regression test: an `Unknown` provider config value could silently clobber a valid `PADDLE_API_KEY`/`PADDLE_ENVIRONMENT` environment fallback; `fromAPIPrice` left a stale `quantity` value in state when Paddle's response omitted it; `Delete()` on all three resources didn't tolerate an object already gone (404) the way `Read()` did; `fromAPIDiscount`'s conversion errors weren't checked before writing state; `paddle_product`'s `Read()` was the one resource still doing a full, not narrowly-scoped, state decode; the HTTP client had no overall time budget across retry attempts; and a zero-delay retry wait could skip its own cancellation check.

### Changed

- GitHub Actions pinned to current major versions (`checkout`, `setup-go`, GPG import, GoReleaser) to avoid immediate Dependabot churn.
- Unit test coverage added for the pure JSON-marshaling and conversion logic that every early bug actually lived in, running in CI on every push with no credentials required.
- Full `TF_ACC`-gated acceptance test suite added for `paddle_product`/`paddle_price` (and later `paddle_discount`) against a real Paddle sandbox account, wired into a dedicated CI job.
- CI now installs the Terraform CLI explicitly rather than letting a dependency auto-download it, working around an expired signing key in that download path.
- Go bumped from 1.22 to 1.25, clearing dependency-version ceilings that had forced pinning `terraform-plugin-testing`, `terraform-plugin-framework-validators`, and `tfplugindocs` below their latest releases.

### Documentation

- Fixed the GPG signing-key upload instructions to point at the public Terraform Registry's own Signing Keys page, not the separate HCP Terraform portal.
- Synced the implementation plan's status tracking with what was actually already implemented (import support, provider auth schema had landed earlier than tracked).
- Recorded the real-sandbox bug patterns found while building acceptance test coverage for `paddle_price` and `paddle_discount`, so the same mistakes wouldn't recur in later resources.
- Generated Terraform Registry-facing docs (`docs/index.md`, `docs/resources/*.md`, `docs/data-sources/*.md`) via `tfplugindocs` from real usage examples, with a CI job that fails on drift from the schema.
- A full pre-merge review pass fixed stale comments, phantom references to code/decisions that no longer matched reality, and outdated status claims across the project's knowledge-artifact docs before merging into `master`.
