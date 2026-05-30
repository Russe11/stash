---
name: fixes
description: Non-defect fixes — tech debt, contract hardening, code-quality adjustments
updated: 2026-05-30
---

# Fixes

Code that works but should be improved (security hardening, contract tightening, dead code, misleading comment) — things that "need fixing" but aren't behavioral bugs. Remove an entry when done.

<!--
### YYYY-MM-DD — <short title>
<one paragraph: what to change and why + file:line>
-->

### 2026-05-29 — Tracked dataloaden `*_gen.go` files are stale on `deploy` (regen breaks build)
Running `go generate ./...` regenerates four committed loader files — `internal/api/loaders/{folderrelatedfolderidsloader,scenelastplayedloader,sceneohistoryloader,sceneplayhistoryloader}_gen.go` — with content that differs from what's committed: a `FolderParentFolderIDs`→`FolderRelatedFolderIDs` rename and, worse, a **duplicate `time` import** that fails to compile (`time redeclared in this block`). The committed versions are correct/build; the generator (`github.com/vektah/dataloaden`, directives in `internal/api/loaders/dataloaders.go`) emits broken output against the current toolchain. So `go generate` is NOT idempotent on `deploy`. Workaround today: `git checkout` those four files after generating. Fix: pin/patch dataloaden or hand-fix the generated import, so `go generate ./...` is clean end-to-end. Unrelated to similar-scenes; discovered while regenerating gqlgen for it.

### 2026-05-29 — Outbound webhooks have no signing, retry, or SSRF guard
`pkg/plugin/webhook.go` POSTs to operator-configured URLs verbatim (`dispatchWebhooks` :77-100, `postWebhook` :55-72): no `X-Stash-Signature`/HMAC (receivers can't authenticate the sender), no retry/backoff/queue (a transient failure is logged and dropped, so a webhook-only consumer silently desyncs), no private-IP/scheme validation. Payload is metadata-only `{type,entity,operation,id,time}` (:17-28) so no PII leaks — good. Add an HMAC header over the body using a new `webhook_secret` config key, bounded retry, and optional private-IP deny. NG-only; no upstream impact.

### 2026-05-29 — webhook_urls is config-file-only (no GraphQL getter/setter)
`webhook_urls` (`internal/manager/config/config.go:168`, `GetWebhookURLs` :946-949) has no field in `ConfigResult`/`ConfigGeneralInput` and no setter in `configureGeneral`, so neither the web UI nor any client can view or manage webhook targets — operators must hand-edit `config.yml`. Add it to `config.graphql` + the configure resolver. NG-only.

### 2026-05-29 — apikey query param is written to the access log
The key is accepted as `?apikey=` (`pkg/session/session.go:154-159`) and stream/funscript URLs append it (`internal/api/urlbuilders/scene.go:32-37,67-71`). When `log_access` is on, `httplog.RequestLogger` (`internal/api/server.go:133-138`) records `RequestURI` (with the query) as a sticky field emitted on every response, so the credential lands in stash's own access log (and proxy logs / history). Redact the `apikey` query param in the httplog config, or prefer header auth for media routes. Mixed upstream/NG (funscript apikey was upstream #6760).

### 2026-05-29 — Non-constant-time API key comparison
`pkg/session/session.go:165` uses `c.GetAPIKey() != apiKey` (plain compare) — a timing side-channel on the single credential protecting the whole library. Use `subtle.ConstantTimeCompare`. Upstream-inherited; minor.

### 2026-05-29 — Scene media URL apikey asymmetry
Only `GetStreamURL`/`GetFunscriptURL` append `?apikey=` (`urlbuilders/scene.go`); screenshot/preview/sprite/vtt/caption/heatmap do not, so a client relying on the query-param channel 401s on those. Either append apikey consistently or document that header auth is required for non-stream media. Upstream-inherited.

### 2026-05-29 — deletedSince contract: add id cursor + sub-second precision
See bugs.md (same-second skip). The contract fix: add an opaque `id`-based cursor (`after`/`limit`) to `deletedSince`/`DeletedRecord` and/or store `deleted_at` as `RFC3339Nano`; add tombstone retention/prune. NG-only.
