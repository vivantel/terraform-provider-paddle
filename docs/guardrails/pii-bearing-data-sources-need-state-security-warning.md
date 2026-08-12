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

**PII doesn't only mean a dedicated field.** `paddle_events`' `data`
attribute is arbitrary, per-event-type JSON that can carry customer PII
just as directly as a dedicated `email`/`name` field would (a
`customer.created` event's payload *is* a customer record) — this
guardrail applies to a data source whose output *can* carry PII, not
only one whose schema names a PII field explicitly. When a data source's
output is opaque/variable-shape like this, redacting it reliably isn't
generally possible (the shape varies per event type, arbitrarily) — the
same documentation-as-mitigation treatment applies instead, not a
technical filter.

**Plural/list variants compound this, not just repeat it.** A data
source returning many records at once (e.g. `paddle_customers`) puts
*more* PII into state per use than its singular counterpart, and deserves
the same warning stated in those terms — "this returns multiple
customers' PII," not a copy-pasted singular-lookup sentence.

Also check `Sensitive: true` schema marking while auditing for this — a
related but distinct protection (hides a value from CLI/log output; does
*not* stop it writing to state). A PII-bearing attribute missing
`Sensitive: true` is worth fixing alongside the state-file warning, even
though closing that gap doesn't substitute for the warning itself.

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
- `paddle_events` (v5) — its `data` field, not a dedicated PII attribute,
  see above.
- `paddle_customers` (v5, plural, shipped `docs/plans/paddle-provider-v5.md`
  Step 4) — compounds the concern, see above; its schema
  `MarkdownDescription` and `internal/provider/customers_data_source.go`
  carry the "multiple customers' PII" wording this section calls for.
- `internal/provider/*_data_source.go` schema `MarkdownDescription` text.
- `README.md`.
- The v5 full-audit pass
  ([[0012-v5-scope-pii-data-sources-timeouts-testing]] item 1) checking
  every existing resource/data source/action schema for any other
  overlooked PII vector and any missing `Sensitive: true` marking.

## Related

- [[0010-v3-scope-lifecycle-actions]]
- [[0011-v4-scope-data-sources-and-regression-guard]]
- [[0012-v5-scope-pii-data-sources-timeouts-testing]]
