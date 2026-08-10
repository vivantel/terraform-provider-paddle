---
title: terraform-plugin-framework v1.19.0 already satisfies the Actions version floor
status: current
date: 2026-08-10
tags: [paddle, provider, actions, dependencies]
---

## Fact

`go.mod` already pins `github.com/hashicorp/terraform-plugin-framework
v1.19.0` — well past the `v1.15` minimum required for the Plugin
Framework's Actions support (which itself requires Terraform `≥1.14`).
Confirmed 2026-08-10 while scoping [[0010-v3-scope-lifecycle-actions]]: no
dependency bump is needed to add actions to this provider.

What *is* missing: no `required_version` constraint exists anywhere in this
repo today — checked every `.tf` example/doc and `.github/workflows/*.yaml`
(2026-08-10), `hashicorp/setup-terraform@v4` in CI is unpinned (pulls
latest). Adding actions means declaring `required_version = ">= 1.14.0"`
for the first time, not bumping an existing one — a documentation/schema
addition, not a risky dependency change.

## Related

- [[0010-v3-scope-lifecycle-actions]]
- `docs/guardrails/example-version-constraints-track-latest-minor.md`
