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

### 2026-05-29 — findSimilarScenes "more like this" (server DONE — clients to adopt)
Added NG query `findSimilarScenes(scene_id: ID!, distance: Int = 10, limit: Int = 40): [SimilarSceneResult!]!` returning phash-neighbour scenes nearest-first with their Hamming distance (`SimilarSceneResult { scene, distance }`). Schema `graphql/schema/{schema.graphql, types/scene.graphql}`; resolver `internal/api/resolver_query_find_scene.go` (`FindSimilarScenes`); storage `pkg/sqlite/scene.go` (`(*SceneStore).FindSimilar`) reusing the registered `phash_distance` SQL fn (same as the duplicate finder). Capability flag `similarScenes` added to `NGFeatures()` (no `NGAPIVersion` bump — additive). **Client follow-up:** tvOS/Android/macOS viewers can add a "More like this" row gated on `features.contains("similarScenes")`. Apple clients need a StashKit query string + `SimilarSceneResult` decode; Android needs the Apollo regen (after submodule repoint, Wave 2 Stream H).

### 2026-05-29 — serverCapabilities query (DONE — clients must now adopt it)
Added `serverCapabilities { edition apiVersion features }` (`graphql/schema/types/server-capabilities.graphql`, resolver `internal/api/resolver.go`, consts `internal/build/version.go`: `Edition="ng"`, `NGAPIVersion=1`, `NGFeatures()`). This is the explicit fork-detection handshake replacing both clients' try-and-fail probing. **Follow-up lives in the client repos:** macOS should probe it once at connect and gate `moveFolder`/`deletedSince`; Android should probe it and gate the (to-be-added) NG folder/deletion paths. Keep `NGFeatures()` in sync as NG ops are added.

### 2026-05-29 — Entity-change GraphQL subscription
Only `jobsSubscribe`/`loggingSubscribe`/`scanCompleteSubscribe` exist; there is no entity create/update/delete subscription, so clients poll (`findScenes` by `updated_at` + `deletedSince`). A scene/image/gallery change subscription over the existing WS transport would let clients live-update without polling. Larger effort; gate behind a feature flag and keep polling as the upstream fallback.
