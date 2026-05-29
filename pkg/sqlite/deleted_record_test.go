//go:build integration
// +build integration

package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/sqlite"
)

// insertTombstone writes a raw deleted_records row with a controlled deleted_at and returns its
// autoincrement id (the cursor). It uses ExecSQL so the row participates in the rollback txn.
func insertTombstone(t *testing.T, ctx context.Context, recordType string, recordID int, deletedAt time.Time) int64 {
	t.Helper()
	deletedAtStr := deletedAt.UTC().Format(sqlite.TimestampFormat)
	_, lastID, err := db.ExecSQL(ctx,
		"INSERT INTO deleted_records (record_type, record_id, deleted_at) VALUES (?, ?, ?)",
		[]interface{}{recordType, recordID, deletedAtStr})
	if err != nil {
		t.Fatalf("inserting tombstone (%s/%d @ %s): %v", recordType, recordID, deletedAtStr, err)
	}
	if lastID == nil {
		t.Fatalf("inserting tombstone (%s/%d): no last insert id", recordType, recordID)
	}
	return *lastID
}

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

// TestDeletedSinceSameSecondCursor is the same-second regression test. Two tombstones share the
// exact same whole-second deleted_at but have distinct, monotonic ids. A client that advances by
// the deleted_at watermark (legacy GetDeletedSince with strict `>`) PERMANENTLY MISSES the second
// row. The id cursor (GetDeletedSincePage with `after`) must STILL return it. This fails on the
// legacy path and must pass on the cursor path.
func TestDeletedSinceSameSecondCursor(t *testing.T) {
	runWithRollbackTxn(t, "deletedSince same-second cursor", func(t *testing.T, ctx context.Context) {
		// Two tombstones written within the same whole second (identical RFC3339 text).
		sameSecond := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)

		firstCursor := insertTombstone(t, ctx, "scenes", 9001, sameSecond)
		secondCursor := insertTombstone(t, ctx, "scenes", 9002, sameSecond)

		if secondCursor <= firstCursor {
			t.Fatalf("expected monotonic ids; got first=%d second=%d", firstCursor, secondCursor)
		}

		// --- Legacy time-watermark behaviour (demonstrates the bug) ---
		// A client that read the first row and advanced its watermark to that row's deleted_at
		// will, with strict `>` on a 1-second-resolution column, drop the second same-second row.
		legacy, err := db.GetDeletedSince(ctx, sameSecond)
		if err != nil {
			t.Fatalf("GetDeletedSince: %v", err)
		}
		var legacySaw9002 bool
		for _, r := range legacy {
			if r.RecordType == "scenes" && r.RecordID == 9002 {
				legacySaw9002 = true
			}
		}
		if legacySaw9002 {
			t.Fatalf("precondition failed: legacy time-watermark unexpectedly returned the same-second row; the regression scenario is not being exercised")
		}

		// --- Cursor behaviour (the fix) ---
		// Resuming AFTER the first row's id must still return the second same-second row.
		page, err := db.GetDeletedSincePage(ctx, nil, &firstCursor, 100)
		if err != nil {
			t.Fatalf("GetDeletedSincePage(after=firstCursor): %v", err)
		}
		var cursorSaw9002 bool
		for _, r := range page.Records {
			if r.RecordType == "scenes" && r.RecordID == 9002 {
				cursorSaw9002 = true
			}
			if r.RecordType == "scenes" && r.RecordID == 9001 {
				t.Errorf("row already consumed (id %d) must not reappear when resuming after it", firstCursor)
			}
		}
		if !cursorSaw9002 {
			t.Errorf("SAME-SECOND REGRESSION: tombstone scenes/9002 (same second as the cursor row) was dropped; cursor paging must return it")
		}
		if page.NextCursor != secondCursor {
			t.Errorf("expected NextCursor=%d (second row id), got %d", secondCursor, page.NextCursor)
		}
	})
}

// TestDeletedSincePagination asserts that limit is honored, the cursor advances exactly, hasMore
// flips correctly, and the since-anchor produces a usable starting cursor.
func TestDeletedSincePagination(t *testing.T) {
	runWithRollbackTxn(t, "deletedSince pagination", func(t *testing.T, ctx context.Context) {
		base := time.Date(2031, 6, 1, 12, 0, 0, 0, time.UTC)

		// Insert 5 tombstones, one per second so ordering by id matches insertion order.
		var ids []int64
		for i := 0; i < 5; i++ {
			id := insertTombstone(t, ctx, "tags", 7000+i, base.Add(time.Duration(i)*time.Second))
			ids = append(ids, id)
		}

		// Page 1: limit 2 from the very beginning (after the lowest id - 1).
		startCursor := ids[0] - 1
		p1, err := db.GetDeletedSincePage(ctx, nil, &startCursor, 2)
		if err != nil {
			t.Fatalf("page 1: %v", err)
		}
		if len(p1.Records) != 2 {
			t.Fatalf("page 1: expected 2 records (limit honored), got %d", len(p1.Records))
		}
		if !p1.HasMore {
			t.Errorf("page 1: expected HasMore=true")
		}
		if p1.NextCursor != ids[1] {
			t.Errorf("page 1: expected NextCursor=%d, got %d", ids[1], p1.NextCursor)
		}

		// Page 2: resume from page 1's cursor, limit 2.
		p2, err := db.GetDeletedSincePage(ctx, nil, &p1.NextCursor, 2)
		if err != nil {
			t.Fatalf("page 2: %v", err)
		}
		if len(p2.Records) != 2 {
			t.Fatalf("page 2: expected 2 records, got %d", len(p2.Records))
		}
		if p2.Records[0].RecordID != 7002 || p2.Records[1].RecordID != 7003 {
			t.Errorf("page 2: expected record ids 7002,7003 got %d,%d", p2.Records[0].RecordID, p2.Records[1].RecordID)
		}
		if !p2.HasMore {
			t.Errorf("page 2: expected HasMore=true (one row remains)")
		}

		// Page 3: the last row; HasMore must be false.
		p3, err := db.GetDeletedSincePage(ctx, nil, &p2.NextCursor, 2)
		if err != nil {
			t.Fatalf("page 3: %v", err)
		}
		if len(p3.Records) != 1 {
			t.Fatalf("page 3: expected 1 record, got %d", len(p3.Records))
		}
		if p3.HasMore {
			t.Errorf("page 3: expected HasMore=false (feed exhausted)")
		}
		if p3.Records[0].RecordID != 7004 {
			t.Errorf("page 3: expected last record id 7004, got %d", p3.Records[0].RecordID)
		}

		// since-anchor: anchoring at base+1s should start strictly after the boundary, i.e. the
		// next id, and yield a usable cursor. base+1s is the deleted_at of ids[1]; MAX(id) where
		// deleted_at <= base+1s is ids[1], so paging starts at ids[2].
		sinceAnchor := base.Add(time.Second)
		ps, err := db.GetDeletedSincePage(ctx, &sinceAnchor, nil, 10)
		if err != nil {
			t.Fatalf("since-anchor page: %v", err)
		}
		if len(ps.Records) != 3 {
			t.Fatalf("since-anchor: expected 3 records (ids 7002..7004), got %d", len(ps.Records))
		}
		if ps.Records[0].RecordID != 7002 {
			t.Errorf("since-anchor: expected first record 7002, got %d", ps.Records[0].RecordID)
		}
	})
}

// TestDeletedSinceLimitClamp asserts a limit above the maximum is clamped and a non-positive
// limit falls back to the default (we just verify it returns rows without error and is bounded).
func TestDeletedSinceLimitClamp(t *testing.T) {
	runWithRollbackTxn(t, "deletedSince limit clamp", func(t *testing.T, ctx context.Context) {
		base := time.Date(2032, 1, 1, 0, 0, 0, 0, time.UTC)
		insertTombstone(t, ctx, "studios", 8001, base)

		// Over-max limit must not error and must return the single available row.
		start := int64(0)
		page, err := db.GetDeletedSincePage(ctx, nil, &start, sqlite.MaxDeletedSinceLimit+100000)
		if err != nil {
			t.Fatalf("over-max limit: %v", err)
		}
		if len(page.Records) != 1 {
			t.Errorf("expected 1 record under clamped limit, got %d", len(page.Records))
		}

		// Zero limit falls back to the default and still returns the row.
		page0, err := db.GetDeletedSincePage(ctx, nil, &start, 0)
		if err != nil {
			t.Fatalf("zero limit: %v", err)
		}
		if len(page0.Records) != 1 {
			t.Errorf("expected 1 record under default limit, got %d", len(page0.Records))
		}
	})
}

// TestPruneDeletedRecords asserts the retention prune deletes only tombstones older than the
// horizon, that a zero-time horizon is a no-op, and that MinDeletedRecordCursor reflects the floor
// (used by the resolver to set the `pruned` flag).
func TestPruneDeletedRecords(t *testing.T) {
	runWithRollbackTxn(t, "prune deleted records", func(t *testing.T, ctx context.Context) {
		now := time.Now().UTC()
		oldID := insertTombstone(t, ctx, "images", 6001, now.AddDate(0, 0, -400)) // older than horizon
		newID := insertTombstone(t, ctx, "images", 6002, now.AddDate(0, 0, -1))   // within horizon

		// Floor before pruning is the old row.
		minID, exists, err := db.MinDeletedRecordCursor(ctx)
		if err != nil {
			t.Fatalf("MinDeletedRecordCursor: %v", err)
		}
		if !exists || minID != oldID {
			t.Fatalf("expected floor cursor %d, got %d (exists=%v)", oldID, minID, exists)
		}

		// Zero-time horizon is a no-op.
		n, err := db.PruneDeletedRecords(ctx, time.Time{})
		if err != nil {
			t.Fatalf("prune(zero): %v", err)
		}
		if n != 0 {
			t.Errorf("prune(zero): expected 0 rows pruned, got %d", n)
		}

		// Prune everything deleted before (now - 365d): only the 400-day-old row goes.
		horizon := now.AddDate(0, 0, -365)
		n, err = db.PruneDeletedRecords(ctx, horizon)
		if err != nil {
			t.Fatalf("prune: %v", err)
		}
		if n != 1 {
			t.Errorf("expected 1 row pruned, got %d", n)
		}

		// The new row remains and the floor advances to it.
		minID, exists, err = db.MinDeletedRecordCursor(ctx)
		if err != nil {
			t.Fatalf("MinDeletedRecordCursor after prune: %v", err)
		}
		if !exists || minID != newID {
			t.Errorf("expected floor cursor %d after prune, got %d (exists=%v)", newID, minID, exists)
		}
	})
}
