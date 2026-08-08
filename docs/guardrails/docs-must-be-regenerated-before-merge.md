---
title: Registry docs must be regenerated via tfplugindocs before merge
status: active
date: 2026-08-08
tags: [paddle, provider, docs, ci]
---

## Guardrail

Any PR that changes a resource/data-source schema (adds, removes, or
redescribes an attribute; adds a new resource or data source) must include a
regenerated `docs/index.md`, `docs/resources/*.md`, `docs/data-sources/*.md`
via `tfplugindocs generate`, committed in the same PR. CI must run
`tfplugindocs generate` and fail the build if it produces any diff against
what's committed — docs are never hand-edited to "look right" independent of
the schema.

## Why

Derived from [[0003-docs-via-tfplugindocs]]. Registry-facing docs that drift
from the actual schema are worse than no docs — they actively mislead users
about what attributes exist, are required, or are computed. A CI drift check
is the only way to guarantee this doesn't happen silently over time as
schemas evolve.

## Applies to

- Any PR touching `internal/provider/*_resource.go` or
  `internal/provider/*_data_source.go` schema definitions.
- The CI workflow file (see `docs/plans/paddle-provider-v1.md`) that adds the
  `tfplugindocs generate && git diff --exit-code` check.

## Related

- [[0003-docs-via-tfplugindocs]]
- `docs/plans/paddle-provider-v1.md`
