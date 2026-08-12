---
title: Provider maturity stage, for consumption by vivantel-www's tool registry
status: current
date: 2026-08-12
tags: [paddle, provider, maturity, vivantel-www]
---

## Fact

This file exists so `vivantel/vivantel-www` (the company site) has a real maturity claim to source
site copy from if/when this repo is registered in its `docs/ai/facts/tool-registry.md`, rather than
inferring one from this repo's version number or commit activity — see that repo's
`docs/ai/skills/brand-steward.md` § "Products vs. tools" and `tool-registry.md`'s prerequisite
section, which named this file as the blocker.

**Stage: Pre-1.0, functionally verified, no known production adopters.**

- **Semver**: latest tag `v0.5.0` (released 2026-08-11; `v0.4.0` is the latest release with every
  shipped surface stable and sandbox-verified end-to-end — see `CHANGELOG.md`). Pre-1.0 per
  [[0004-versioning-v0.1.0-and-changelog]] — the public API (resource/data-source schemas, action
  signatures) is not yet under a stability guarantee and can still change between minor versions.
- **Functional verification**: every resource and its matching data source has a real-sandbox,
  not just unit-tested, create/update/import/destroy round trip, run in CI on every push
  (`.github/workflows/ci.yaml`'s `acceptance` job). The five Terraform actions added in `v0.4.0`
  are real-sandbox-verified the same way and additionally covered by a scheduled `e2e.yaml`
  regression run against the published Registry binary — see README's "Status" section, which this
  fact defers to as the live source of exactly what's verified as of any given commit.
- **Distribution**: published to the public Terraform Registry (`registry.terraform.io/vivantel/paddle`,
  per [[0001-existing-provider-baseline]]) and installable by anyone — this is not a private/internal-only
  artifact.
- **Known production/paying-customer usage: none evidenced in this repo.** No changelog entry,
  issue, or doc records an external adopter, a support request, or usage outside this repo's own
  sandbox-verification CI. Do not describe this provider as "in production use" or "adopted by"
  anyone on that basis — there is no such evidence here, the same posture
  `virage-ee`'s `product-maturity-stage.md` takes toward unevidenced revenue claims.
- **Support/SLA**: none formally offered. `SECURITY.md` covers vulnerability reporting only;
  no support-response-time or backward-compatibility commitment exists anywhere in this repo.

## How a consuming site should phrase this

Accurate framing: "actively developed, pre-1.0 Terraform provider; every resource verified
end-to-end against a real Paddle sandbox; published to the Terraform Registry." Do not claim
"production-ready," "stable," or "adopted by X" — none of those have supporting evidence in this
repo as of this fact's date. Re-verify against README's "Status" section and the latest tag before
reusing this claim, since both change frequently (multiple releases within days of each other per
`CHANGELOG.md`).

## Related

- [[0004-versioning-v0.1.0-and-changelog]]
- [[0001-existing-provider-baseline]]
- README.md § "Status"
- `vivantel/vivantel-www`'s `docs/ai/facts/tool-registry.md` and `docs/ai/skills/brand-steward.md`
