package api

import (
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/sqlite"
)

func i64ptr(n int64) *int64 { return &n }

func TestParseDeletedSinceAfter(t *testing.T) {
	// nil / empty / malformed all degrade to "no cursor" (nil) so a corrupted client watermark is
	// non-fatal — the resolver then falls back to `since` / the beginning.
	if got := parseDeletedSinceAfter(nil); got != nil {
		t.Errorf("nil after = %d, want nil", *got)
	}

	cases := []struct {
		name string
		in   string
		want *int64
	}{
		{"empty", "", nil},
		{"malformed", "not-a-number", nil},
		{"trailing junk", "42x", nil},
		{"zero", "0", i64ptr(0)},
		{"positive", "42", i64ptr(42)},
		{"large", "9007199254740993", i64ptr(9007199254740993)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := c.in
			got := parseDeletedSinceAfter(&in)
			switch {
			case c.want == nil && got != nil:
				t.Errorf("parseDeletedSinceAfter(%q) = %d, want nil", c.in, *got)
			case c.want != nil && got == nil:
				t.Errorf("parseDeletedSinceAfter(%q) = nil, want %d", c.in, *c.want)
			case c.want != nil && got != nil && *got != *c.want:
				t.Errorf("parseDeletedSinceAfter(%q) = %d, want %d", c.in, *got, *c.want)
			}
		})
	}
}

func TestBuildDeletedSinceResultMapsRecords(t *testing.T) {
	t0 := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Second)
	page := sqlite.DeletedSincePage{
		Records: []sqlite.DeletedRecord{
			{Cursor: 10, RecordType: "scenes", RecordID: 100, DeletedAt: t0},
			{Cursor: 11, RecordType: "performers", RecordID: 7, DeletedAt: t1},
		},
		NextCursor: 11,
		HasMore:    true,
	}

	res := buildDeletedSinceResult(page, nil, 0, false)

	if len(res.Records) != 2 {
		t.Fatalf("records len = %d, want 2", len(res.Records))
	}
	// store row -> GraphQL DeletedRecord: RecordType->Entity, RecordID->ID (decimal),
	// Cursor->Cursor (decimal), DeletedAt passthrough.
	r0 := res.Records[0]
	if r0.Entity != "scenes" || r0.ID != "100" || r0.Cursor != "10" || !r0.DeletedAt.Equal(t0) {
		t.Errorf("record[0] = %+v, want entity=scenes id=100 cursor=10 deletedAt=%v", r0, t0)
	}
	r1 := res.Records[1]
	if r1.Entity != "performers" || r1.ID != "7" || r1.Cursor != "11" || !r1.DeletedAt.Equal(t1) {
		t.Errorf("record[1] = %+v, want entity=performers id=7 cursor=11 deletedAt=%v", r1, t1)
	}
	if !res.HasMore {
		t.Error("HasMore should pass through from the page (true)")
	}
}

func TestBuildDeletedSinceResultEmptyPageHasEmptyRecords(t *testing.T) {
	// An empty page must yield a non-nil empty slice, not nil, so the GraphQL [DeletedRecord!]! is
	// always present.
	res := buildDeletedSinceResult(sqlite.DeletedSincePage{}, nil, 0, false)
	if res.Records == nil {
		t.Error("Records must be a non-nil (empty) slice")
	}
	if len(res.Records) != 0 {
		t.Errorf("records len = %d, want 0", len(res.Records))
	}
}

func TestBuildDeletedSinceResultNextCursor(t *testing.T) {
	cases := []struct {
		name       string
		nextCursor int64
		afterID    *int64
		want       string
	}{
		// The page advanced: use its last id.
		{"page advanced", 11, nil, "11"},
		{"page advanced overrides afterID", 11, i64ptr(3), "11"},
		// Empty page with a caller cursor: HOLD it (so an empty poll doesn't reset to 0 and replay).
		{"empty page holds afterID", 0, i64ptr(3), "3"},
		// Empty page, no caller cursor: empty string (no watermark yet).
		{"empty page no cursor", 0, nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			page := sqlite.DeletedSincePage{NextCursor: c.nextCursor}
			res := buildDeletedSinceResult(page, c.afterID, 0, false)
			if res.Cursor != c.want {
				t.Errorf("Cursor = %q, want %q", res.Cursor, c.want)
			}
		})
	}
}

func TestBuildDeletedSinceResultPruned(t *testing.T) {
	cases := []struct {
		name    string
		afterID *int64
		minID   int64
		hasAny  bool
		want    bool
	}{
		// Caller's cursor is below the oldest retained tombstone -> pruned -> must full-resync.
		{"cursor below floor", i64ptr(2), 5, true, true},
		// Cursor exactly at the floor is still retained -> not pruned.
		{"cursor at floor", i64ptr(5), 5, true, false},
		// Cursor above the floor -> not pruned.
		{"cursor above floor", i64ptr(9), 5, true, false},
		// No tombstones at all -> nothing was pruned relative to this caller.
		{"no tombstones", i64ptr(2), 0, false, false},
		// No caller cursor (initial sync) -> never pruned.
		{"no cursor", nil, 5, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := buildDeletedSinceResult(sqlite.DeletedSincePage{}, c.afterID, c.minID, c.hasAny)
			if res.Pruned != c.want {
				t.Errorf("Pruned = %v, want %v", res.Pruned, c.want)
			}
		})
	}
}
