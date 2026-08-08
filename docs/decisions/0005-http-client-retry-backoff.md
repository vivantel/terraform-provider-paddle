---
title: HTTP client retries 429/5xx with bounded exponential backoff
status: accepted
date: 2026-08-08
tags: [paddle, provider, client, resilience]
---

## Decision

`internal/client.Client.do` (the single chokepoint all API calls already go
through — see [[0001-existing-provider-baseline]]) gets bounded exponential
backoff retry logic for:

- `429 Too Many Requests` (Paddle's rate limiting)
- `5xx` responses (transient upstream failures)

Non-retryable errors (4xx other than 429, network errors after retries
exhausted) are returned as-is via the existing `APIError` type — no behavior
change there.

## Why

Paddle's API is rate-limited, and any HTTP API can return transient 5xxs.
Without retry logic, a `terraform apply` managing more than a handful of
catalog objects (plausible once Discounts are added — see
[[0001-catalog-only-scope-v1]]) can fail spuriously under normal load, which
is a poor experience for a provider marketed as "mature." Terraform itself
does not retry provider-level HTTP failures for you — that's the provider's
job.

The alternative (leave the client as-is, let Terraform's own error surfacing
handle it) was rejected: Terraform's response to a provider error is to fail
the apply, not retry it, so this would leave users to manually re-run
`terraform apply` on transient failures.

## Consequences

- `internal/client/client.go`'s `do` method needs a retry loop with capped
  attempts and exponential backoff (with jitter, to avoid thundering-herd
  retries across concurrent resource operations in the same apply).
- Respect a `Retry-After` header on 429s if Paddle sends one, falling back to
  the exponential schedule if it doesn't.
- Retry attempts must respect the request's `context.Context` — a cancelled
  or timed-out context aborts the retry loop immediately rather than sleeping
  through it.
- This is the *only* place retry logic should live — see
  `docs/guardrails/client-calls-must-use-retry-wrapper.md`. Resources must
  never bypass `client.Client` for raw HTTP calls.

## Addendum: overall call budget (2026-08-08, `/code-review high`)

The original implementation bounded each individual HTTP request (30s
client timeout) but not the retry loop as a whole — a persistently *slow*
(not fast-failing) backend could still block a single `do()` call for
minutes, up to `retryMaxAttempts * defaultTimeout` plus backoff, all
sequential. Fixed by wrapping the whole `do()` call in
`context.WithTimeout(ctx, retryOverallBudget)` (60s) — `context.WithTimeout`
always takes the earlier of two deadlines, so this only ever tightens an
already-bounded caller context, never loosens an unbounded one beyond this
budget. See `internal/client/client.go`'s `retryOverallBudget` and
`TestDo_OverallBudgetBoundsSlowPersistentFailures`.

Also added in the same pass: `client.IsNotFound(err)`, a shared helper for
the 404-detection every resource's `Read()` and `Delete()` now use, backing
the same tolerance this decision's retry logic doesn't itself cover (a 404
isn't retried — it's not in the retryable-status set — but callers need a
consistent way to recognize it).

## Related

- [[0001-existing-provider-baseline]]
- `docs/guardrails/client-calls-must-use-retry-wrapper.md`
- `docs/plans/paddle-provider-v1.md`
