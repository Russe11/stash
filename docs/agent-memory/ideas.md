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

### 2026-05-29 — entityChanged GraphQL subscription (DONE — clients to adopt)
Added NG subscription `entityChanged(types: [String!]): EntityChangeEvent!` (`EntityChangeEvent { entity, id, operation, time }`, metadata-only — no titles/paths, mirroring the webhook payload) over the WS transport already wired in `internal/api/server.go`. Published from the single post-commit choke point `pkg/plugin/plugins.go` (`ExecutePostHooks`, same call that fires webhooks — no new hooks in N places) via a small in-process broadcaster `pkg/plugin/entity_events.go` (`EntityEventBroadcaster`/`EntityEvents`, non-blocking 256-event-buffered fan-out). Resolver `internal/api/resolver_subscription_entity.go` filters by `types` and cleans up on disconnect. Capability flag `entityChanged` added to `NGFeatures()` (no `NGAPIVersion` bump — additive). Documented in `docs/api/ng-contract.md` §5b. **Client follow-up:** viewers/manager can live-update on `features.contains("entityChanged")` by subscribing and refetching the entity by id (or pruning on Destroy), falling back to `deletedSince`+poll when absent. Treat the feed as a refetch hint, not a gap-free log (slow subscribers drop events); use `deletedSince` for authoritative deletion reconciliation.
