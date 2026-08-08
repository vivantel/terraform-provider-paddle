# terraform-provider-paddle

Unofficial Terraform provider for [Paddle Billing](https://developer.paddle.com/api-reference/overview). Manages `paddle_product`, `paddle_price`, and `paddle_discount` (plus matching data sources) by calling Paddle's public REST API directly — no third-party service in the request path.

Not affiliated with or endorsed by Paddle.

## Status

Pre-1.0 (`v0.1.x`), but every resource and data source is verified end-to-end against a real Paddle sandbox account, not just built and unit-tested — CI's `acceptance` job runs the full create/update/import/archive-on-destroy lifecycle for all three resources plus their data sources on every push (`.github/workflows/ci.yaml`). Schema fields were taken from Paddle's published API reference (`/products`, `/prices`, `/discounts`), not guessed.

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

Full schema reference and more examples: [`docs/`](docs/index.md), or on the [Terraform Registry](https://registry.terraform.io/providers/vivantel/paddle/latest) once published (see below).

## Development

Requires Go 1.22+.

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
go install github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.19.4  # @latest needs go >= 1.25
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

Steps 1-3 need your own registry account — not something that can be done from CI. See `docs/plans/paddle-provider-v1.md` (Step 0/Step 8) for the full history of what's been done and what's still open.
