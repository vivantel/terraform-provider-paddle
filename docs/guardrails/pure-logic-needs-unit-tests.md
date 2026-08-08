---
title: Pure conversion/marshaling logic needs unit tests, not just acceptance tests
status: active
date: 2026-08-08
tags: [paddle, provider, testing]
---

## Guardrail

Any new pure function in `internal/client` (struct marshaling) or
`internal/provider` (`toAPI*`/`fromAPI*` conversions) must ship with table-
driven unit tests in the same PR — particularly nil/null/unknown edge cases
for optional and nested fields. This is required regardless of whether the
resource also gets acceptance test coverage per
`docs/guardrails/acceptance-tests-require-tf-acc-gate.md`; the two are not
substitutes for each other.

## Why

Derived from [[0006-unit-tests-for-pure-logic]]: every bug the initial
code review found was in this exact category of code, and every one was
catchable in milliseconds with no network call. Relying on acceptance tests
alone to catch this class of bug means waiting for a live-sandbox run (slow,
needs credentials, only runs when `TF_ACC=1`) to catch something a plain
`go test` would have caught on every push.

## Applies to

- `internal/client/*.go` struct definitions and their `_test.go` files.
- `internal/provider/*_resource.go` conversion functions and their
  `_test.go` files.

## Related

- [[0006-unit-tests-for-pure-logic]]
- `docs/guardrails/acceptance-tests-require-tf-acc-gate.md`
