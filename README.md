# terraform-provider-paddle

Unofficial Terraform provider for [Paddle Billing](https://developer.paddle.com/api-reference/overview). Manages `paddle_product`, `paddle_price`, `paddle_discount`, `paddle_discount_group`, and `paddle_notification_setting` (plus matching data sources), and looks up checkout domains via `paddle_checkout_domain` (data source only — see below), by calling Paddle's public REST API directly — no third-party service in the request path.

Not affiliated with or endorsed by Paddle.

## Status

Pre-1.0 (`v0.2.x`), but every resource and data source is verified end-to-end against a real Paddle sandbox account, not just built and unit-tested — CI's `acceptance` job runs the full create/update/import/destroy lifecycle for all five resources plus their data sources on every push (`.github/workflows/ci.yaml`); `paddle_checkout_domain` (data-source-only, see below) is checked against whatever real domain already exists in the sandbox account, since there's no API to create a fixture with. Schema fields were taken from Paddle's published API reference (`/products`, `/prices`, `/discounts`, `/discount-groups`, `/notification-settings`, `/checkout-domains`), not guessed. Product/Price/Discount/Discount Group archive on destroy (Paddle has no hard delete for these); Notification Setting is deleted for real — see `docs/plans/paddle-provider-v2.md` for why.

## Usage

```hcl
terraform {
  required_providers {
    paddle = {
      source  = "vivantel/paddle"
      version = "~> 0.1"
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
