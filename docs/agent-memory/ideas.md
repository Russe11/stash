---
name: ideas
description: Future work / features for the Stash NG server fork
updated: 2026-05-29
---

# Ideas

Broader future work, features, unscheduled ideas. Promote to `docs/plans/` when an idea matures; remove once shipped.

<!--
### YYYY-MM-DD — <short title>
<one paragraph>
-->

### 2026-05-29 — serverCapabilities query (DONE — clients must now adopt it)
Added `serverCapabilities { edition apiVersion features }` (`graphql/schema/types/server-capabilities.graphql`, resolver `internal/api/resolver.go`, consts `internal/build/version.go`: `Edition="ng"`, `NGAPIVersion=1`, `NGFeatures()`). This is the explicit fork-detection handshake replacing both clients' try-and-fail probing. **Follow-up lives in the client repos:** macOS should probe it once at connect and gate `moveFolder`/`deletedSince`; Android should probe it and gate the (to-be-added) NG folder/deletion paths. Keep `NGFeatures()` in sync as NG ops are added.

### 2026-05-29 — Entity-change GraphQL subscription
Only `jobsSubscribe`/`loggingSubscribe`/`scanCompleteSubscribe` exist; there is no entity create/update/delete subscription, so clients poll (`findScenes` by `updated_at` + `deletedSince`). A scene/image/gallery change subscription over the existing WS transport would let clients live-update without polling. Larger effort; gate behind a feature flag and keep polling as the upstream fallback.
