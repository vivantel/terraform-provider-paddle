---
title: Confirmed real API/module facts for v5 — notification replay endpoint and the terraform-plugin-framework-timeouts pattern
status: current
date: 2026-08-12
tags: [paddle, provider, actions, timeouts, v5]
---

## Fact

Confirmed against real sources, 2026-08-12, while scoping
[[0012-v5-scope-pii-data-sources-timeouts-testing]] and
[[0013-configurable-timeouts-architecture]]:

- **`POST /notifications/{notification_id}/replay`** exists on Paddle's
  real API (confirmed against `developer.paddle.com/api-reference/
  notifications/replay-notification`). "Attempts to resend a `delivered`
  or `failed` notification using its ID." Replaying creates a *new*
  notification entity linked to the same underlying event and returns
  the new notification's ID — it does not mutate or re-deliver the
  original notification record in place.
- **`github.com/hashicorp/terraform-plugin-framework-timeouts`** is
  HashiCorp's own official module for per-resource configurable
  timeouts, confirmed against `developer.hashicorp.com/terraform/plugin/
  framework/resources/timeouts`. For new (non-SDKv2-migrated) providers,
  the recommended schema shape is nested *attributes*, not a block:
  ```go
  "timeouts": timeouts.Attributes(ctx, timeouts.Opts{
      Create: true,
      Read:   true,
      Update: true,
      Delete: true,
  })
  ```
  A resource reads a configured value with a fallback default via e.g.
  `data.Timeouts.Delete(ctx, 60*time.Second)` — the module handles
  parsing/defaulting, this provider's own code is responsible for
  actually enforcing it (turning the resolved `time.Duration` into a
  `context.WithTimeout` passed down to client calls) and for the 30m
  ceiling ([[configurable-timeouts-need-a-hard-ceiling]]), neither of
  which the module provides itself.
- This provider currently (as of `v0.5.0`) has **zero** timeout
  configurability anywhere — confirmed by `grep`, not assumed: no
  `terraform-plugin-framework-timeouts` import anywhere in `go.mod` or
  `internal/`, no `timeouts` attribute on any resource schema. The only
  existing timeout is `internal/client/client.go`'s hardcoded
  `retryOverallBudget = 60 * time.Second`, applied uniformly to every
  client call with no override path (not per-resource, not via provider
  config, not via an environment variable).

## Related

- [[0012-v5-scope-pii-data-sources-timeouts-testing]]
- [[0013-configurable-timeouts-architecture]]
- `docs/plans/paddle-provider-v5.md`
