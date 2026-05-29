//go:build integration
// +build integration

package sqlite_test

import (
	"context"
	"testing"
	"time"
)

// TestDeletedRecordsFeed verifies the deletedSince tombstone feed: deleting a synced top-level
// entity records a tombstone, deleting a non-synced entity (saved_filters) does NOT, no
// non-allowlisted entity type ever leaks into the feed, and the `since` watermark filters.
func TestDeletedRecordsFeed(t *testing.T) {
	runWithRollbackTxn(t, "deleted records feed", func(t *testing.T, ctx context.Context) {
		before := time.Now().Add(-time.Second)

		sceneID := sceneIDs[sceneIdxWithGroup]
		savedFilterID := savedFilterIDs[0]

		if err := db.Scene.Destroy(ctx, sceneID); err != nil {
			t.Fatalf("destroying scene %d: %v", sceneID, err)
		}
		// saved_filters is deliberately NOT a synced entity, so its deletion must not be recorded.
		if err := db.SavedFilter.Destroy(ctx, savedFilterID); err != nil {
			t.Fatalf("destroying saved filter %d: %v", savedFilterID, err)
		}

		recs, err := db.GetDeletedSince(ctx, before)
		if err != nil {
			t.Fatalf("GetDeletedSince: %v", err)
		}

		// the only allowlisted (synced) entity types — see syncTrackedTables in deleted_record.go
		allowed := map[string]bool{
			"scenes": true, "images": true, "galleries": true, "performers": true,
			"studios": true, "tags": true, "groups": true, "scene_markers": true,
			"galleries_chapters": true,
		}

		var sawScene bool
		for _, r := range recs {
			if !allowed[r.RecordType] {
				t.Errorf("non-synced entity leaked into deletion feed: %q (id %d)", r.RecordType, r.RecordID)
			}
			if r.RecordType == "saved_filters" {
				t.Errorf("saved_filters must never appear in the deletion feed (allowlist failure)")
			}
			if r.RecordType == "scenes" && r.RecordID == sceneID {
				sawScene = true
			}
		}
		if !sawScene {
			t.Errorf("deleted scene %d is missing from the deletion feed", sceneID)
		}

		// a future watermark must return nothing (the since filter is exclusive and works)
		future, err := db.GetDeletedSince(ctx, time.Now().Add(time.Hour))
		if err != nil {
			t.Fatalf("GetDeletedSince(future): %v", err)
		}
		if len(future) != 0 {
			t.Errorf("expected no records after a future watermark, got %d", len(future))
		}
	})
}
