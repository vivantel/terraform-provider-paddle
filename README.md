# terraform-provider-paddle

Unofficial Terraform provider for [Paddle Billing](https://developer.paddle.com/api-reference/overview). Manages `paddle_product`, `paddle_price`, `paddle_discount`, `paddle_discount_group`, and `paddle_notification_setting` (plus matching data sources), and looks up checkout domains via `paddle_checkout_domain` (data source only — see below), by calling Paddle's public REST API directly — no third-party service in the request path.

Not affiliated with or endorsed by Paddle.

## Status

Pre-1.0 (`v0.2.x`), but every resource and data source is verified end-to-end against a real Paddle sandbox account, not just built and unit-tested — CI's `acceptance` job runs the full create/update/import/destroy lifecycle for all five resources plus their data sources on every push (`.github/workflows/ci.yaml`); `paddle_checkout_domain` (data-source-only, see below) is checked against whatever real domain already exists in the sandbox account, since there's no API to create a fixture with. Schema fields were taken from Paddle's published API reference (`/products`, `/prices`, `/discounts`, `/discount-groups`, `/notification-settings`, `/checkout-domains`), not guessed. Product/Price/Discount/Discount Group archive on destroy (Paddle has no hard delete for these); Notification Setting is deleted for real — see `docs/plans/paddle-provider-v2.md` for why.

## Usage

```hcl
terraform {
  # >= 1.14.0 is required by this provider's actions (paddle_adjustment,
  # paddle_subscription_cancel/pause/resume/charge).
  required_version = ">= 1.14.0"

  required_providers {
    paddle = {
      source  = "vivantel/paddle"
      version = "~> 0.3"
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

This provider exposes five [Terraform actions](https://developer.hashicorp.com/terraform/language/actions) — imperative, one-time operations, not resources with a lifecycle Terraform reconciles on a later plan:

- `paddle_adjustment` — creates a refund, credit, or chargeback-related adjustment against a transaction.
- `paddle_subscription_cancel` / `paddle_subscription_pause` / `paddle_subscription_resume` / `paddle_subscription_charge` — subscription lifecycle operations. There's no `paddle_subscription` resource in this provider (subscriptions are checkout-created, not declared upfront — see `docs/decisions/0010-v3-scope-lifecycle-actions.md`), so each of these takes a plain `subscription_id` string rather than a resource reference.

**⚠️ These move real money or change a real customer's live billing state.** Two things make them categorically higher-stakes than every resource this provider manages:

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

**Testing note**: this provider's own acceptance tests for the subscription actions (`cancel`/`pause`/`resume`/`charge`) can't self-provision a subscription fixture — Paddle subscriptions can only be created via a real checkout with a test card, no pure-API path exists, even in sandbox. Provisioning one via a real sandbox checkout is a manual, one-time step, same as `paddle_checkout_domain`'s dashboard-approval precondition above:

1. Create a recurring (has a billing cycle) catalog price in the sandbox, if you don't have one already.
2. Complete a real checkout against it with a [test card](https://developer.paddle.com/concepts/payment-methods/credit-debit-card#test-cards) (e.g. `4242 4242 4242 4242`, any future expiry/CVC) — any customer email works, sandbox only, no real charge.
3. Note the resulting subscription's ID (`sub_...`, visible in the dashboard or via `GET /subscriptions`) and set it as `PADDLE_TEST_SUBSCRIPTION_ID`, either as a local env var or as a repo secret for CI (`.github/workflows/ci.yaml` already passes it through).

With that set, `internal/provider/action_paddle_subscription_acc_test.go`'s tests target that exact, recognizable subscription rather than searching the account for "whatever's in the right status" — set once, reused indefinitely: nothing in this repo sweeps subscriptions (there's no `paddle_subscription` resource to sweep), and the pause/resume/charge tests always leave it back in `active`, so it never needs recreating. Without `PADDLE_TEST_SUBSCRIPTION_ID` set, these tests fall back to searching the account for any subscription in the right status, and skip cleanly if none exists.

`TestAccPaddleAdjustment_basic` self-provisions its own fixture transaction, but needs one manual, one-time sandbox account setting first, **found by running this test against the real sandbox** (2026-08-10, `feat/v3-lifecycle-actions` PR CI): Paddle rejects *any* transaction creation via the API — even a fully manual, non-checkout one — until the account has a default payment link set (Paddle dashboard → **Checkout** → your default pay link). Without it, this test skips cleanly with a message pointing here rather than failing.

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
