---
title: Add tflog debug logging and acceptance test sweepers
status: accepted
date: 2026-08-08
tags: [paddle, provider, observability, testing]
---

## Decision

Two provider-engineering gaps, unrelated to each other except that both are
"expected of a mature provider" items rather than new resource coverage:

1. **`tflog` debug logging** in `internal/client/client.go`'s `do()` and
   in each resource's CRUD methods — request method/path, retry attempts,
   and response status at `tflog.Debug` level, so `TF_LOG=debug
   terraform apply` shows what this provider actually sent to and received
   from Paddle. Never log the API key or full request/response bodies
   unredacted — see Consequences.
2. **Acceptance test sweepers** (`terraform-plugin-testing`'s sweeper
   support) for `paddle_product`/`paddle_price`/`paddle_discount` (and
   whatever v2 adds), so a killed/cancelled/timed-out acceptance test run
   doesn't leave orphaned sandbox objects behind indefinitely.

## Why

Both are things a "mature" provider is expected to have that this one
currently has zero of — confirmed by grep, not assumed:
`grep -rl tflog internal/` returns nothing at all, and there's no
`TestMain`/sweeper registration anywhere in the acceptance test files.

**Logging**: right now, debugging a failed `terraform apply` against this
provider means either reading Go source or re-running with a debugger —
there's no `TF_LOG=debug` output showing what was actually sent. Every
mainstream Terraform provider (AWS, GitHub, etc.) logs at this level via
`tflog`; its absence here is a real usability gap for anyone hitting an
issue this provider's own error messages don't fully explain.

**Sweepers**: the acceptance suite creates real objects in the sandbox
account on every CI run (`docs/decisions/0003-acceptance-tests-against-live-sandbox.md`).
`CheckDestroy` cleans up after a *successful* test run, but a run that's
cancelled mid-test (CI timeout, a `git push --force` interrupting an
in-flight workflow, a `Ctrl-C` locally) leaves whatever was created
orphaned in the sandbox with no cleanup path at all. This will
accumulate silently over time as the acceptance suite grows with v2's new
resources.

## Consequences

- Logging must not leak the API key: `client.Client.APIKey` never appears
  in a log line, even at debug level. Response bodies may contain
  `custom_data` or other fields a user considers sensitive — log
  status/method/path/attempt-count, not full bodies, unless a specific
  debugging need justifies it later.
- Sweepers need a naming convention to identify sweepable objects (e.g. a
  name/description prefix like `"acc-test-"` used consistently across all
  acceptance test configs) so a sweeper run doesn't have to guess which
  sandbox objects are test fixtures versus real data — retrofit existing
  acceptance test configs (`testAccProductConfig`, `testAccPriceConfig`,
  `testAccDiscountConfig`) to use a consistent prefix if they don't
  already.
- Neither of these blocks v2's new resources from shipping — they're
  cross-cutting engineering improvements, not scope. But new resources
  added under [[0007-v2-scope-discount-groups-and-notification-settings]]
  should get logging and sweeper support from the start once this lands,
  per the new guardrail.

## Related

- [[0007-v2-scope-discount-groups-and-notification-settings]]
- [[0003-acceptance-tests-against-live-sandbox]]
- `docs/guardrails/log-client-requests-with-tflog.md`
- `docs/plans/paddle-provider-v2.md`
