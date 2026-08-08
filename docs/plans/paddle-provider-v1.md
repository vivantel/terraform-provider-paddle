---
title: Implementation plan — mature v1 of terraform-provider-paddle
status: not started
date: 2026-08-08
tags: [paddle, provider, plan]
---

# Implementation plan: terraform-provider-paddle v1

**Read this whole file before doing anything.** It is written to be
self-sufficient for a completely fresh Claude Code session with zero prior
context — no step below assumes you remember a conversation that isn't in
this repo.

Repo: `/home/ubuntu/projects/vivantel/terraform-provider-paddle`
(GitHub: `vivantel/terraform-provider-paddle`, moved from a personal
account on 2026-08-08).

## Why this plan exists

The user asked for a "mature" Terraform provider for the Paddle billing
domain, following best practices. A roadmap interview on 2026-08-08 turned
that into concrete decisions, recorded in:

- `docs/decisions/0001-catalog-only-scope-v1.md` through `0005-http-client-retry-backoff.md`
- `docs/facts/0001-existing-provider-baseline.md` through `0004-versioning-v0.1.0-and-changelog.md`
- `docs/guardrails/*.md` (5 files)
- `docs/skills/*.md` (2 files)

Read those before making any judgment call not spelled out below — they
carry the *why*, this file carries the *what/how*, in order.

## Ground truth before you start

- Go 1.22, HashiCorp Terraform Plugin Framework (not SDKv2).
- Module: `github.com/vivantel/terraform-provider-paddle`.
- Existing: `internal/provider/provider.go`, `internal/provider/product_resource.go`
  (`paddle_product`), `internal/provider/price_resource.go` (`paddle_price`),
  `internal/client/client.go` (Products + Prices endpoints only, Bearer auth,
  no retry logic yet).
- A Paddle sandbox account/API key already exists (user-provided, not stored
  in this repo — expect it via `PADDLE_API_KEY` env var locally, and as a
  GitHub Actions secret of the same name in CI).
- A GPG signing key for release artifacts does **not** exist yet and must be
  created (Step 0 below).
- `go` was not available in the shell used to write this plan — verify
  `go version` works before running any Go commands below; if it's missing,
  that's an environment problem to solve first, not part of this plan's
  scope.

## How to update this file as you work

Each step below has a `Status:` line. Update it in place
(`not started` → `in progress` → `done`, or `blocked: <reason>`) as you go,
so re-reading this file alone — in this session, a resumed one, or a brand
new one — tells you exactly what remains. Don't remove completed steps;
leave them marked `done` for the historical record.

Commit using `docs/skills/commit-with-kms-attribute.md` (Conventional
Commits + `Refs:` trailers to the relevant `docs/decisions/`,
`docs/guardrails/`, etc. file(s) each step implements).

---

## Step 0: Manual prerequisites (not automatable by an agent)

Status: not started

These need a human with access to GitHub org settings, a terminal for GPG,
and the Terraform Registry UI. List them explicitly so a fresh session
doesn't waste time trying to script around them.

- [ ] Generate a GPG key dedicated to signing this provider's releases
      (`gpg --full-generate-key`, RSA 4096 recommended). Export the private
      key (`gpg --export-secret-keys --armor <key-id>`) and store it as the
      GitHub Actions secret `GPG_PRIVATE_KEY` on the
      `vivantel/terraform-provider-paddle` repo; store its passphrase (if
      any) as `PASSPHRASE`.
  - See [[0004-release-via-goreleaser-github-actions]] for why this is
    required (Terraform Registry verifies the signature on `SHA256SUMS`
    before trusting a provider).
  - See [[0004-versioning-v0.1.0-and-changelog]] for how this GPG key
    relates to the `v0.1.0` first release.
- [ ] Store the existing sandbox API key as the GitHub Actions secret
      `PADDLE_API_KEY` on the same repo (see
      [[0002-paddle-sandbox-account-available]] — the key itself already
      exists, this is just wiring it into CI).
- [ ] Register `vivantel/terraform-provider-paddle` with the Terraform
      Registry's "Publish a provider" flow (registry.terraform.io → Publish
      → Provider → connect the GitHub App), and upload the GPG public key
      generated above to the Registry publisher account. This is what makes
      the Registry actually ingest releases once GoReleaser starts producing
      them — without it, GitHub Releases will exist but never show up on the
      Registry.

Nothing in Steps 1+ below is blocked on this step being done first — the
code/CI work can proceed in parallel — but no tagged release will actually
reach the Terraform Registry until Step 0 is complete.

---

## Step 1: Client — retry/backoff + Discounts endpoints

Status: not started

Implements: [[0005-http-client-retry-backoff]], and the client-side prep for
`paddle_discount` from [[0001-catalog-only-scope-v1]].

File: `internal/client/client.go`

1. Add retry/backoff to the `do` method:
   - Retry on `429` and any `5xx` status code.
   - Bounded exponential backoff with jitter (e.g. base 500ms, factor 2,
     max 5 attempts, cap individual sleep at ~10s) — a small local helper
     function is fine, no need for an external dependency.
   - Honor a `Retry-After` header on 429 responses if present (seconds or
     HTTP-date per RFC 7231), falling back to the exponential schedule if
     absent or unparsable.
   - The retry loop must respect `ctx` — if the context is cancelled or its
     deadline passes, stop retrying and return that error immediately,
     don't sleep through it.
   - Non-retryable errors (other 4xx, or the final failed attempt) still
     return `*APIError` as today — no change to that type or its `Error()`
     format.
2. Add Discount types and methods, mirroring the existing `Product`/`Price`
   pattern in the same file (see the existing `Product` struct and its
   `productEnvelope` type for the shape to follow):
   - `type Discount struct { ID, Type (string: "flat" or "percentage"),
     Amount *string, Currency *string (for flat discounts only), Description
     string, EnabledForCheckout *bool, CustomData map[string]any, Recur
     *bool, MaximumRecurringIntervals *int, UsageLimit *int, StartsAt/
     ExpiresAt *string, Status string }` — cross-check exact field names/
     types against
     https://developer.paddle.com/api-reference/discounts/overview before
     finalizing; the API reference is the source of truth, this list is a
     starting sketch, not a spec.
   - `CreateDiscount`, `GetDiscount`, `UpdateDiscount`, `ListDiscounts` (or
     just the CRUD subset the resource needs — Paddle discounts don't
     support hard delete, only archiving via `status: "archived"`, confirm
     this against the API docs and design `DeleteDiscount` as an archive
     call, not an actual DELETE).
3. Unit tests for the retry logic specifically (stubbed `http.RoundTripper`
   returning 429 then 200, verifying it retries and eventually succeeds;
   returning 429 `Retry-After: 1` and asserting the sleep respects it;
   returning persistent 500s and asserting it gives up after max attempts
   and returns `*APIError`). These are plain unit tests, not acceptance
   tests — no `TF_ACC` gate needed here.

## Step 2: `paddle_discount` resource + data source

Status: not started

Implements: [[0001-catalog-only-scope-v1]],
`docs/guardrails/catalog-resources-need-data-source.md`,
`docs/guardrails/resources-need-import-support.md`,
`docs/guardrails/client-calls-must-use-retry-wrapper.md`.

Files: `internal/provider/discount_resource.go`,
`internal/provider/discount_data_source.go` (new), plus registration in
`internal/provider/provider.go`'s `Resources()`/`DataSources()` methods.

1. Model `discount_resource.go` closely on the existing
   `product_resource.go` / `price_resource.go` structure (same
   `resource.Resource` interface shape, same pattern for Create/Read/Update/
   Delete calling into `internal/client`). Attributes should cover at least:
   `type`, `amount`, `currency_code` (flat only), `description`,
   `enabled_for_checkout`, `recur`, `maximum_recurring_intervals`,
   `usage_limit`, `custom_data`, `status`. Use `stringplanmodifier`/
   `boolplanmodifier` `RequiresReplace()` on any field Paddle's API doesn't
   support updating in place (check the API reference).
2. Implement `ImportState` via `resource.ImportStatePassthroughID` against
   the discount's Paddle ID (per
   `docs/guardrails/resources-need-import-support.md`).
3. `Delete` calls the archive-not-hard-delete method from Step 1 — document
   this clearly in the resource's schema `MarkdownDescription` so users
   aren't surprised that `terraform destroy` archives rather than deletes.
4. `discount_data_source.go`: read-only lookup by ID (and/or by matching
   attributes if Paddle's list-with-filter API supports it cleanly — ID
   lookup alone is the minimum bar per
   `docs/guardrails/catalog-resources-need-data-source.md`).
5. Register both in `provider.go`.

## Step 3: Retrofit `paddle_product` / `paddle_price`

Status: not started

Implements: `docs/guardrails/catalog-resources-need-data-source.md`,
`docs/guardrails/resources-need-import-support.md`.

The two existing resources predate these guardrails and don't yet meet them.

1. Add `ImportState` (via `resource.ImportStatePassthroughID`) to both
   `product_resource.go` and `price_resource.go`. Note: a Price's ID alone
   may not be enough context depending on how Paddle scopes price lookups —
   check whether `GetPrice` needs a product ID too; if so, design the import
   ID format accordingly (e.g. `<price_id>` if Paddle's price lookup is
   global, or document a composite import ID if not).
2. Add `internal/provider/product_data_source.go` and
   `internal/provider/price_data_source.go`, same pattern as Step 2's
   discount data source.
3. Register the two new data sources in `provider.go`.

## Step 4: Provider-level auth schema

Status: not started

Implements: [[0002-provider-auth-schema-with-env-fallback]].

File: `internal/provider/provider.go`

1. Add `api_key` (String, Optional, Sensitive) and `environment` (String,
   Optional) to the provider's `Schema()`.
2. In `Configure()`: read `api_key` from config, falling back to
   `os.Getenv("PADDLE_API_KEY")` if unset (and unknown-check per Plugin
   Framework convention — `req.Config.Get...` then check `.IsNull()`);
   same pattern for `environment` falling back to
   `os.Getenv("PADDLE_ENVIRONMENT")`, defaulting to `"production"` if both
   are unset. Map `"sandbox"`/`"production"` to the existing
   `client.SandboxBaseURL`/`client.ProductionBaseURL` constants; return a
   clear `resp.Diagnostics.AddError` if `environment` is set to anything
   else.
3. If `api_key` ends up empty after checking both sources, add an error
   diagnostic rather than constructing a client that will just 401 on first
   use.

## Step 5: Acceptance test suite

Status: not started

Implements: [[0003-acceptance-tests-against-live-sandbox]],
`docs/guardrails/acceptance-tests-require-tf-acc-gate.md`.

1. Add `internal/provider/provider_test.go` (if it doesn't already exist)
   with a shared `testAccPreCheck(t *testing.T)` that `t.Skip`s when
   `PADDLE_API_KEY` is unset, and a `testAccProtoV6ProviderFactories` map
   for use across test files (standard Plugin Framework acceptance test
   boilerplate — see the `terraform-provider-development:provider-test-patterns`
   skill in this environment for the exact pattern).
2. For each of `paddle_product`, `paddle_price`, `paddle_discount`:
   basic create/read test, an update test, an import test
   (`ImportState: true, ImportStateVerify: true`), and a `CheckDestroy` that
   calls the client directly to confirm the object is actually gone (or
   archived, for discounts) in the sandbox after `terraform destroy`.
3. Confirm locally with `TF_ACC=1 PADDLE_API_KEY=<sandbox key> go test ./... -v`
   before relying on CI to catch failures.

## Step 6: CI workflows

Status: not started

Implements: `docs/guardrails/docs-must-be-regenerated-before-merge.md`,
`docs/guardrails/acceptance-tests-require-tf-acc-gate.md`,
[[0004-release-via-goreleaser-github-actions]].

File: `.github/workflows/ci.yml` (new)

1. `unit` job: `go build ./...`, `go vet ./...`, `go test ./...` (no
   `TF_ACC`, no secrets — must pass with zero external dependencies) on
   every push/PR.
2. `acceptance` job: same repo, sets `TF_ACC=1` and
   `PADDLE_API_KEY: ${{ secrets.PADDLE_API_KEY }}`, runs
   `go test ./... -run TestAcc -v`. Gate this to run on PRs from
   trusted branches only (not forks) since it uses a real secret — use
   `pull_request_target` carefully or restrict to `push` on `main` plus
   manual `workflow_dispatch`, whichever fits the repo's actual trust model;
   don't expose `PADDLE_API_KEY` to arbitrary fork PRs.
3. `docs` job: run `tfplugindocs generate`, then `git diff --exit-code` —
   fail if generation produced any diff against committed `docs/index.md` /
   `docs/resources/*.md` / `docs/data-sources/*.md`.

File: `.github/workflows/release.yml` (new)

4. Triggered on tag push matching `v*`. Imports `GPG_PRIVATE_KEY` +
   `PASSPHRASE` secrets (see Step 0), runs `goreleaser release --clean`.

File: `.goreleaser.yml` (new, repo root)

5. Standard `terraform-provider-scaffolding-framework` GoReleaser config:
   builds for `darwin/linux/windows` × `amd64/arm64` (skip
   `windows/arm64` if following the common HashiCorp template exactly —
   check the current
   `hashicorp/terraform-provider-scaffolding-framework` template's
   `.goreleaser.yml` for the up-to-date target matrix and signing config
   rather than hand-rolling it), produces `terraform-registry-manifest.json`
   from `metadata.json`/version, sha256sums + GPG signs them.

File: `terraform-registry-manifest.json` (new, repo root) — required by the
Registry to know which protocol versions this provider supports (Plugin
Framework providers use protocol version 6).

## Step 7: Docs

Status: not started

Implements: [[0003-docs-via-tfplugindocs]].

1. `go install github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@latest`
   (or add as a tool dependency per Go 1.22 tool-dependency conventions).
2. Add `templates/` (if any resource needs example usage beyond what
   `MarkdownDescription` on each schema attribute already documents) and
   `examples/` directories per `tfplugindocs` convention
   (`examples/resources/paddle_product/resource.tf`, etc. — these `.tf`
   files get embedded into generated docs).
3. Run `tfplugindocs generate`, commit the resulting `docs/index.md`,
   `docs/resources/*.md`, `docs/data-sources/*.md`.
4. Update `README.md`: replace the `dev_overrides`-only workflow description
   with a note that the provider is now published to the Terraform Registry
   as `vivantel/paddle` once Step 0 + a `v0.1.0` tag are done — keep the
   `dev_overrides` section too, since it's still useful for local iteration.

## Step 8: First release

Status: not started

Implements: [[0004-versioning-v0.1.0-and-changelog]],
`docs/skills/release-with-kms-changelog.md`.

1. Confirm Step 0 is fully done (GPG key in secrets, sandbox key in
   secrets, repo registered with Terraform Registry).
2. Run the `kms:changelog` skill to generate the initial `CHANGELOG.md` from
   full commit history (no prior tag exists yet).
3. Commit the changelog.
4. `git tag v0.1.0 && git push origin v0.1.0` — this triggers
   `.github/workflows/release.yml` from Step 6.
5. Verify the GitHub Release was created with signed artifacts, then verify
   the provider appears on the Terraform Registry at
   `registry.terraform.io/providers/vivantel/paddle` within a few minutes
   (Registry ingestion via the GitHub App webhook is not instant).

---

## Definition of done for this plan

- All of Steps 1–8 marked `done` above.
- `go build ./...`, `go vet ./...`, `go test ./...` pass with no `TF_ACC` set.
- `TF_ACC=1 go test ./... -run TestAcc` passes against the sandbox.
- `tfplugindocs generate` produces no diff.
- `v0.1.0` is live on the Terraform Registry.
- Every commit made for this plan carries `Refs:` trailers per
  `docs/skills/commit-with-kms-attribute.md`.
