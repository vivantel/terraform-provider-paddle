---
title: A data source that exposes customer PII must document the state-file risk explicitly
status: active
date: 2026-08-11
tags: [paddle, provider, data-sources, pii, security]
---

## Guardrail

Any data source this provider ships whose schema includes customer PII
(email, name, postal address, tax ID, or equivalent) must carry an
explicit warning, in both its schema `MarkdownDescription` and this
repo's `README.md`, that Terraform persists data source reads into state
just like resource state — plaintext by default — so using that data
source puts that PII into the state file on every `plan`/`refresh`, not
just once. Point to the same "treat state as sensitive, consider an
encrypted/access-controlled backend" guidance the Actions section of
`README.md` already gives for financial risk — same treatment, different
risk category.

"Read-only" does not make this concern go away — a common
misclassification this guardrail exists to prevent. A data source writes
to state exactly as durably as a resource does; the only difference is
that a resource's write happens once at apply-time and a data source's
happens on every refresh.

## Why

Derived from [[0011-v4-scope-data-sources-and-regression-guard]] item 4
(`paddle_customer`), which reopened part of
[[0010-v3-scope-lifecycle-actions]]'s PII-in-state deferral. That
decision's original PII concern was raised specifically against a
*resource* design (`paddle_customer` as full CRUD); revisiting it for a
*data source* instead surfaced that the same concern applies essentially
unchanged — Terraform's state-persistence behavior doesn't distinguish
resources from data sources for this purpose. Rather than re-deferring
indefinitely (0010's original disposition) or shipping silently (treating
"read-only" as sufficient mitigation on its own), the resolution here is
documentation-as-mitigation: the same posture this provider already takes
for actions' financial risk, applied consistently to a different kind of
sensitive data.

## Applies to

- `paddle_customer` (v4) and any future data source exposing customer
  PII (a hypothetical `paddle_address`, `paddle_business`, etc., should
  0010's fuller deferral ever be revisited).
- `internal/provider/*_data_source.go` schema `MarkdownDescription` text.
- `README.md`.

## Related

- [[0010-v3-scope-lifecycle-actions]]
- [[0011-v4-scope-data-sources-and-regression-guard]]
