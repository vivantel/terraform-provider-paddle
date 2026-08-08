---
title: Provider auth via schema attributes with env var fallback
status: accepted
date: 2026-08-08
tags: [paddle, provider, auth, schema]
---

## Decision

The provider configuration block gets two optional attributes:

- `api_key` (string, optional, sensitive)
- `environment` (string, optional — `"sandbox"` or `"production"`, default
  `"production"`)

If `api_key` is unset in the provider block, the provider falls back to the
`PADDLE_API_KEY` environment variable. If `environment` is unset, it falls
back to `PADDLE_ENVIRONMENT`, defaulting to `"production"` if neither is set.

```hcl
provider "paddle" {
  api_key     = var.paddle_api_key   # or leave unset and use PADDLE_API_KEY
  environment = "sandbox"            # or leave unset and use PADDLE_ENVIRONMENT
}
```

## Why

This is the standard Terraform provider convention (AWS, GitHub, and most
registry providers follow it): explicit provider-block config takes
precedence, environment variables are the fallback. It supports two common
workflows without extra code:

- **CI/CD**: inject credentials via env vars, keep `.tf` files credential-free.
- **Multi-environment configs**: multiple `provider "paddle"` blocks with
  aliases, each pointed at a different sandbox/production key explicitly in
  the block, for configs that manage both environments side by side.

Alternatives considered:
- *Env var only* — simpler schema, but breaks the common pattern of
  provisioning multiple Paddle accounts/environments from one root module via
  provider aliases.
- *Provider block only, no fallback* — most explicit, but forces every CI
  pipeline to template the key into `.tf`/`.tfvars` rather than using
  Terraform's normal env var injection path.

## Consequences

- `internal/provider/provider.go`'s schema and `Configure` method need the
  optional attributes plus env var fallback logic (`os.Getenv` reads, in that
  precedence order).
- Because this shapes the provider's public configuration contract, renaming
  or removing these attributes after the v0.1.0 release is a breaking change
  for every user's config — treat the attribute names as fixed once released.
- `api_key` must be marked `Sensitive: true` in the schema so it never shows
  up in plan/apply output or logs.

## Related

- [[0001-existing-provider-baseline]]
- `docs/plans/paddle-provider-v1.md`
