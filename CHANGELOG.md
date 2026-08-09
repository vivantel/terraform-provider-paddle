# Changelog

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
