---
title: Example version constraints must track the latest released minor
status: active
date: 2026-08-09
tags: [paddle, provider, docs, release]
---

## Guardrail

Every `required_providers` block shown in this repo's own docs/examples
(`examples/provider/provider.tf`, `README.md`, `examples/full-stack/main.tf`,
and any future example that includes one) must use `~> <current latest
minor>`, updated as part of tagging any new MINOR release. A stale
constraint here is a silent trap: `~> 0.1` blocks `0.2.0`/`0.3.0`+ entirely —
anyone who copy-pastes straight from the README or the Registry's own
generated docs gets pinned to a version several releases behind with no
error, just a provider that's missing everything shipped since.

## Why

Found 2026-08-09: `~> 0.1` was still present in three places
(`examples/provider/provider.tf` — the source `docs/index.md`'s example is
generated from — plus its two hand-copied duplicates in `README.md` and
`examples/full-stack/main.tf`) after both `v0.2.0` and `v0.3.0` had already
shipped. Nothing catches this automatically: `tfplugindocs generate`
regenerates `docs/index.md` from the example file faithfully, stale
constraint and all — a stale version number isn't a schema drift, so the
existing drift-check CI job (see
`docs/guardrails/docs-must-be-regenerated-before-merge.md`) has no way to
catch it.

## Applies to

- `examples/provider/provider.tf` (the source of `docs/index.md`'s example).
- `README.md`'s Usage section.
- `examples/full-stack/main.tf` (or any future full-stack/guide example).
- The same three locations' `required_version` constraint, not just
  `required_providers`'s — added 2026-08-10
  ([[0010-v3-scope-lifecycle-actions]]) alongside this provider's first
  actions, which need Terraform `>= 1.14.0`. Lower-risk than the
  `required_providers` case (a version floor only ever needs raising, never
  a moving target tracking "latest minor"), but still worth a grep pass if
  a future change raises the actual minimum Terraform version this
  provider needs.

## How to apply

Before tagging a new MINOR release (a PATCH release doesn't need this — the
constraint should already cover it), grep for the old minor's constraint
string across the three locations above and bump it, in the same PR/commit
that updates `CHANGELOG.md` for the release.

## Related

- `docs/guardrails/docs-must-be-regenerated-before-merge.md`
- `docs/plans/paddle-provider-v2.md`
