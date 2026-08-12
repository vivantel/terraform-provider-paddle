---
title: Mock-server unit tests supplement real-sandbox verification, they never replace it
status: active
date: 2026-08-12
tags: [paddle, provider, testing, v5]
---

## Guardrail

Any `httptest`-based mock-server unit test added to this provider
(covering resource CRUD logic, client retry/timeout behavior, or
anything else) is additive — a faster, cheaper local/CI signal
*underneath* real-sandbox verification, never a substitute for it. A
resource or feature is still not considered done until confirmed against
the real Paddle sandbox, exactly as
[[0003-acceptance-tests-against-live-sandbox]] already establishes. Do
not let a passing mock-based test suite be treated as "this is verified"
in a PR description, commit message, or plan `Status:` line — that
language is reserved for real sandbox confirmation.

The one exception this guardrail exists specifically to carve out:
behavior that genuinely cannot be exercised against the live sandbox at
all — e.g. proving a configured `timeouts{}` value actually fires,
since Paddle's real API can't be made to respond slowly on demand. For
that narrow class of behavior, a mock-server test *is* the verification,
not a supplement to one — but this should stay a rare, explicitly-argued
exception, not the default reach for anything inconvenient to test live.

## Why

Derived from [[0012-v5-scope-pii-data-sources-timeouts-testing]] (item
5)'s decision to invest in a general, reusable mock-server testing
pattern and retrofit all five existing resources with it. Introducing
this kind of infrastructure into a codebase whose whole verification
culture has been "real sandbox or it didn't happen"
([[0003-acceptance-tests-against-live-sandbox]],
`docs/skills/verify-before-claiming.md`) creates a real risk of
gradually treating fast, convenient mock coverage as sufficient on its
own — this guardrail exists to name that risk explicitly before it
happens, not after.

## Applies to

- Any new `httptest`-based test file, whether covering `internal/client`
  or `internal/provider/*_resource.go`/`*_data_source.go` CRUD logic.
- PR descriptions, commit messages, and `docs/plans/*.md` `Status:`
  lines — "unit-tested" (mock or pure) and "real-sandbox-verified" must
  stay distinguishable, never conflated.

## Related

- [[0003-acceptance-tests-against-live-sandbox]]
- [[0013-configurable-timeouts-architecture]]
- [[0012-v5-scope-pii-data-sources-timeouts-testing]]
- `docs/skills/verify-before-claiming.md`
