---
title: A resource's configurable timeout must never exceed a hard ceiling, no matter what's configured
status: active
date: 2026-08-12
tags: [paddle, provider, timeouts, v5]
---

## Guardrail

Every resource's `timeouts{}` support must cap the *effective* timeout at
30 minutes, regardless of what value a user configures. If a user sets
`timeouts { delete = "24h" }`, the resource must still only ever wait up
to 30 minutes before giving up — the configured value is honored up to
that ceiling, not beyond it. Enforce this where the configured
`time.Duration` is read and turned into a `context.WithTimeout`, not just
documented in the schema description.

This applies to every operation (`create`/`read`/`update`/`delete`) on
every resource that implements `timeouts{}` support, present and future
— any new resource added after this guardrail's date must respect the
same ceiling, not just the five resources this was first implemented
against.

## Why

Derived from [[0013-configurable-timeouts-architecture]]'s "caller-
supplied deadline wins entirely" precedence decision: once a configured
timeout can genuinely extend past the client's previous fixed 60s budget
with nothing else bounding it, a typo'd or misunderstood config (a
missing unit, an extra zero, copy-pasting a value meant for a different
tool) could hang a `terraform apply` indefinitely on a call that was
never going to succeed. 30 minutes gives real, meaningful headroom over
today's fixed 60s for legitimate slow-operation cases while still
bounding the worst case to something a CI job's own timeout (or a
human's patience) will catch before it becomes "the apply has been stuck
for hours and nobody noticed."

## Applies to

- Every resource's `Create`/`Read`/`Update`/`Delete` that reads a
  `timeouts{}`-derived duration and turns it into a `context.WithTimeout`.
- `internal/provider/*_resource.go` — wherever the shared "resolve
  configured timeout, apply the ceiling, derive a context" logic lives
  (a shared helper, not five independent implementations, is the
  intended shape — see `docs/plans/paddle-provider-v5.md`'s own step for
  this).

## Related

- [[0013-configurable-timeouts-architecture]]
- [[0012-v5-scope-pii-data-sources-timeouts-testing]]
- `docs/plans/paddle-provider-v5.md`
