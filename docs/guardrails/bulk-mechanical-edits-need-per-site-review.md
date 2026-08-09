---
title: Bulk mechanical edits (sed/regex sweeps) need per-site semantic review
status: active
date: 2026-08-09
tags: [paddle, provider, process, review]
---

## Guardrail

After any bulk mechanical edit across multiple call sites (a `sed`/regex
sweep, a find-and-replace, a codemod) — even one that's textually
unambiguous — read every individual site it touched and confirm the change
is semantically correct there, not just that the result compiles and tests
pass. Do this before committing, not as a follow-up.

## Why

`internal/client/client.go`'s `FriendlyErrorMessage(err)` helper was added
2026-08-09 to make Paddle API errors readable in Terraform diagnostics.
Wiring it in was a mechanical sweep: every
`resp.Diagnostics.AddError("Error ...", err.Error())` across all six
entities' resources/data sources became
`resp.Diagnostics.AddError("Error ...", client.FriendlyErrorMessage(err))`
— ~34 call sites, one `sed` pass.

8 of those sites were wrong: `"Error decoding Paddle X response"` in
`price`/`product`'s `Create`/`Read`/`Update` wraps
`fromAPIPrice`/`fromAPIProduct`'s local `customDataFromAPI` re-marshal
error, never an error the API client returned. Applying an "is this a
Paddle API error?" parser to an error that structurally can never be one
is wrong, even though `FriendlyErrorMessage` falls back to `err.Error()`
for any non-`*APIError` input — so the *behavior* was identical either
way. `go build`, `go vet`, `golangci-lint`, and the full acceptance suite
all passed with the bug still in the diff. Nothing automated could have
caught it: it's not a compile error, not a lint finding, not a test
failure — it's purely "does this line make semantic sense here," which
only a human read of each site answered. Found on a manual pre-tag review
pass, one release cycle after the sweep first shipped.

## Applies to

Any future sweep of this shape — a helper added or changed, then wired
into many call sites by pattern-matching rather than one at a time.

## Related

- `docs/skills/verify-before-claiming.md`
