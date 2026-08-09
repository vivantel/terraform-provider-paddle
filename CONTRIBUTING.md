# Contributing

## Local development

See the [Development](README.md#development) and [Publishing](README.md#publishing-to-the-public-terraform-registry) sections in `README.md` for Go version, build/test/lint commands, `dev_overrides` setup for iterating without a full release, and the release process. This file doesn't repeat that — it covers what README.md doesn't.

## Where design rationale lives

This repo keeps *why* separate from *what*: commit messages and code comments explain a specific change, but the durable reasoning behind a decision — why an entity is in scope, why a field is modeled the way it is, what was verified against the real API and when — lives in:

- `docs/decisions/` — numbered, one per decision (`NNNN-slug.md`). Read before touching anything an existing decision covers.
- `docs/facts/` — descriptive, numbered the same way.
- `docs/guardrails/` — standing "must/must not" rules derived from a decision (slug-only, no number).
- `docs/plans/` — self-sufficient implementation plans for a specific batch of work (e.g. `paddle-provider-v1.md`, `paddle-provider-v2.md`), each with a per-step `Status:` line updated as work progresses. Start here for "what's already done and what's still open."

If you're about to make a judgment call that isn't spelled out in code or an open plan step, check whether a decision already covers it before improvising.

## Patterns every resource follows

Before adding or changing a resource, read at least one existing one in full (`internal/provider/discount_resource.go` is a good example — it has custom_data, several Optional+Computed fields, and a nested list) and its guardrail-referenced comments. Non-obvious rules that have each cost a real sandbox failure to discover once already, and shouldn't need discovering twice:

- Every Optional+Computed attribute needs both an `IsNull()` *and* `IsUnknown()` check before calling `Value*()` — `ValueString()`/`ValueBool()`/etc. on an Unknown value silently returns the zero value instead of erroring, which sends a wrong value to Paddle instead of omitting the field.
- An Optional+Computed field the API auto-assigns when omitted needs a real `Default` (or `UseStateForUnknown` if there's no sensible static default), not just one or the other — omitting create-time client behavior for this Paddle would otherwise auto-fill produces "Provider produced inconsistent result after apply" on the very first real `Create`.
- `Read()` (and the matching data source's `Read()`) fetches only `id` via `req.State.GetAttribute`/`req.Config.GetAttribute`, not the whole model via a full `Get()` — a model with a Required non-pointer nested struct field crashes decoding a null value into it otherwise (see `price_resource.go`'s `Read()` comment for the exact failure).
- If an API rejects a field on update that it accepts on create (or vice versa), that needs a dedicated request-body type, not a reused one — see `client.PriceUpdate` (drops `product_id`) or `client.NotificationSettingCreate`/`Update` (different field sets entirely, plus an asymmetric `subscribed_events` shape between request and response).
- Every pure `toAPI*`/`fromAPI*` function gets unit tests (no sandbox needed). Every resource gets acceptance tests confirmed against the real sandbox — `go test ./...` passing locally without `PADDLE_API_KEY` set doesn't mean a resource is done, only that its unit tests and compile are clean.
- Regenerate docs (`tfplugindocs generate`) after any schema change and confirm `git diff --exit-code -- docs/` is clean before pushing — CI's `docs` job enforces this, but catching it locally first is faster.

## Commits and PRs

This repo uses Conventional Commits (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`, ...) with `Refs:` trailers pointing at the `docs/{decisions,guardrails,facts,plans}/` file(s) a commit implements, e.g.:

```
feat: add paddle_discount_group resource and data source

<why paragraph — the reasoning, not a restatement of the diff>

Refs: docs/decisions/0007-v2-scope-discount-groups-and-notification-settings.md
Refs: docs/plans/paddle-provider-v2.md
```

A commit message's job is to explain *why*, not to re-describe the diff — that's already visible in `git show`. PR descriptions follow the same why/what-changed/refs structure, built from the union of `Refs:` trailers already on the branch's commits.
