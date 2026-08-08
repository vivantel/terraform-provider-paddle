---
title: Client requests must log via tflog, never leak the API key
status: active
date: 2026-08-08
tags: [paddle, provider, observability]
---

## Guardrail

Every request `internal/client.Client.do()` sends must log at
`tflog.Debug` level: HTTP method, path, attempt number (for retries), and
response status code. The API key must never appear in a log line at any
level. Request/response bodies are not logged by default — only add body
logging behind an explicit, narrowly-scoped need, and redact anything that
looks like a credential or sensitive field first.

## Why

Derived from [[0009-tflog-observability-and-acceptance-test-sweepers]]:
`grep -rl tflog internal/` returned nothing before this decision — no way
to debug a failing `terraform apply` against this provider via
`TF_LOG=debug` at all. Every mainstream Terraform provider logs at this
level; its absence was a real usability gap, not a stylistic choice.

## Applies to

- `internal/client/client.go`'s `do()` method.
- Any future client method that bypasses `do()` (there shouldn't be any,
  per `docs/guardrails/client-calls-must-use-retry-wrapper.md` — this
  guardrail is what makes that one's logging coverage complete once both
  are followed).

## Related

- [[0009-tflog-observability-and-acceptance-test-sweepers]]
- `docs/guardrails/client-calls-must-use-retry-wrapper.md`
