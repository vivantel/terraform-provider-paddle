# AGENTS.md

Guidance for any AI coding agent working in this repository. `CLAUDE.md` is
a symlink to this file — one source of truth, not two files to keep in
sync.

## Read this first, every time

- `CONTRIBUTING.md` — dev setup, the per-resource patterns this codebase has
  learned the hard way (each one cost a real sandbox failure to discover
  once; don't rediscover any of them).
- `docs/plans/` — the actual state of the world. Before touching anything,
  check the relevant plan's per-step `Status:` line rather than assuming
  what's done. Don't trust a status line's claim either without checking
  what it cites — see `docs/skills/verify-before-claiming.md`.
- `docs/decisions/`, `docs/facts/`, `docs/guardrails/`, `docs/skills/` — the
  project's durable knowledge base. A judgment call not spelled out in code
  or an open plan step is very likely already covered by one of these;
  check before improvising.

## Non-negotiables

1. **Verify against reality, don't infer from a similar prior case.**
   `docs/skills/verify-before-claiming.md`. Every real bug in this
   provider's history happened when this was skipped — an assumed API
   shape, an assumed sandbox state, a status line written because it's what
   usually happens at that step rather than because it was checked.
2. **TDD for bug fixes**: a failing test first, confirmed red, then the
   fix, confirmed green. Not "fix then add a test after."
3. **A resource/feature isn't done until confirmed against the real Paddle
   sandbox** (`TF_ACC=1 PADDLE_API_KEY=... go test ./... -run TestAcc -v`,
   or let CI's `acceptance` job do it), not just `go build`/unit tests
   passing locally. `docs/decisions/0003-acceptance-tests-against-live-sandbox.md`.
4. **Bulk mechanical edits (sed/regex sweeps across many call sites) need a
   per-site semantic read afterward**, not just "it compiles and tests
   pass." `docs/guardrails/bulk-mechanical-edits-need-per-site-review.md` —
   this exact class of bug shipped once already, invisible to every
   automated check.
5. **Regenerate docs after any schema change** (`tfplugindocs generate`)
   and confirm `git diff --exit-code -- docs/` is clean before pushing —
   CI enforces this, but catching it locally is faster.
   `docs/guardrails/docs-must-be-regenerated-before-merge.md`.
6. **Commits carry `Refs:` trailers** to the `docs/{decisions,facts,
   guardrails,skills}/` file(s) they implement. `docs/skills/commit-with-kms-attribute.md`.

## What NOT to do

- Don't add resources for entities without confirming the real API's field
  list and CRUD support first — Checkout Domains looked like every other
  entity until the live API reference showed it has no create/update
  operation at all. Fetch the real docs before designing a schema.
- Don't assume an Optional field stays purely user-set — if the API might
  silently default it, it needs `Computed` + either a real `Default` or
  `UseStateForUnknown`, or the very first `Create` produces "Provider
  produced inconsistent result after apply."
- Don't reuse a resource's Create body type for its Update body (or vice
  versa) without checking whether the API's accepted field set actually
  matches between the two — see `client.PriceUpdate` and
  `client.NotificationSettingCreate`/`Update` for two different reasons
  this bit before.
