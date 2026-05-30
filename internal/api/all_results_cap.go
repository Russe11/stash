package api

import (
	"github.com/stashapp/stash/pkg/logger"
)

// maxAllResults is the hard upper bound on the number of rows any deprecated
// all*() query resolver (allScenes, allImages, allGalleries, allStudios,
// allTags, allMovies, allPerformers, allSceneMarkers) may return.
//
// Those resolvers call repository.*.All(ctx) with no LIMIT, materializing every
// row of the entity's table into memory and then serializing it. On a large
// library a single such query is a memory spike / latency cliff, and an
// unauthenticated-adjacent or buggy caller can trigger an OOM by repeatedly
// asking for whole tables. The all*() queries are deprecated in the SDL in
// favour of the paginated find*() queries, so this cap exists purely as a DoS
// guardrail rather than a feature.
//
// 100000 is deliberately generous: it comfortably exceeds the entity counts of
// any normal personal library (tags/studios/performers/movies number in the
// hundreds-to-thousands; even very large scene/image collections rarely cross
// six figures), so real callers — including the example plugin's allTags walk
// and any admin/internal use — are unaffected. It is small enough that the
// worst-case in-memory result set stays bounded. If a future caller legitimately
// needs every row of a >100k table, it should migrate to the paginated find*()
// API rather than raise this cap.
const maxAllResults = 100000

// capAllResults enforces maxAllResults on the result of an all*() resolver.
//
// Below or at the cap the slice is returned unchanged. Above the cap it is
// truncated to exactly maxAllResults and a warning is logged naming the entity
// and both counts — the truncation is never silent. entity is the GraphQL
// entity name (e.g. "scenes") used only for the log line.
func capAllResults[T any](entity string, ret []T) []T {
	if len(ret) > maxAllResults {
		logger.Warnf("all%s: result set of %d rows exceeds the %d-row cap; truncating. Use the paginated find query instead.",
			entity, len(ret), maxAllResults)
		return ret[:maxAllResults]
	}
	return ret
}
