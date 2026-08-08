# terraform-provider-paddle

Unofficial Terraform provider for [Paddle Billing](https://developer.paddle.com/api-reference/overview). Manages `paddle_product` and `paddle_price` by calling Paddle's public REST API directly — no third-party service in the request path.

Not affiliated with or endorsed by Paddle.

## Status

Freshly scaffolded, **not yet tested against a real Paddle account** (no sandbox credentials were available at build time — verify carefully before relying on it, especially the archive-on-destroy behavior). Schema fields were taken from Paddle's published API reference (`/products`, `/prices`), not guessed.

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
```

## Development

Requires Go 1.22+.

```bash
go build ./...
go vet ./...
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

Not done yet — needs, in order:

1. A GPG key, with the public key uploaded to your [Terraform Registry](https://registry.terraform.io) account (Settings → Signing Keys).
2. Two repo secrets: `GPG_PRIVATE_KEY` (armored private key) and `PASSPHRASE`.
3. Push a `v*` tag — `.github/workflows/release.yaml` runs GoReleaser, which builds, signs, and creates a GitHub Release with the required registry manifest.
4. On registry.terraform.io, "Publish provider" → pick this repo. The registry reads releases directly from GitHub; no separate upload step.

Steps 1-2 and 4 need your own registry account — not something that can be done from CI.
