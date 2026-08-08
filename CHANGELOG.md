# Changelog

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
