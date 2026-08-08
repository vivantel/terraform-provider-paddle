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

This removes "provision a sandbox account" from the list of manual
prerequisites in `docs/plans/paddle-provider-v1.md` — unlike the GPG signing
key (see [[0004-release-via-goreleaser-github-actions]] / the plan), which
does not yet exist and does need to be created.

## Related

- [[0003-acceptance-tests-against-live-sandbox]]
- `docs/plans/paddle-provider-v1.md`
