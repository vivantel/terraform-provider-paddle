---
title: Unit test pure logic separately from live-sandbox acceptance tests
status: accepted
date: 2026-08-08
tags: [paddle, provider, testing]
---

## Decision

Alongside the live-sandbox acceptance tests from
[[0003-acceptance-tests-against-live-sandbox]], this provider also has plain
Go unit tests — no network, no `TF_ACC`, no sandbox credentials — for:

- The `client` package's struct JSON marshaling (`internal/client/client_test.go`):
  does an unset optional pointer field marshal as explicit `null`, does the
  `statusPatch` archive body really contain only `status`.
- The `provider` package's pure conversion functions
  (`internal/provider/*_resource_test.go`): `toAPIProduct`/`fromAPIProduct`,
  `toAPIPrice`/`fromAPIPrice` — given a model, does the API struct come out
  right, including edge cases like partially-unknown nested objects.

These run in the same `go test ./...` CI step as everything else, with no
gating — unlike acceptance tests, they need no credentials and cost nothing
to run on every push.

## Why

[[0003-acceptance-tests-against-live-sandbox]] settled *acceptance* testing
(full CRUD against real Paddle) but never separately addressed unit testing
of pure logic — that was a gap in the original roadmap interview, not a
deliberate exclusion.

The gap was concrete, not theoretical: a code review of the initial scaffold
found six bugs, and every one of them lived in pure marshaling/conversion
logic — an `omitempty` tag that silently dropped a field instead of nulling
it, an archive body that reused a struct with required fields, a nested
object's unknown-state check that only looked at one of two fields. None of
these needed a live API call to catch; a `json.Marshal` call or a call to
`toAPIPrice` with a fabricated model catches them immediately, in
milliseconds, in every CI run — not just the runs that happen to exercise
that code path against a real sandbox.

Acceptance tests remain necessary for what only a real API can tell you
(does Paddle actually accept this payload, does archiving actually change
status) — this decision doesn't replace them, it fills the layer below them.

## Consequences

- New pure functions added to `internal/client` or `internal/provider`
  should get unit tests in the same PR, per
  `docs/guardrails/pure-logic-needs-unit-tests.md` — followed since for
  `paddle_discount`'s `toAPIDiscount`/`fromAPIDiscount`
  (`discount_resource_test.go`), and again for `client.IsNotFound`,
  `configureClient`, and the retry/budget logic added in the later
  `/code-review high` pass.
- `ci.yaml`'s build job now runs `go test ./...` — previously it only ran
  `go build`/`go vet`/`gofmt -l`, so tests could be added without ever
  running in CI. This runs unconditionally (no `TF_ACC` needed); acceptance
  tests were added later per [[0003-acceptance-tests-against-live-sandbox]]
  and skip themselves via `testAccPreCheck` when `PADDLE_API_KEY` is unset,
  exactly as anticipated — this step needed no modification once they
  existed.

## Related

- [[0003-acceptance-tests-against-live-sandbox]]
- `docs/guardrails/pure-logic-needs-unit-tests.md`
- `docs/plans/paddle-provider-v1.md`
