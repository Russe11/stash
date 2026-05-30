---
name: fixes
description: Non-defect fixes — tech debt, contract hardening, code-quality adjustments
updated: 2026-05-29
---

# Fixes

Code that works but should be improved (security hardening, contract tightening, dead code, misleading comment) — things that "need fixing" but aren't behavioral bugs. Remove an entry when done.

<!--
### YYYY-MM-DD — <short title>
<one paragraph: what to change and why + file:line>
-->

### 2026-05-29 — Tracked dataloaden `*_gen.go` files are stale on `deploy` (regen breaks build)
Running `go generate ./...` regenerates four committed loader files — `internal/api/loaders/{folderrelatedfolderidsloader,scenelastplayedloader,sceneohistoryloader,sceneplayhistoryloader}_gen.go` — with content that differs from what's committed: a `FolderParentFolderIDs`→`FolderRelatedFolderIDs` rename and, worse, a **duplicate `time` import** that fails to compile (`time redeclared in this block`). The committed versions are correct/build; the generator (`github.com/vektah/dataloaden`, directives in `internal/api/loaders/dataloaders.go`) emits broken output against the current toolchain. So `go generate` is NOT idempotent on `deploy`. Workaround today: `git checkout` those four files after generating. Fix: pin/patch dataloaden or hand-fix the generated import, so `go generate ./...` is clean end-to-end. Unrelated to similar-scenes; discovered while regenerating gqlgen for it.

### 2026-05-29 — Outbound webhooks have no signing or retry (SSRF scheme/redirect guard landed)
`pkg/plugin/webhook.go` now validates each URL is plain http(s) with a host (`validateWebhookURL`, drops `file://`/`gopher://`/etc.) and the shared client no longer follows redirects (`CheckRedirect` → `ErrUseLastResponse`), closing the SSRF-pivot vectors. Private/loopback hosts are intentionally still allowed (LAN-first server; home-automation targets are private IPs). **Still missing:** no `X-Stash-Signature`/HMAC (receivers can't authenticate the sender) and no retry/backoff/queue (a transient failure is logged and dropped, so a webhook-only consumer silently desyncs). Add an HMAC header over the body using a new `webhook_secret` config key + bounded retry. NG-only; no upstream impact.

### 2026-05-29 — webhook_urls is config-file-only (no GraphQL getter/setter)
`webhook_urls` (`internal/manager/config/config.go:168`, `GetWebhookURLs` :946-949) has no field in `ConfigResult`/`ConfigGeneralInput` and no setter in `configureGeneral`, so neither the web UI nor any client can view or manage webhook targets — operators must hand-edit `config.yml`. Add it to `config.graphql` + the configure resolver. NG-only.

### 2026-05-29 — Scene media URL apikey asymmetry
Only `GetStreamURL`/`GetFunscriptURL` append `?apikey=` (`urlbuilders/scene.go`); screenshot/preview/sprite/vtt/caption/heatmap do not, so a client relying on the query-param channel 401s on those. Either append apikey consistently or document that header auth is required for non-stream media. Upstream-inherited.
