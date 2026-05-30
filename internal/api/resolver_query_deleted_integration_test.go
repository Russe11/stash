//go:build integration
// +build integration

package api

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/sqlite"

	// register custom migrations so a fresh DB opens at the latest schema
	_ "github.com/stashapp/stash/pkg/sqlite/migrations"
)

// TestMain initialises an empty config singleton before any test runs. Opening a fresh sqlite DB
// runs the full migration chain, and a post-migration step (schema12 migrateConfig) reads
// config.GetInstance(); without this it panics ("config not initialized"). InitializeEmpty just
// sets an in-memory config — it is NOT an app/manager bootstrap, so the manager-singleton coupling
// the seam removed stays removed. Mirrors pkg/sqlite/setup_test.go.
func TestMain(m *testing.M) {
	_ = config.InitializeEmpty()
	os.Exit(m.Run())
}

// itoa64 renders an int64 as the decimal string the deletedSince cursor uses.
func itoa64(n int64) string { return strconv.FormatInt(n, 10) }

// newDeletedSinceTestResolver stands up a throwaway sqlite database (its own temp file, full
// migration) and returns a Resolver wired the same way Initialize wires production: repository =
// db.Repository() (so withReadTxn runs against real sqlite) and deletedRecordReader = the real *db.
// No manager singleton / app bootstrap is involved — that is the whole point of the seam.
func newDeletedSinceTestResolver(t *testing.T) (*Resolver, *sqlite.Database) {
	t.Helper()

	db := sqlite.NewDatabase()
	dbPath := filepath.Join(t.TempDir(), "deleted_since_test.sqlite")
	if err := db.Open(dbPath); err != nil {
		t.Fatalf("opening test database at %s: %v", dbPath, err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})

	return &Resolver{
		repository:          db.Repository(),
		deletedRecordReader: db,
	}, db
}

// seedTombstone writes a raw deleted_records row with a controlled deleted_at, inside a write txn so
// the ctx-bound ExecSQL participates, and returns its autoincrement id (the cursor). Mirrors the
// pkg/sqlite insertTombstone helper.
func seedTombstone(t *testing.T, db *sqlite.Database, recordType string, recordID int, deletedAt time.Time) int64 {
	t.Helper()
	deletedAtStr := deletedAt.UTC().Format(sqlite.TimestampFormat)

	var id int64
	repo := db.Repository()
	if err := repo.WithTxn(context.Background(), func(ctx context.Context) error {
		_, lastID, err := db.ExecSQL(ctx,
			"INSERT INTO deleted_records (record_type, record_id, deleted_at) VALUES (?, ?, ?)",
			[]interface{}{recordType, recordID, deletedAtStr})
		if err != nil {
			return err
		}
		if lastID == nil {
			t.Fatalf("seeding tombstone (%s/%d): no last insert id", recordType, recordID)
		}
		id = *lastID
		return nil
	}); err != nil {
		t.Fatalf("seeding tombstone (%s/%d @ %s): %v", recordType, recordID, deletedAtStr, err)
	}
	return id
}

func strptr(s string) *string { return &s }
func intptr(n int) *int       { return &n }

// TestDeletedSinceResolverEndToEnd drives the DeletedSince query resolver method against a real
// sqlite database through the new deletedRecordReader seam, asserting the full path the helper-level
// unit tests cannot reach: store row -> GraphQL DeletedRecord field mapping, the cursor advancing
// across pages, has_more flipping, and the pruned boundary.
func TestDeletedSinceResolverEndToEnd(t *testing.T) {
	resolver, db := newDeletedSinceTestResolver(t)
	r := resolver.Query().(*queryResolver)
	ctx := context.Background()

	base := time.Date(2031, 6, 1, 12, 0, 0, 0, time.UTC)

	// Seed 5 tombstones, one per second so id order matches insertion order. Mixed entity types so
	// the entity field mapping is exercised, not just "scenes".
	type seed struct {
		recordType string
		recordID   int
	}
	seeds := []seed{
		{"scenes", 100},
		{"performers", 7},
		{"tags", 42},
		{"studios", 9},
		{"images", 500},
	}
	var ids []int64
	for i, s := range seeds {
		ids = append(ids, seedTombstone(t, db, s.recordType, s.recordID, base.Add(time.Duration(i)*time.Second)))
	}

	// --- Page 1: initial sync from the very beginning (no after, no since), limit 2 ---
	p1, err := r.DeletedSince(ctx, nil, nil, intptr(2))
	if err != nil {
		t.Fatalf("page 1 DeletedSince: %v", err)
	}
	if len(p1.Records) != 2 {
		t.Fatalf("page 1: expected 2 records (limit honored), got %d", len(p1.Records))
	}
	// Field mapping: first seeded row is scenes/100 with cursor ids[0].
	rec0 := p1.Records[0]
	if rec0.Entity != "scenes" || rec0.ID != "100" || rec0.Cursor != itoa64(ids[0]) {
		t.Errorf("page 1 record[0] = entity=%q id=%q cursor=%q, want scenes/100/%d", rec0.Entity, rec0.ID, rec0.Cursor, ids[0])
	}
	if !rec0.DeletedAt.Equal(base) {
		t.Errorf("page 1 record[0] deletedAt = %v, want %v", rec0.DeletedAt, base)
	}
	rec1 := p1.Records[1]
	if rec1.Entity != "performers" || rec1.ID != "7" || rec1.Cursor != itoa64(ids[1]) {
		t.Errorf("page 1 record[1] = entity=%q id=%q cursor=%q, want performers/7/%d", rec1.Entity, rec1.ID, rec1.Cursor, ids[1])
	}
	if !p1.HasMore {
		t.Errorf("page 1: expected has_more=true (3 rows remain)")
	}
	// Cursor advanced to the last row's id (ids[1]).
	if p1.Cursor != itoa64(ids[1]) {
		t.Errorf("page 1: cursor = %q, want %q", p1.Cursor, itoa64(ids[1]))
	}
	if p1.Pruned {
		t.Errorf("page 1: pruned must be false (initial sync with no after cursor is never pruned)")
	}

	// --- Page 2: resume from page 1's cursor, limit 2 ---
	p2, err := r.DeletedSince(ctx, nil, strptr(p1.Cursor), intptr(2))
	if err != nil {
		t.Fatalf("page 2 DeletedSince: %v", err)
	}
	if len(p2.Records) != 2 {
		t.Fatalf("page 2: expected 2 records, got %d", len(p2.Records))
	}
	if p2.Records[0].Entity != "tags" || p2.Records[0].ID != "42" {
		t.Errorf("page 2 record[0] = %q/%q, want tags/42", p2.Records[0].Entity, p2.Records[0].ID)
	}
	if p2.Records[1].Entity != "studios" || p2.Records[1].ID != "9" {
		t.Errorf("page 2 record[1] = %q/%q, want studios/9", p2.Records[1].Entity, p2.Records[1].ID)
	}
	if !p2.HasMore {
		t.Errorf("page 2: expected has_more=true (one row remains)")
	}
	if p2.Cursor != itoa64(ids[3]) {
		t.Errorf("page 2: cursor = %q, want %q", p2.Cursor, itoa64(ids[3]))
	}

	// --- Page 3: the last row; has_more must flip to false ---
	p3, err := r.DeletedSince(ctx, nil, strptr(p2.Cursor), intptr(2))
	if err != nil {
		t.Fatalf("page 3 DeletedSince: %v", err)
	}
	if len(p3.Records) != 1 {
		t.Fatalf("page 3: expected 1 record, got %d", len(p3.Records))
	}
	if p3.Records[0].Entity != "images" || p3.Records[0].ID != "500" {
		t.Errorf("page 3 record[0] = %q/%q, want images/500", p3.Records[0].Entity, p3.Records[0].ID)
	}
	if p3.HasMore {
		t.Errorf("page 3: expected has_more=false (feed exhausted)")
	}
	if p3.Cursor != itoa64(ids[4]) {
		t.Errorf("page 3: cursor = %q, want %q", p3.Cursor, itoa64(ids[4]))
	}

	// --- Empty page beyond the end holds the watermark (does not reset to 0) ---
	pEnd, err := r.DeletedSince(ctx, nil, strptr(p3.Cursor), intptr(2))
	if err != nil {
		t.Fatalf("end page DeletedSince: %v", err)
	}
	if len(pEnd.Records) != 0 {
		t.Fatalf("end page: expected 0 records, got %d", len(pEnd.Records))
	}
	if pEnd.HasMore {
		t.Errorf("end page: expected has_more=false")
	}
	if pEnd.Cursor != p3.Cursor {
		t.Errorf("end page: cursor = %q, want held at %q", pEnd.Cursor, p3.Cursor)
	}
}

// TestDeletedSinceResolverPrunedBoundary asserts the pruned flag end-to-end: a caller resuming from
// an `after` cursor that predates the oldest still-retained tombstone (the floor) is told to
// full-resync. Driven through the resolver against a real DB, so the MinDeletedRecordCursor read and
// the boundary comparison are exercised together.
func TestDeletedSinceResolverPrunedBoundary(t *testing.T) {
	resolver, db := newDeletedSinceTestResolver(t)
	r := resolver.Query().(*queryResolver)
	ctx := context.Background()

	base := time.Date(2032, 1, 1, 0, 0, 0, 0, time.UTC)
	// Seed two tombstones; the floor (oldest retained id) is the first.
	floorID := seedTombstone(t, db, "scenes", 1001, base)
	seedTombstone(t, db, "scenes", 1002, base.Add(time.Second))

	// A caller cursor strictly below the floor means rows were pruned out from under it -> pruned.
	belowFloor := floorID - 1
	pPruned, err := r.DeletedSince(ctx, nil, strptr(itoa64(belowFloor)), nil)
	if err != nil {
		t.Fatalf("pruned-case DeletedSince: %v", err)
	}
	if !pPruned.Pruned {
		t.Errorf("expected pruned=true for after-cursor %d below floor %d", belowFloor, floorID)
	}
	// It still returns the live rows (the caller can full-resync, but the feed isn't broken).
	if len(pPruned.Records) != 2 {
		t.Errorf("pruned case: expected 2 retained records, got %d", len(pPruned.Records))
	}

	// A caller cursor exactly at the floor is still retained -> not pruned.
	pAtFloor, err := r.DeletedSince(ctx, nil, strptr(itoa64(floorID)), nil)
	if err != nil {
		t.Fatalf("at-floor DeletedSince: %v", err)
	}
	if pAtFloor.Pruned {
		t.Errorf("expected pruned=false for after-cursor exactly at floor %d", floorID)
	}

	// Initial sync (no after cursor) is never pruned.
	pInitial, err := r.DeletedSince(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("initial DeletedSince: %v", err)
	}
	if pInitial.Pruned {
		t.Errorf("expected pruned=false for initial sync (no after cursor)")
	}
}
