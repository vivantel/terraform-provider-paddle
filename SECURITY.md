# Security Policy

This is an unofficial, community-maintained Terraform provider for Paddle
Billing. It talks directly to Paddle's public REST API using a
user-supplied API key — no third party in the request path (see
`README.md`). That API key is the main thing worth protecting here.

## Supported versions

Only the latest published release is supported. There's no long-term
support branch at this stage (pre-1.0) — see `CHANGELOG.md` for what's
shipped.

## Reporting a vulnerability

Please report security issues privately rather than opening a public GitHub
issue, using GitHub's own [private vulnerability reporting](https://github.com/vivantel/terraform-provider-paddle/security/advisories/new)
for this repository (Security tab → "Report a vulnerability").

Include what you'd normally include in any report: affected version, a
reproduction if you have one, and what you think the impact is. Expect an
initial response within a few days.

## Scope

In scope: this repository's code — the provider binary, the API client,
anything that could leak an API key (e.g. into logs — see
`docs/guardrails/log-client-requests-with-tflog.md` for what this provider
deliberately never logs), or make a request other than what the resource
schema documents.

Out of scope: vulnerabilities in Paddle's own platform or API — report
those to Paddle directly, not here. Vulnerabilities in Terraform itself or
the `terraform-plugin-framework`/`terraform-plugin-go` libraries this
provider depends on — report those upstream to HashiCorp.
