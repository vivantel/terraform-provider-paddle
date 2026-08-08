---
title: Provider baseline as of 2026-08-08, before v1 maturity work
status: current
date: 2026-08-08
tags: [paddle, provider, baseline]
---

## Fact

As of 2026-08-08, before the v1 maturity work in
`docs/plans/paddle-provider-v1.md` began:

- Module path: `github.com/vivantel/terraform-provider-paddle` (moved from
  a personal GitHub account to the `vivantel` org this same day — see git
  history / commit that updated `go.mod`, imports, `main.go`,
  README, and LICENSE).
- Registry address in `main.go`: `registry.terraform.io/vivantel/paddle`.
- Go version: 1.22. Built on the HashiCorp Terraform Plugin Framework
  (`terraform-plugin-framework`), not the legacy SDKv2.
- Resources implemented: `paddle_product`, `paddle_price`
  (`internal/provider/product_resource.go`,
  `internal/provider/price_resource.go`) — CRUD only, no `ImportState`, no
  matching data sources yet.
- `internal/client/client.go`: a minimal hand-rolled HTTP client. Bearer auth
  via `Authorization` header, `SandboxBaseURL`/`ProductionBaseURL` constants,
  a generic `APIError{StatusCode, Body}` type that surfaces Paddle's error
  envelope verbatim. Only Products and Prices endpoints implemented. No
  retry/backoff logic (see [[0005-http-client-retry-backoff]] which changes
  this).
- README documents `dev_overrides`-based local iteration only; no
  release/CI/registry-publishing setup exists yet (see
  [[0004-release-via-goreleaser-github-actions]] which adds this).
- No `docs/` knowledge-artifact structure existed in this repo before this
  session — the `docs/{facts,decisions,guardrails,skills,plans}/` layout
  used here was introduced by this roadmap session, following the
  convention already assumed by the `kms:attribute` and `kms:changelog`
  skills available in this environment.

## Related

- [[0001-catalog-only-scope-v1]]
- `docs/plans/paddle-provider-v1.md`
