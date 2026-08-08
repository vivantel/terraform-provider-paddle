---
title: A Paddle sandbox account/API key already exists
status: current
date: 2026-08-08
tags: [paddle, sandbox, prerequisite]
---

## Fact

A Paddle sandbox account and API key already exist and are available for use
in acceptance testing and CI (confirmed by the user during the
[[0003-acceptance-tests-against-live-sandbox]] interview on 2026-08-08). The
key itself is not recorded here — it is expected to be provided locally via
`PADDLE_API_KEY` when running acceptance tests, and stored as a GitHub
Actions secret (also named `PADDLE_API_KEY`) for CI.

This removed "provision a sandbox account" from the list of manual
prerequisites in `docs/plans/paddle-provider-v1.md` from the start — unlike
the GPG signing key (see [[0004-release-via-goreleaser-github-actions]] /
the plan's Step 0), which didn't exist yet at the time this fact was
written and had to be generated; it's since been created and uploaded to
the Terraform Registry, see Step 0's status block for the details.

## Related

- [[0003-acceptance-tests-against-live-sandbox]]
- `docs/plans/paddle-provider-v1.md`
