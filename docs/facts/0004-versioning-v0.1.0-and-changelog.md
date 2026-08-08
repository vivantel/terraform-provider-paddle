---
title: Versioning starts at v0.1.0; CHANGELOG.md generated via kms:changelog
status: current
date: 2026-08-08
tags: [paddle, provider, versioning, changelog]
---

## Fact

The first tagged release of `terraform-provider-paddle` is `v0.1.0`,
signaling a pre-1.0/unstable public API per semver convention — this
matched the README's framing at the time this decision was made ("not yet
tested against a real Paddle account"), before every resource and data
source was later verified end-to-end against the real sandbox (see
`docs/plans/paddle-provider-v1.md` Step 5) and the README was rewritten to
reflect that. `CHANGELOG.md` entries are generated from Conventional Commit
history using the `kms:changelog` skill already available in this
environment, run before each tag is pushed (see
`docs/skills/release-with-kms-changelog.md`).

This was a low-friction call during the roadmap interview — starting at
`v1.0.0` before the provider has real-world usage was explicitly rejected as
premature, and hand-writing the changelog was rejected since the
`kms:changelog` tooling already exists and Conventional Commit messages are
already the convention via `kms:attribute` (see
`docs/skills/commit-with-kms-attribute.md`).

## Related

- [[0004-release-via-goreleaser-github-actions]]
- `docs/skills/release-with-kms-changelog.md`
- `docs/plans/paddle-provider-v1.md`
