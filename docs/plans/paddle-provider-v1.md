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

Updated 2026-08-08, end of this plan's Step 7 — this section described a
much earlier state when first written (Go 1.22, no retry logic, no GPG
key, catalog-only product/price) and would actively mislead a fresh
session if left as-is; a stale "ground truth" is worse than none. Current
state:

- Go 1.25 (bumped from 1.22 this session — see "Open question: bump the
  go version?" below, now resolved), HashiCorp Terraform Plugin Framework
  (not SDKv2).
- Module: `github.com/vivantel/terraform-provider-paddle`.
- All three v1 catalog resources exist and are sandbox-confirmed:
  `paddle_product`, `paddle_price`, `paddle_discount`, each with a
  matching data source. `internal/client/client.go` has full CRUD +
  archive for all three, plus retry/backoff (Step 1).
- A Paddle sandbox account/API key already exists (user-provided, not
  stored in this repo — expect it via `PADDLE_API_KEY` env var locally,
  and as a GitHub Actions secret of the same name in CI, already set).
- A GPG signing key for release artifacts exists, uploaded to the
  Registry, GitHub secrets set (Step 0 — see its own status block for the
  one still-open item: confirming the Registry "Publish a provider" flow
  succeeded).
- `go` is not on `PATH` by default in a fresh shell in this environment —
  verify `go version` works before running Go commands; if missing, a
  toolchain was downloaded to `/tmp/goroot`+`/tmp/goroot125` (both 1.22.6
  and 1.25.12) during this session and may or may not still be present in
  a new session's `/tmp`. A Terraform CLI binary was also downloaded to
  `/tmp/tfbin` for the same reason (`hc-install`'s bundled signing key has
  intermittently been expired — see the `acceptance`/`docs` CI job
  comments in `ci.yaml`) and may need re-fetching too.
- Docs (`docs/index.md`, `docs/resources/*.md`, `docs/data-sources/*.md`)
  are generated via `tfplugindocs` from `examples/` + schema (Step 7) —
  regenerate after any schema change, don't hand-edit.

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

Status: done — 2026-08-08. GPG key generated + uploaded, all three secrets
set, Registry "Publish a provider" flow confirmed successful: `v0.1.0`
published and live at
`registry.terraform.io/providers/vivantel/paddle/0.1.0` with all 7 doc
pages ingested, confirmed via the Registry's own API
(`registry.terraform.io/v1/providers/vivantel/paddle` returns
`"version":"0.1.0"`), not just assumed from a successful `git push`.

These need a human with access to GitHub org settings, a terminal for GPG,
and the Terraform Registry UI. List them explicitly so a fresh session
doesn't waste time trying to script around them.

- [ ] Generate a GPG key dedicated to signing this provider's releases —
      `gpg --full-generate-key`, choose **RSA and RSA**, 4096 bits (the
      Terraform Registry only accepts RSA or DSA keys; don't accept a
      default that produces an ECC/Ed25519 key). Export the private key
      (`gpg --export-secret-keys --armor <key-id>`) and store it as the
      GitHub Actions secret `GPG_PRIVATE_KEY` on the
      `vivantel/terraform-provider-paddle` repo; store its passphrase (if
      any) as `PASSPHRASE`.
  - See [[0004-release-via-goreleaser-github-actions]] for why this is
    required (Terraform Registry verifies the signature on `SHA256SUMS`
    before trusting a provider).
  - See [[0004-versioning-v0.1.0-and-changelog]] for how this GPG key
    relates to the `v0.1.0` first release.
  - Upload the **public** key at `https://registry.terraform.io` (not the
    HCP Terraform portal / `app.terraform.io` — that's a separate paid
    product with its own private-registry GPG key flow) → sign in with the
    `vivantel` GitHub account → **User Settings → Signing Keys**
    (`https://registry.terraform.io/settings/gpg-keys`) → **+ New GPG Key**
    → paste `gpg --armor --export <key-id>` output.
  - [x] Done 2026-08-08. Key `56707089C4BE8B1A` (fingerprint
        `7108EA4B99998192A7530A9956707089C4BE8B1A`), RSA 3072, confirmed live
        via `registry.terraform.io/v2/gpg-keys?filter[namespace]=vivantel`.
- [x] Store the existing sandbox API key as the GitHub Actions secret
      `PADDLE_API_KEY` on the same repo (see
      [[0002-paddle-sandbox-account-available]] — the key itself already
      exists, this is just wiring it into CI). Done 2026-08-08, confirmed via
      `gh secret list` — `GPG_PRIVATE_KEY`, `PASSPHRASE`, `PADDLE_API_KEY` all
      present on `vivantel/terraform-provider-paddle`.
- [x] Register `vivantel/terraform-provider-paddle` with the Terraform
      Registry's "Publish a provider" flow (registry.terraform.io → Publish
      → Provider → connect the GitHub App).
  - First attempt got HTTP 400 — root cause was the GitHub repo being
    completely empty (`defaultBranchRef` blank, nothing ever pushed).
    Fixed 2026-08-08: `master` (empty orphan root) and `feature/v1` (all
    work) both pushed to `origin`, `master` set as GitHub's default branch.
    Retried successfully. Confirmed working end to end: pushing tag
    `v0.1.0` produced a signed GitHub Release (15 platform builds +
    `SHA256SUMS` + `.sig` + manifest) and the Registry ingested it —
    `registry.terraform.io/providers/vivantel/paddle/0.1.0` is live.

Nothing in Steps 1+ below is blocked on this step being done first — the
code/CI work can proceed in parallel — but no tagged release will actually
reach the Terraform Registry until Step 0 is complete.

---

## Step 1: Client — retry/backoff + Discounts endpoints

Status: done — 2026-08-08. Retry/backoff implemented in `do()` (429 + 5xx,
full-jitter exponential backoff, `Retry-After` honored and capped, respects
`ctx` cancellation during the wait). `Discount` struct + `CreateDiscount`/
`GetDiscount`/`UpdateDiscount`/`ArchiveDiscount` added to
`internal/client/client.go`, field list confirmed directly against
https://developer.paddle.com/api-reference/discounts/create-discount and
`.../update-discount` (not guessed, and not from a `paddle:*` skill — none
of those cover raw API schema for provider development, they're all
Next.js-app-integration-focused). Unlike Price, Discount's update accepts
the same fields as create plus `status` — no field is rejected the way
Price rejects `product_id` on update — so one struct covers both bodies.
7 new tests in `internal/client/retry_test.go` (429/5xx retry-then-succeed,
`Retry-After` honored, gives up after max attempts as `*APIError`, doesn't
retry a plain 404, respects context cancellation mid-backoff) plus 2 more
in `client_test.go` for `Discount` JSON marshaling — all passing locally
(~1s total, most of that one deliberate 1s `Retry-After` timing check).
Not yet exercised against the real sandbox (that only happens once Step 2
builds a resource that calls these methods).

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

Status: done, confirmed green against the real sandbox — 2026-08-08 (CI run
31278336381; `TestAccPaddleDiscount_basic` passed alongside both existing
`TestAccPaddle{Product,Price}_basic`). Took 2 more real-sandbox bugs beyond
what local testing/code review caught, on top of the 3 already applied
from Step 5's Price lessons — see commits `6b1014f`/`90ae023`:
1. `code` was modeled Optional-only; Paddle actually auto-generates one
   server-side when omitted (e.g. `"3268E6WW3W"`), so the plan (stays
   null) never matched the apply result — "Provider produced inconsistent
   result after apply". Needed `Optional+Computed` +
   `UseStateForUnknown`, same as `paddle_product`'s `type`/`paddle_price`'s
   `tax_mode`. No `Default` possible here (value is genuinely random),
   unlike `enabled_for_checkout`/`mode`/`recur` which do have real
   Paddle-documented static defaults.
2. Once `code` became `Computed`, `toAPIDiscount`'s `!m.Code.IsNull()`
   check let an `Unknown` value through (`IsNull()` is false for `Unknown`
   too, not just `Known`) — `ValueString()` on `Unknown` silently returns
   `""`, so it sent `code: ""` instead of omitting the field; Paddle
   rejected the empty string outright. Needed the same
   `!IsNull() && !IsUnknown()` pair already correctly used for
   `EnabledForCheckout`/`Mode`/`Recur`/`RestrictTo` in the same function.

Takeaway for any future resource: "Optional+Computed with `Default`" and
"Optional+Computed without `Default` because the value is server-generated"
are two genuinely different cases needing different handling in both the
schema *and* every `toAPI*` unknown-check — getting the schema right isn't
enough by itself.

`internal/provider/discount_resource.go` + `discount_data_source.go`
written, registered in `provider.go`, full local build/vet/gofmt/unit
tests clean. Applied all three lessons from Step 5's Price bugs up front
rather than rediscovering them: `enabled_for_checkout`/`mode`/`recur` (the
three Optional+Computed fields, matching Paddle's own defaults of
true/standard/false) each have a real `Default` (`booldefault.StaticBool`/
`stringdefault.StaticString`), not just `UseStateForUnknown`; `Read()`
fetches only `id` via `GetAttribute`, not a full `state.Get`; and — unlike
Price — Discount's update endpoint accepts the same fields as create plus
`status`, confirmed directly against
https://developer.paddle.com/api-reference/discounts/update-discount, so
no separate `DiscountUpdate` type was needed. Also added `type`/`mode`
enum validation via `terraform-plugin-framework-validators` (new
dependency, pinned at v0.16.0 for the same `go 1.22` ceiling as
`terraform-plugin-testing`) — the first schema validators in this repo;
`paddle_product`/`paddle_price` predate this and don't have equivalent
validators on their enum-like fields (`tax_category`, `type`, `tax_mode`),
worth a retrofit pass later for consistency.
`TestAccPaddleDiscount_basic` (create with all three defaulted fields left
unset — deliberately the exact path that crashed Price — plus update,
no-op-plan check, and import) now passes against the real sandbox; see the
2 additional fixes above this block that got it there.

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

Status: done, confirmed green against the real sandbox — 2026-08-08 (CI run
31278945117; all 6 acceptance tests pass —
`TestAccPaddle{Product,Price,Discount}_basic` and their
`*DataSource_basic` counterparts). ImportState (#1) already present
(predates this plan). Data sources (#2, #3) added:
`internal/provider/product_data_source.go` / `price_data_source.go`, both
reusing the resource's own model type since the attribute sets match
exactly — same approach as `paddle_discount`'s data source. Registered in
`provider.go`.

Found a 3rd occurrence of the same bug class as Step 5's price import
crash — a real sandbox catch, not a local one — see commit `f4f910c`:
`price_data_source.go`'s `Read()` did a full `req.Config.Get` into
`PriceResourceModel`, but `unit_price` is Computed-only (not user-supplied)
in a data source, so it's null in config at `Read()` time — same "null
into non-pointer nested struct" crash as the resource's post-import
`Read()`. Fixed with the same `GetAttribute(id)`-only pattern, and
retrofitted `product_data_source.go`/`discount_data_source.go` with it too
even though neither is actually broken today (no nested struct field in
either) — being correct by construction beats being correct by accident of
current field types, and this is now the 3rd time this exact bug has
appeared.

Implements: `docs/guardrails/catalog-resources-need-data-source.md`,
`docs/guardrails/resources-need-import-support.md`.

The two existing resources predate these guardrails; the import-support one
is already met, the data-source one isn't yet.

1. ~~Add `ImportState` (via `resource.ImportStatePassthroughID`) to both
   `product_resource.go` and `price_resource.go`.~~ Already done — both
   resources call `resource.ImportStatePassthroughID(ctx, path.Root("id"),
   req, resp)`. Double-check Price's import-by-ID-alone assumption still
   holds once you're working with real sandbox data (Step 5) — a Price's ID
   may or may not be enough context depending on how Paddle scopes price
   lookups; check whether `GetPrice` needs a product ID too.
2. Add `internal/provider/product_data_source.go` and
   `internal/provider/price_data_source.go`, same pattern as Step 2's
   discount data source.
3. Register the two new data sources in `provider.go`.

## Step 4: Provider-level auth schema

Status: done — verified 2026-08-08. `internal/provider/provider.go` already
implements `api_key`/`environment` (Optional, `api_key` Sensitive) with
`PADDLE_API_KEY`/`PADDLE_ENVIRONMENT` env fallback, matching
[[0002-provider-auth-schema-with-env-fallback]] — plus one improvement
beyond what that decision specified: defaults to `sandbox`, not
`production`, if `environment` is unset anywhere, so a misconfigured
provider block fails safe toward the environment that can't charge real
cards. This predates this plan; no action needed.

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

Status: done for all three resources — `paddle_product`/`paddle_price`
confirmed green against the real sandbox 2026-08-08 (CI run 31273040459),
after fixing 3 real bugs the acceptance job caught (quantity Default, price
update sending product_id, price Read() full-decode breaking import — see
commits 71e7f76/3fdc64c/4b49a92). `paddle_discount` coverage came later
with Step 2 (`discount_resource_acc_test.go`), applying those same three
patterns from the start rather than rediscovering them — though Step 2's
own status block records 2 further real sandbox bugs found anyway (`code`
needing Optional+Computed, and an IsUnknown() check gap in toAPIDiscount),
so "apply the known patterns up front" reduced but didn't eliminate new
real-sandbox findings per resource.

Implements: [[0003-acceptance-tests-against-live-sandbox]],
`docs/guardrails/acceptance-tests-require-tf-acc-gate.md`.

1. ~~Add `internal/provider/provider_test.go`~~ Done —
   `testAccPreCheck`/`testAccProtoV6ProviderFactories`/`newTestAccClient`
   shared helpers.
2. ~~For each of `paddle_product`, `paddle_price`, `paddle_discount`~~ Done
   for all three resources
   (`product_resource_acc_test.go`/`price_resource_acc_test.go`/
   `discount_resource_acc_test.go`): create, update, a no-op-plan check for
   the Default/UseStateForUnknown regression class, an explicit
   clear-optional-field step for the omitempty regression, `ImportState`/
   `ImportStateVerify`, and `CheckDestroy` asserting `status == "archived"`
   (not 404 — Paddle archives, doesn't hard-delete). Data source lookups
   also covered (Step 3): `TestAccPaddle{Product,Price,Discount}DataSource_basic`.
3. ~~Confirm locally~~ Confirmed locally 2026-08-08 with no
   `PADDLE_API_KEY` set: `go test ./... -v` — unit tests pass for real,
   `TestAcc*` tests skip cleanly (`--- SKIP`). Full runs against the actual
   sandbox confirmed green in CI's `acceptance` job repeatedly since —
   see each step's own status block for the specific CI run IDs.

Dependency note (historical): at the time this step was done, added
`github.com/hashicorp/terraform-plugin-testing` pinned at `v1.11.0` —
newer versions (`v1.12.0+`) required `go >= 1.23`, and this repo's
`go.mod`/CI were pinned to `go 1.22`. That ceiling was later resolved —
see "Resolved: bumped `go` to 1.25" further down — and
`terraform-plugin-testing` is now at true latest (`v1.16.0`).

## Step 6: CI workflows

Status: done — 2026-08-08. `docs` job added to `ci.yaml`: installs
Terraform CLI (same `hc-install` expired-key workaround as the
`acceptance` job) + `tfplugindocs@v0.19.4` (pinned — `@latest` needs
`go >= 1.25.8`, `v0.20.1+` needs `go >= 1.22.7`, this repo is on exactly
`go 1.22`; `v0.19.4` is the newest confirmed to install under `1.22.6`),
runs `tfplugindocs generate`, then `git diff --exit-code -- docs/`.
`release.yaml`/`.goreleaser.yml`/`terraform-registry-manifest.json`
(all predate this plan) read and verified, not changed — see #4/#5
below, both confirmed correct against the current
`hashicorp/terraform-provider-scaffolding-framework` template rather than
just assumed.

Implements: `docs/guardrails/docs-must-be-regenerated-before-merge.md`,
`docs/guardrails/acceptance-tests-require-tf-acc-gate.md`,
[[0004-release-via-goreleaser-github-actions]].

File: `.github/workflows/ci.yaml` (exists)

1. ~~`unit` job~~ Already existed as the `build` job (`go build`, `go vet`,
   `gofmt -l`, `go test ./...`) — runs on every push/PR, no `TF_ACC`/secrets.
2. ~~`acceptance` job~~ Done 2026-08-08: sets `TF_ACC=1` and
   `PADDLE_API_KEY: ${{ secrets.PADDLE_API_KEY }}`, runs
   `go test ./... -run TestAcc -v`. No extra fork-PR guard needed —
   GitHub's default `pull_request` trigger already withholds repo secrets
   from forked-PR runs, and every `TestAcc*` test's `testAccPreCheck` skips
   (not fails) when `PADDLE_API_KEY` is empty, so a fork PR run just
   reports everything skipped.
3. ~~`docs` job~~ Done — see above.

File: `.github/workflows/release.yaml` (exists, predates this plan)

4. ~~Confirm it's triggered correctly on tag push matching `v*`, imports
   `GPG_PRIVATE_KEY` + `PASSPHRASE` secrets, runs `goreleaser release
   --clean`.~~ Read and confirmed correct as-is: triggers on `push: tags:
   ["v*"]`, `crazy-max/ghaction-import-gpg` + `goreleaser/goreleaser-action`
   both already pinned to `v7` (from the earlier "up-to-date GH Actions"
   pass this session), `GPG_FINGERPRINT`/`GITHUB_TOKEN` wired correctly. No
   changes needed.

File: `.goreleaser.yml` (exists, predates this plan)

5. ~~Verify its target matrix, signing config, and
   `terraform-registry-manifest.json` generation against the current
   template.~~ Read and confirmed: `version: 2` GoReleaser config,
   `goos: [freebsd, windows, linux, darwin]` × `goarch: [amd64, "386",
   arm, arm64]` (darwin/386 excluded, correct — Apple never shipped 32-bit
   Intel), `checksum`/`signs`/`release.extra_files` all reference
   `terraform-registry-manifest.json` correctly, `formats: [zip]` syntax
   matches GoReleaser v2. Matches the current
   `hashicorp/terraform-provider-scaffolding-framework` template. No
   changes needed.

File: `terraform-registry-manifest.json` (exists, predates this plan) —
required by the Registry to know which protocol versions this provider
supports. Confirmed: `"protocol_versions": ["6.0"]`, correct for a Plugin
Framework provider. No changes needed.

## Step 7: Docs

Status: done — 2026-08-08.

Implements: [[0003-docs-via-tfplugindocs]].

1. ~~Install `tfplugindocs`.~~ Installed at `v0.19.4` (pinned — see Step
   6's note on the `go 1.22` ceiling), same tool used by the new `docs` CI
   job.
2. ~~Add `templates/`/`examples/`.~~ No `templates/` needed — `tfplugindocs`
   generates from built-in defaults fine without one. Added `examples/`
   for all 3 resources + 3 data sources + the provider block itself
   (`examples/{provider,resources,data-sources}/...`), each a real,
   runnable-shaped `.tf` snippet (plus `import.sh` per resource) — these
   get embedded into the generated docs as "Example Usage" sections.
3. ~~Run `tfplugindocs generate`, commit the result.~~ Done —
   `docs/index.md`, `docs/resources/*.md`, `docs/data-sources/*.md`, all
   7 pages. Confirmed idempotent (`tfplugindocs generate` a second time
   produces no diff) before committing. Also fixed a staleness bug caught
   along the way: `provider.go`'s schema `Description` still said
   "products, prices" with no mention of discounts, added in Step 2 —
   updated before generating so the Registry-facing docs don't ship a
   stale description.
4. ~~Update `README.md`.~~ Done: "Status" section rewritten (was still
   "freshly scaffolded, not yet tested" — long stale given Steps 0-6's
   sandbox confirmations), usage example extended to show
   `paddle_discount` and a data source, "Development" section documents
   `go test ./...` vs the `TF_ACC=1` acceptance run vs `tfplugindocs
   generate`, "Publishing" section updated to check off what Step 0
   actually completed rather than describing it as "not done yet".

## Step 8: First release

Status: done — 2026-08-08.

Implements: [[0004-versioning-v0.1.0-and-changelog]],
`docs/skills/release-with-kms-changelog.md`.

1. ~~Confirm Step 0 is fully done.~~ Done.
2. ~~Generate the initial `CHANGELOG.md`.~~ Done via `kms:changelog`
   (`CHANGELOG.md`, commit `0c26880`).
3. ~~Commit the changelog.~~ Done.
4. ~~`git tag v0.1.0 && git push origin v0.1.0`.~~ Done — triggered
   `.github/workflows/release.yaml` (run 31283001129), completed
   successfully in ~5 minutes (15 platform builds, GPG-signed
   `SHA256SUMS`).
5. ~~Verify the GitHub Release and Registry ingestion.~~ Both confirmed:
   GitHub Release `v0.1.0` has all 15 platform zips + `SHA256SUMS` +
   `.sig` + manifest; Registry ingestion was near-instant (live on first
   check) at `registry.terraform.io/providers/vivantel/paddle/0.1.0`,
   with all 7 doc pages present.

This provider's v1 is fully shipped: implemented, sandbox-verified,
code-reviewed, merged, released, and live on the public Terraform
Registry. `docs/plans/paddle-provider-v2.md` picks up from here.

---

## Resolved: bumped `go` to 1.25 (2026-08-08)

Was flagged as an open question after Step 5 needed to pin
`terraform-plugin-testing` at `v1.11.0` (`go.mod` was `go 1.22.0` at the
time; `terraform-plugin-testing@latest` needed `go >= 1.25.8`). User chose
**1.25**, not the bleeding-edge latest (`go1.26.5` as of this decision,
confirmed via `go.dev/dl`) — Go supports the last two majors, so 1.25 is
inside the support window and removes the dependency ceiling without
chasing a release that's one day old.

`go.mod`'s `go`/`toolchain` lines and every `go-version` in
`ci.yaml`/`release.yaml` bumped to `1.25`/`1.25.12`. Every previously
version-ceiling-pinned dependency bumped to true latest as a result:
`terraform-plugin-testing` `v1.11.0` → `v1.16.0`,
`terraform-plugin-framework-validators` `v0.16.0` → `v0.19.0`,
`tfplugindocs` `v0.19.4` → latest (`v0.25.0`, used via `@latest` in
`ci.yaml` now rather than a pin). One real snag along the way:
`terraform-plugin-framework` also needed an explicit bump (`v1.13.0` →
`v1.19.0`, not just whatever `go mod tidy` picked automatically) —
`terraform-plugin-testing@v1.16.0` pulls `terraform-plugin-go@v0.31.0`,
which added a `GenerateResourceConfig` method to the provider-server
interface that `terraform-plugin-framework@v1.16.1` (what `go mod tidy`
landed on initially) doesn't implement, so the build failed with a missing-
method error until `terraform-plugin-framework` was bumped to a version
that does implement it. Worth remembering for any future Go-version-driven
dependency bump: `go mod tidy` doesn't guarantee every transitively-pulled
package stays mutually compatible, verify with a real `go build` after.

Verified locally (go1.25.12 downloaded to `/tmp/goroot125` for this
session): `go build ./...`, `go vet ./...`, `gofmt -l .`,
`go test ./...` all clean, `tfplugindocs generate` idempotent (one cosmetic
formatting diff from the tool version bump itself, no content change).
Confirmed green in CI too — `build`, `acceptance` (real sandbox), and
`docs` all passed on the first push at this Go version (run 31279694650).

---

## Definition of done for this plan

- All of Steps 1–8 marked `done` above.
- `go build ./...`, `go vet ./...`, `go test ./...` pass with no `TF_ACC` set.
- `TF_ACC=1 go test ./... -run TestAcc` passes against the sandbox.
- `tfplugindocs generate` produces no diff.
- `v0.1.0` is live on the Terraform Registry.
- Every commit made for this plan carries `Refs:` trailers per
  `docs/skills/commit-with-kms-attribute.md`.
