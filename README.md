# terraform-provider-paddle

[![CI](https://github.com/vivantel/terraform-provider-paddle/actions/workflows/ci.yaml/badge.svg)](https://github.com/vivantel/terraform-provider-paddle/actions/workflows/ci.yaml)
[![Release](https://img.shields.io/github/v/release/vivantel/terraform-provider-paddle)](https://github.com/vivantel/terraform-provider-paddle/releases)
[![Terraform Registry](https://img.shields.io/badge/terraform-registry-623CE4?logo=terraform&logoColor=white)](https://registry.terraform.io/providers/vivantel/paddle/latest)
[![License: MIT](https://img.shields.io/github/license/vivantel/terraform-provider-paddle)](LICENSE)

A Terraform provider for [Paddle Billing](https://developer.paddle.com/api-reference/overview): 5 resources, ~15 data sources, 6 lifecycle actions, an ephemeral resource for secrets, and resource identity/list-query support — every resource and data source verified end-to-end against a real Paddle sandbox account before each release, not just unit-tested (see [Status](#status) below).

Manages `paddle_product`, `paddle_price`, `paddle_discount`, `paddle_discount_group`, and `paddle_notification_setting` (plus matching data sources, each configurable via a `timeouts` attribute — see below); looks up checkout domains, subscriptions, transactions, customers, account events, and notification deliveries via `paddle_checkout_domain`/`paddle_subscription`/`paddle_transaction`/`paddle_customer`/`paddle_events`/`paddle_notification` (data sources only — see below), plus plural/list variants (`paddle_subscriptions`/`paddle_transactions`/`paddle_notifications`/`paddle_customers`) for "everything matching these filters" lookups; exposes six [Terraform actions](https://developer.hashicorp.com/terraform/language/actions) (`paddle_adjustment`, `paddle_subscription_cancel`/`pause`/`resume`/`charge`, `paddle_notification_replay`) for one-time lifecycle operations; fetches secret-shaped values via [ephemeral resources](https://developer.hashicorp.com/terraform/language/resources/ephemeral) (`paddle_notification_setting_secret` — see below) that are never written to state; and supports bulk-discovering existing infrastructure via [`terraform query`](https://developer.hashicorp.com/terraform/language/query) `list` blocks (`paddle_product` — see below) — by calling Paddle's public REST API directly, no third-party service in the request path.

Unofficial — not affiliated with or endorsed by Paddle.

## Status

Pre-1.0, but every resource and its matching data source is verified end-to-end against a real Paddle sandbox account, not just built and unit-tested — CI's `acceptance` job runs the full create/update/import/destroy lifecycle for all five resources plus their data sources on every push (`.github/workflows/ci.yaml`). `v0.4.0` (stable) added this provider's first actions, real-sandbox-verified the same way (`docs/plans/paddle-provider-v3.md`). `v0.5.0` (in progress, `docs/plans/paddle-provider-v4.md`) adds `paddle_subscription`/`paddle_transaction`/`paddle_customer`/`paddle_events`/`paddle_notification` — lookup data sources closing the discovery gap the actions otherwise have (each needs an opaque Paddle ID with no other way to find one from inside Terraform) — plus scheduled action regression coverage (`.github/workflows/e2e.yaml`); check that plan's per-step `Status:` line for exactly what's sandbox-verified versus implemented-and-unit-tested-only as of any given commit. `paddle_checkout_domain`/`paddle_subscription`/`paddle_transaction`/`paddle_customer`/`paddle_events`/`paddle_notification` (data-source-only, see below) are checked against whatever already exists in the sandbox account or a fixture created via direct API call, since none of these have a matching managed resource. Schema fields were taken from Paddle's published API reference, not guessed. Product/Price/Discount/Discount Group archive on destroy (Paddle has no hard delete for these); Notification Setting is deleted for real — see `docs/plans/paddle-provider-v2.md` for why.

## Usage

```hcl
terraform {
  # >= 1.14.0 is required by this provider's actions (paddle_adjustment,
  # paddle_subscription_cancel/pause/resume/charge).
  required_version = ">= 1.14.0"

  required_providers {
    paddle = {
      source  = "vivantel/paddle"
      version = "~> 0.4"
    }
  }
}

provider "paddle" {
  # api_key can also come from PADDLE_API_KEY
  # environment can also come from PADDLE_ENVIRONMENT — defaults to "sandbox"
  environment = "sandbox"
}

resource "paddle_product" "pro" {
  name         = "Pro"
  tax_category = "saas"
}

resource "paddle_price" "pro_monthly" {
  product_id  = paddle_product.pro.id
  description = "Pro tier, monthly"
  unit_price = {
    amount        = "2900"
    currency_code = "USD"
  }
  billing_cycle = {
    interval  = "month"
    frequency = 1
  }
}

resource "paddle_discount" "launch_promo" {
  type        = "percentage"
  amount      = "20"
  description = "Launch week promotion"
  code        = "LAUNCH20"
}

data "paddle_product" "existing" {
  id = "pro_..." # look up a product created outside this Terraform config
}
```

Full schema reference and more examples: [`docs/`](docs/index.md), or on the [Terraform Registry](https://registry.terraform.io/providers/vivantel/paddle/latest) once published (see below). For how these resources actually fit together — not just isolated single-resource snippets — see [`examples/full-stack/main.tf`](examples/full-stack/main.tf): a product with a recurring price, a discount group capping usage across multiple discounts, a notification setting wired to billing events, and a checkout domain lookup, all referencing each other.

### Checkout domains

`paddle_checkout_domain` is a **data source only** — there is no matching resource, and it can't be created via `terraform apply`. Paddle's API has no create or update operation for checkout domains at all (confirmed against the live API reference, not assumed): a domain must be added and approved through the Paddle dashboard first —

1. Paddle dashboard → **Checkout → Website approval → Domain approval**, add the domain, wait for Paddle to move it out of `pending_review`.
2. Once approved, look it up from Terraform by its ID (`chedom_...`, visible in the dashboard or via `GET /checkout-domains`):

   ```hcl
   data "paddle_checkout_domain" "example" {
     id = "chedom_..."
   }

   output "checkout_domain_status" {
     value = data.paddle_checkout_domain.example.status
   }
   ```

There's nothing to `terraform import` either — this data source is read-only lookup, not lifecycle management, for exactly this entity type.

### Actions — refunds, credits, and subscription lifecycle operations

Terraform requires `>= 1.14.0` for this section — see the `required_version` constraint above.

This provider exposes six [Terraform actions](https://developer.hashicorp.com/terraform/language/actions) — imperative, one-time operations, not resources with a lifecycle Terraform reconciles on a later plan:

- `paddle_adjustment` — creates a refund, credit, or chargeback-related adjustment against a transaction.
- `paddle_subscription_cancel` / `paddle_subscription_pause` / `paddle_subscription_resume` / `paddle_subscription_charge` — subscription lifecycle operations. There's no `paddle_subscription` resource in this provider (subscriptions are checkout-created, not declared upfront — see `docs/decisions/0010-v3-scope-lifecycle-actions.md`), so each of these takes a plain `subscription_id` string rather than a resource reference.
- `paddle_notification_replay` — resends a `delivered`/`failed` notification, creating a new notification entity linked to the same underlying event. Unlike the five actions above, this one isn't financial or irreversible (the worst case of an accidental duplicate invocation is one extra webhook delivery attempt, not a duplicate charge), so it has no search-before-invoke check and no special no-retry handling — see `docs/decisions/0012-v5-scope-pii-data-sources-timeouts-testing.md` item 4.

**⚠️ The five subscription/adjustment actions below move real money or change a real customer's live billing state** (`paddle_notification_replay` above does not — it's called out separately for exactly this reason). Two things make them categorically higher-stakes than every resource this provider manages:

1. **Paddle has no idempotency-key mechanism anywhere in its API** (confirmed directly against Paddle's own docs — no header, no dedup support of any kind). A network failure partway through one of these calls leaves the actual outcome genuinely ambiguous — the request may or may not have been processed. This provider does not blindly retry these calls (unlike every resource's `Create`/`Update`, which do retry on `429`/`5xx`) — an ambiguous failure surfaces as an explicit error telling you to check the Paddle dashboard or API directly for the real outcome **before manually re-running `terraform apply`**. Each action also checks for a matching prior invocation before acting (a status check for the subscription actions, a search for `paddle_adjustment`/`paddle_subscription_charge`) — but this is best-effort correlation, not a guarantee, especially for `paddle_subscription_charge` (see its own schema description for the known false-positive case: two deliberately separate charges for identical items are indistinguishable from a retry).
2. **`terraform apply -auto-approve` gives these zero human review before they execute.** This repo's own `.github/workflows/e2e.yaml` uses `-auto-approve` throughout, and it's a common pattern in CI generally — but that pattern is a poor fit for a config that includes any of these five actions. If you use one in an automated pipeline, review the plan (`terraform plan` shows exactly what an action will do — invoke arguments included — before `apply`) or gate it behind a real approval step, rather than auto-approving blind.

**Operational recommendation**: use a separate, more tightly-scoped Paddle API key for any configuration that includes these actions, distinct from the key used for catalog-management configs (`paddle_product`/`paddle_price`/etc.). Paddle API keys are account-wide by default; a key that can only be misused to create a duplicate `paddle_product` is a much smaller blast radius than one that can also issue refunds or cancel subscriptions.

```hcl
action "paddle_adjustment" "refund_order" {
  config {
    action         = "refund"
    transaction_id = "txn_..."
    reason         = "Customer requested refund"
  }
}

resource "terraform_data" "trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.paddle_adjustment.refund_order]
    }
  }
}
```

See `examples/lookup-then-act/main.tf` for a real, complete config putting this pattern together end-to-end: look up a subscription/transaction via a data source, feed the looked-up value straight into an action, with nothing hardcoded — the actual discovery-gap payoff these lookup data sources exist for.

**Testing note**: this provider's own acceptance tests for the subscription actions (`cancel`/`pause`/`resume`/`charge`) can't self-provision a subscription fixture — Paddle subscriptions can only be created via a real checkout with a test card, no pure-API path exists, even in sandbox. Provisioning one via a real sandbox checkout is a manual, one-time step, same as `paddle_checkout_domain`'s dashboard-approval precondition above:

1. Create a recurring (has a billing cycle) catalog price in the sandbox, if you don't have one already.
2. Complete a real checkout against it with a [test card](https://developer.paddle.com/concepts/payment-methods/credit-debit-card#test-cards) (e.g. `4242 4242 4242 4242`, any future expiry/CVC) — any customer email works, sandbox only, no real charge.
3. Note the resulting subscription's ID (`sub_...`, visible in the dashboard or via `GET /subscriptions`) and set it as `PADDLE_TEST_SUBSCRIPTION_ID`, either as a local env var or as a repo secret for CI (`.github/workflows/ci.yaml` already passes it through).

With that set, `internal/provider/action_paddle_subscription_acc_test.go`'s tests target that exact, recognizable subscription rather than searching the account for "whatever's in the right status" — set once, reused indefinitely: nothing in this repo sweeps subscriptions (there's no `paddle_subscription` resource to sweep), and the pause/resume/charge tests always leave it back in `active`, so it never needs recreating. Without `PADDLE_TEST_SUBSCRIPTION_ID` set, these tests fall back to searching the account for any subscription in the right status, and skip cleanly if none exists.

`TestAccPaddleSubscriptionCancel_alreadyCanceledShortCircuits` needs a **second, separate** subscription that's already canceled — it can't reuse the one above, which the pause/resume/charge tests need to stay `active`. Repeat steps 1-2 above to provision a second subscription, then cancel it once (dashboard, or `terraform apply` with `paddle_subscription_cancel` against it), and set its ID as `PADDLE_TEST_CANCELED_SUBSCRIPTION_ID` (`.github/workflows/ci.yaml` already passes this one through too). Once canceled, nothing in this test suite ever resumes it, so — like the subscription above — it's a one-time setup, reused indefinitely. Without it set, this test falls back to searching the account and skips cleanly if nothing canceled exists.

`TestAccPaddleAdjustment_basic` self-provisions its own fixture transaction, but needs one manual, one-time sandbox account setting first, **found by running this test against the real sandbox** (2026-08-10, `feat/v3-lifecycle-actions` PR CI): Paddle rejects *any* transaction creation via the API — even a fully manual, non-checkout one — until the account has a default payment link set (Paddle dashboard → **Checkout** → your default pay link). Without it, this test skips cleanly with a message pointing here rather than failing.

`TestAccPaddleNotificationDataSource_basic`/`_byFilter` can't self-provision a notification either — a notification is Paddle's own record of an actual delivery attempt, produced only once a real event fires against a configured `notification_setting` destination, not creatable via a direct API call. Without one, both tests skip cleanly ("no notifications exist in this sandbox account"), which is real coverage of the lenient/empty-account path but not of the actual filter/lookup logic. To get that coverage, set up one **permanent** (not test-fixture) `paddle_notification_setting` in the sandbox account, separate from anything `sweep.yaml` cleans up:

1. `type = "url"`, `destination` = any endpoint that reliably responds (e.g. a [webhook.site](https://webhook.site) URL) — Paddle records a delivery attempt either way, but a responding endpoint gets you `delivered` status instead of `failed`/`needs_retry`.
2. `subscribed_events` — pick an event type this repo's own CI already triggers on every push, e.g. `["product.created"]`, so notifications accumulate naturally from routine test runs instead of needing a dedicated trigger.
3. **Don't** put `"Acc Test"` anywhere in its `description` — `sweep.yaml` runs weekly and deletes anything matching that substring (`isAccTestName` in `internal/provider/sweep_test.go`); this one needs to survive sweeps indefinitely, same as the pinned subscriptions above.

Like the default-payment-link setting above, this is an ongoing sandbox account precondition, not a one-time fixture — set once, no code or secret changes needed.

### Configuring `timeouts`

`paddle_product`/`paddle_price`/`paddle_discount`/`paddle_discount_group`/`paddle_notification_setting` each accept an optional `timeouts` attribute (`create`/`read`/`update`/`delete`, each a duration string like `"30s"` or `"2h45m"`) — a nested *attribute*, assigned with `=`, not a block:

```hcl
resource "paddle_product" "example" {
  name         = "Pro"
  tax_category = "saas"

  timeouts = {
    create = "5m"
    delete = "5m"
  }
}
```

Every operation defaults to **60 seconds** if `timeouts` is omitted entirely — the same fixed budget this provider's HTTP client has always used, so nothing changes for a config that doesn't opt in. Configure one when you're actually hitting that default under real load — a catalog operation against a rate-limited or otherwise slow-responding Paddle account, the same real-world motivation that surfaced this provider's own sweeper needing more patience than a fixed 60s gave it (see `docs/decisions/0013-configurable-timeouts-architecture.md`). A caller-configured value fully overrides the default rather than tightening it — set `create = "5m"` and Terraform really will wait up to 5 minutes, not 60 seconds, before giving up.

**Every configured value is capped at a hard 30-minute ceiling, no matter what you set.** `timeouts = { delete = "24h" }` still only waits up to 30 minutes — a safety bound against a typo'd or misunderstood config (a missing unit, an extra zero) hanging a `terraform apply` indefinitely on a call that was never going to succeed (`docs/guardrails/configurable-timeouts-need-a-hard-ceiling.md`).

### Ephemeral resources — secrets without state

Terraform requires `>= 1.10.0` for [ephemeral resources](https://developer.hashicorp.com/terraform/language/resources/ephemeral) — comfortably below the `>= 1.14.0` this provider already requires for Actions above, so no separate version bump is needed to use one.

`paddle_notification_setting`'s `endpoint_secret_key` (the webhook signing secret Paddle uses to sign payloads) is `Sensitive`, but **`Sensitive` only redacts CLI/log output — it does not encrypt state.** That attribute still writes the real secret into your state file in plaintext, same as any other `Computed` attribute, and is now deprecated for exactly this reason:

```hcl
ephemeral "paddle_notification_setting_secret" "webhook" {
  notification_setting_id = paddle_notification_setting.example.id
}
```

`ephemeral.paddle_notification_setting_secret.webhook.endpoint_secret_key` is fetched fresh on every `plan`/`apply` this ephemeral resource appears in and is never written to state at all. Feed it into whatever actually consumes it via a write-only (`*_wo`) attribute, not a regular one — a write-only attribute doesn't persist to state either, regardless of where the value it receives came from. `paddle_notification_setting`'s own `endpoint_secret_key` attribute (and its data source's) still works unchanged — this is additive, not a breaking removal — but prefer the ephemeral resource in any new configuration.

### List resources — bulk-discovering existing infrastructure

Terraform requires `>= 1.14.0` for [`list` blocks](https://developer.hashicorp.com/terraform/language/query) and the `terraform query` command — the same floor this provider already requires for Actions above, so no separate version bump is needed.

`paddle_product` supports resource identity (Terraform `>= 1.12.0`) and a matching `list` block, so you can discover every product already in the account and generate `import` blocks (or full resource config, with `include_resource = true`) for it instead of hand-writing one `paddle_product` block per existing product:

```console
$ terraform query -query-file=list-products.tfquery.hcl
```

```hcl
# list-products.tfquery.hcl
list "paddle_product" "all" {
  provider = paddle

  config {}

  include_resource = true
}
```

Paddle's list-products endpoint takes no filters, so `config {}` is always empty here — every product in the account comes back. The other four resources (`paddle_price`/`paddle_discount`/`paddle_discount_group`/`paddle_notification_setting`) don't have identity or list support yet; `paddle_product` is a first slice, not the full set.

### `paddle_customer` — PII in your state file

`paddle_customer` looks up an existing customer by `id` or `email`. This is a different risk category from the Actions section above — data exposure, not a financial/irreversible action — so it gets its own warning here rather than being folded into that section.

**⚠️ Using `paddle_customer` writes real customer PII (email, name) into your Terraform state file, in plaintext by default.** Terraform persists every data source read into state exactly as durably as a resource's — the only difference is a data source's write happens on every `plan`/`refresh` this data source appears in, not once at apply-time like a resource's `Create`. "Read-only" does not make this concern go away. Treat any state file that uses `paddle_customer` as sensitive: use an encrypted, access-controlled remote backend, not local state or an unencrypted bucket — same recommendation the Actions section above gives for financial risk, applied here to data exposure instead.

There is no `paddle_address` data source or resource in this provider — `paddle_customer`'s email/name alone resolves the actual discovery gap (finding a subscription's or transaction's owning customer) without pulling in postal-address PII too.

### `paddle_events` — `data` can also carry PII

`paddle_events` lists Paddle account events, optionally filtered by `type`. Its `data` attribute is arbitrary JSON whose shape varies by event type — it isn't a dedicated PII field like `paddle_customer`'s `email`/`name`, but it can carry the same kind of PII depending on what happened: a `customer.created` or `customer.updated` event's `data` payload *is* a full customer record, while a `product.created` event's is not.

**⚠️ If you use `paddle_events` with a customer-related `type` filter (or no filter at all), its `data` field can write real customer PII into your Terraform state file, in plaintext by default, on every `plan`/`refresh` — same as `paddle_customer` above.** Because `data`'s shape isn't known in advance, there is no reliable way to filter or redact PII out of it before it reaches state; the mitigation is the same as `paddle_customer`'s: treat any state file that uses `paddle_events` as sensitive, with an encrypted, access-controlled remote backend rather than local state or an unencrypted bucket.

## Development

Requires Go 1.25+ (bumped from 1.22 on 2026-08-08 — see `docs/plans/paddle-provider-v1.md`'s resolved "Open question: bump the go version?").

```bash
go build ./...
go vet ./...
go test ./...              # unit tests — no credentials or network needed
```

Acceptance tests run the full CRUD lifecycle against a real Paddle sandbox account and are skipped unless `PADDLE_API_KEY` is set:

```bash
TF_ACC=1 PADDLE_API_KEY=<your sandbox key> go test ./... -run TestAcc -v
```

Docs (`docs/`) are generated from schema + `examples/`, not hand-written — regenerate after any schema change:

```bash
go install github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@latest
tfplugindocs generate
```

To iterate locally without a full release, add a `dev_overrides` block to `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "vivantel/paddle" = "/path/to/this/repo"  # directory containing the built binary
  }
  direct {}
}
```

## Publishing to the public Terraform Registry

1. ✅ A GPG key, with the public key uploaded to the [Terraform Registry](https://registry.terraform.io) account (Settings → Signing Keys) — **not** the HCP Terraform portal at `app.terraform.io`, that's a separate paid product with its own private-registry signing-key flow.
2. ✅ Two repo secrets: `GPG_PRIVATE_KEY` (armored private key) and `PASSPHRASE`.
3. On registry.terraform.io, "Publish provider" → pick this repo. Do this *after* step 1's key is uploaded — the flow expects a matching signing key to already exist.
4. Push a `v*` tag (e.g. `v0.1.0`) — `.github/workflows/release.yaml` runs GoReleaser, which builds, signs, and creates a GitHub Release with the required registry manifest. The Registry then ingests it via webhook (not instant — allow a few minutes).
5. Once the Registry has ingested it, `.github/workflows/registry-smoke-test.yaml` runs automatically (triggered by `release.yaml`'s completion) and confirms the *published* version actually installs and works: a real `terraform init` against `registry.terraform.io` (no `dev_overrides`), then a real `apply`/`destroy` of a `paddle_product` against the sandbox through the published binary — not just the in-process acceptance tests CI already runs on every push. Can also be run manually (`workflow_dispatch`, pass a version) to re-check an existing release.

Steps 1-3 need your own registry account — not something that can be done from CI. See `docs/plans/paddle-provider-v1.md` (Step 0/Step 8) for the full history of what's been done and what's still open.

## Ongoing health checks

Two scheduled workflows run independently of any release or push, both against the real sandbox:

- `.github/workflows/e2e.yaml` — daily (also `workflow_dispatch`), determines whichever version is *currently* latest on the Terraform Registry (not necessarily the one just released) and runs a real `terraform init`/`apply`/`plan`/`destroy` through the published binary against a small multi-resource config (`paddle_product` → `paddle_price`, `paddle_discount_group` → `paddle_discount`, `paddle_notification_setting`, plus a `paddle_product` data source lookup). Catches drift between releases — a Paddle API change, a Registry-side issue — that `registry-smoke-test.yaml` (which only runs once, right after a release) wouldn't. Doesn't cover `paddle_checkout_domain`: there's no real checkout domain ID reliably available in CI.
- `.github/workflows/sweep.yaml` — weekly (also `workflow_dispatch`), cleans up any sandbox object a crashed/timed-out CI run left behind.
