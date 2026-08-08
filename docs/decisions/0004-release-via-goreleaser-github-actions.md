---
title: Release via GoReleaser + GitHub Actions, GPG-signed, on tag push
status: accepted
date: 2026-08-08
tags: [paddle, provider, release, ci]
---

## Decision

Releases are cut by pushing a `vX.Y.Z` tag, which triggers a GitHub Actions
workflow that runs GoReleaser to:

1. Build multi-platform/multi-arch binaries (the standard `linux/darwin/
   windows` × `amd64/arm64` matrix HashiCorp's `terraform-provider-scaffolding`
   template uses).
2. Generate a `SHA256SUMS` file and sign it with a GPG key
   (see [[0004-versioning-v0.1.0-and-changelog]] and the manual GPG key
   creation step in the plan).
3. Publish a GitHub Release with those artifacts attached.

The Terraform Registry then ingests the release automatically via its GitHub
App webhook once the repo is registered with the Registry and the GPG public
key is uploaded to the publisher's Registry account settings — no separate
publish step needed beyond the GitHub release itself.

## Why

This is HashiCorp's documented and expected release path for a
registry-published provider — deviating from it (e.g. hand-built binaries,
unsigned artifacts) either breaks Registry ingestion outright or produces a
provider users' `terraform init` will refuse to trust (Terraform verifies the
GPG signature on `SHA256SUMS` before accepting a provider from the Registry).

The alternative — "manual builds, no registry publishing yet, keep using
`dev_overrides`" — was on the table but rejected since the goal here is
explicitly a *mature*, presumably registry-distributed provider, not a
perpetually-local one.

## Consequences

- Needs a `.goreleaser.yml` at repo root following the standard
  terraform-provider template (see plan for the exact config).
- Needs a GPG signing key that does not exist yet
  ([[0004-versioning-v0.1.0-and-changelog]] plan has this as a manual
  prerequisite step) — its private key + passphrase go into GitHub Actions
  secrets (`GPG_PRIVATE_KEY`, `PASSPHRASE`), its public key gets uploaded to
  the Terraform Registry publisher account.
- The repo must be connected to the Terraform Registry (via the Registry's
  "publish a provider" GitHub App flow) before the first tag's release will
  actually appear on the Registry — this is a one-time manual step, not
  something a CI workflow can do.
- Every future release is just "push a tag" — no other manual publish step,
  as long as the above one-time setup is done.

## Related

- `docs/plans/paddle-provider-v1.md`
- [[0004-versioning-v0.1.0-and-changelog]]
