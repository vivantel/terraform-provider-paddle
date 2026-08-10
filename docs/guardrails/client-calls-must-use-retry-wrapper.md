---
title: All Paddle API calls must go through the retry-wrapped client
status: active
date: 2026-08-08
tags: [paddle, provider, client, resilience]
---

## Guardrail

Resource and data source code must never construct its own `http.Client` or
call the Paddle API directly. Every API call must go through
`internal/client.Client`, so that the retry/backoff behavior from
[[0005-http-client-retry-backoff]] applies uniformly. Code review should
reject any PR that adds a raw `http.Get`/`http.Post`/`http.NewRequest` call
inside `internal/provider/`.

## Why

Derived from [[0005-http-client-retry-backoff]]: retry/backoff logic only
protects callers that actually go through it. A single bypass reintroduces
the exact spurious-failure-under-rate-limiting problem that decision was
meant to solve, and does so silently — the bypassing resource just looks
flakier than its siblings for no obvious reason.

## Applies to

- `internal/provider/*_resource.go`, `internal/provider/*_data_source.go`
- `internal/provider/actions/*.go` — actions must still go through
  `internal/client.Client`, same as resources/data sources; see the
  Exception below for the one way their usage differs.
- Any future non-catalog resource added after v1, since the same client
  chokepoint should be extended rather than duplicated.

## Exception

Actions wrapping a Paddle endpoint that isn't safe to blindly repeat
(refunds/credits, subscription cancel/pause/resume/charge) must **not**
use this wrapper's automatic retry-on-5xx behavior unmodified — see
`docs/guardrails/money-moving-actions-no-blanket-retry.md`. They still go
through `internal/client.Client` (never a raw `http.Client`), just not
through its default retry path.

## Related

- [[0005-http-client-retry-backoff]]
- `docs/guardrails/money-moving-actions-no-blanket-retry.md`
- `docs/plans/paddle-provider-v1.md`
