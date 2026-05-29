package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"
)

const deletedRecordTable = "deleted_records"

// DefaultDeletedSinceLimit is the page size used when a caller does not specify a limit.
// MaxDeletedSinceLimit caps any caller-supplied limit so a single query can't be unbounded.
const (
	DefaultDeletedSinceLimit = 500
	MaxDeletedSinceLimit     = 5000
)

// DeletedRecord is a tombstone for a deleted top-level entity. It lives in the sqlite package
// (rather than pkg/models) deliberately: gqlgen autobinds pkg/models, so a models.DeletedRecord
// would bind to the GraphQL DeletedRecord type and force its field names to match. Keeping the
// read model here lets gqlgen generate its own GraphQL struct, and the resolver converts.
//
// Cursor is the table's monotonic autoincrement id. It is the exact resume token: paging by id
// (rather than by the 1-second-resolution deleted_at) means a caller resuming after the last
// row's cursor never drops a tombstone written later in the same wall-clock second.
type DeletedRecord struct {
	Cursor     int64
	RecordType string
	RecordID   int
	DeletedAt  time.Time
}

// DeletedSincePage is one page of tombstones plus the resume cursor and pagination flags.
type DeletedSincePage struct {
	Records []DeletedRecord
	// NextCursor is the id of the last record in Records, to be persisted and passed back as the
	// `after` argument. It is 0 only when no rows have ever been recorded; otherwise callers that
	// supplied an `after` keep it (so an empty page holds the watermark) — the resolver handles
	// that fallback.
	NextCursor int64
	// HasMore is true if more tombstones remain beyond this page.
	HasMore bool
}

// syncTrackedTables is the allowlist of top-level entity tables whose deletions are exposed via
// the deletedSince feed. The shared destroy path (table.destroy) is hit by far more tables than
// these — join tables, files, folders, and saved_filters all route through it, and deleting a
// scene/image with file removal cascades into files/folders destroys. Recording only these nine
// keeps the feed to the entities clients actually sync, and prevents a single scene deletion from
// emitting spurious files/folders tombstones.
var syncTrackedTables = map[string]bool{
	sceneTable:             true,
	imageTable:             true,
	galleryTable:           true,
	performerTable:         true,
	studioTable:            true,
	tagTable:               true,
	groupTable:             true,
	sceneMarkerTable:       true,
	galleriesChaptersTable: true,
}

// recordDeletions writes a tombstone for each deleted id, but only for allowlisted entity tables.
// It runs inside the same transaction as the delete (via the ctx-bound exec), so tombstones commit
// and roll back atomically with the deletion they record. A non-tracked table is a no-op.
func recordDeletions(ctx context.Context, tableName string, ids []int) error {
	if !syncTrackedTables[tableName] || len(ids) == 0 {
		return nil
	}

	// store as a canonical UTC RFC3339 string so the textual deleted_at comparison in
	// GetDeletedSince is chronological (matches sqlite.TimestampFormat used elsewhere)
	now := time.Now().UTC().Format(TimestampFormat)
	rows := make([]interface{}, len(ids))
	for i, id := range ids {
		rows[i] = goqu.Record{
			"record_type": tableName,
			"record_id":   id,
			"deleted_at":  now,
		}
	}

	q := dialect.Insert(goqu.T(deletedRecordTable)).Rows(rows...)
	if _, err := exec(ctx, q); err != nil {
		return fmt.Errorf("recording deletions for %s: %w", tableName, err)
	}

	return nil
}

// GetDeletedSince returns tombstones for entities deleted strictly after since, oldest first, so a
// sync client can advance a watermark and reconcile removals incrementally. Uses the ctx-bound
// connection (so it participates in a read transaction when one is open).
//
// NOTE: this is the legacy time-anchored read. It compares against the 1-second-resolution
// deleted_at with a strict `>`, so it can drop a tombstone written later in the same whole second
// as the watermark. It is retained for the initial-anchor / back-compat path only; the resumable
// path is GetDeletedSincePage with an id cursor. Returns ALL matching rows (unbounded) and is kept
// for internal/test use; the resolver always goes through GetDeletedSincePage.
func (db *Database) GetDeletedSince(ctx context.Context, since time.Time) ([]DeletedRecord, error) {
	wrapper := dbWrapperType{}

	const query = "SELECT `id`, `record_type`, `record_id`, `deleted_at` FROM `deleted_records` WHERE `deleted_at` > ? ORDER BY `deleted_at` ASC, `id` ASC"

	// compare against the same canonical UTC RFC3339 text the rows are stored in
	rows, err := wrapper.QueryxContext(ctx, query, since.UTC().Format(TimestampFormat))
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	defer rows.Close()

	ret, err := scanDeletedRecords(rows)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// GetDeletedSincePage returns a page of tombstones using the monotonic autoincrement id as an
// exact, resumable cursor. This is the correct resumable path: paging by id (not by the
// 1-second deleted_at) means a caller resuming after the last row's cursor can never miss a
// tombstone written later in the same wall-clock second.
//
// Anchoring (caller resolves precedence; both may be passed and after wins):
//   - if after != nil: page is `WHERE id > *after`.
//   - else if since != nil: the initial anchor; the lowest id whose deleted_at > since is found,
//     then paging proceeds by id from there. This makes even the first time-anchored page
//     same-second safe and yields a usable cursor for every subsequent poll.
//   - else: from the beginning (id > 0).
//
// limit is clamped to (0, MaxDeletedSinceLimit]; <= 0 means DefaultDeletedSinceLimit.
func (db *Database) GetDeletedSincePage(ctx context.Context, since *time.Time, after *int64, limit int) (DeletedSincePage, error) {
	wrapper := dbWrapperType{}

	switch {
	case limit <= 0:
		limit = DefaultDeletedSinceLimit
	case limit > MaxDeletedSinceLimit:
		limit = MaxDeletedSinceLimit
	}

	// Resolve the exclusive lower-bound id.
	var afterID int64
	switch {
	case after != nil:
		afterID = *after
	case since != nil:
		// Find the largest id whose deleted_at is at or before `since`; paging starts strictly
		// after it. Using MAX(id) over deleted_at <= since (rather than MIN over deleted_at >
		// since) is robust to the same-second case: any tombstone in the same second as `since`
		// that has a higher id than the boundary row is still returned. If no row is <= since,
		// afterID stays 0 (start from the beginning).
		const boundaryQuery = "SELECT COALESCE(MAX(`id`), 0) FROM `deleted_records` WHERE `deleted_at` <= ?"
		if err := wrapper.Get(ctx, &afterID, boundaryQuery, since.UTC().Format(TimestampFormat)); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return DeletedSincePage{}, err
		}
	}

	// Fetch limit+1 to detect whether more rows remain without a second count query.
	const query = "SELECT `id`, `record_type`, `record_id`, `deleted_at` FROM `deleted_records` WHERE `id` > ? ORDER BY `id` ASC LIMIT ?"
	rows, err := wrapper.QueryxContext(ctx, query, afterID, limit+1)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return DeletedSincePage{}, err
	}
	defer rows.Close()

	recs, err := scanDeletedRecords(rows)
	if err != nil {
		return DeletedSincePage{}, err
	}

	page := DeletedSincePage{}
	if len(recs) > limit {
		page.HasMore = true
		recs = recs[:limit]
	}
	page.Records = recs
	if len(recs) > 0 {
		page.NextCursor = recs[len(recs)-1].Cursor
	}

	return page, nil
}

// scanDeletedRecords drains rows produced by the standard deleted_records column projection
// (id, record_type, record_id, deleted_at) into DeletedRecord values.
func scanDeletedRecords(rows *sqlx.Rows) ([]DeletedRecord, error) {
	var ret []DeletedRecord
	for rows.Next() {
		var (
			r            DeletedRecord
			deletedAtStr string
		)
		if err := rows.Scan(&r.Cursor, &r.RecordType, &r.RecordID, &deletedAtStr); err != nil {
			return nil, err
		}
		var err error
		if r.DeletedAt, err = time.Parse(TimestampFormat, deletedAtStr); err != nil {
			return nil, fmt.Errorf("parsing deleted_at %q: %w", deletedAtStr, err)
		}
		ret = append(ret, r)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return ret, nil
}

// MinDeletedRecordCursor returns the smallest (oldest) tombstone id still present, and whether any
// tombstones exist. A caller whose persisted `after` cursor is below this floor has had tombstones
// pruned out from under it and must full-resync: the resolver uses this to set DeletedSinceResult.pruned.
func (db *Database) MinDeletedRecordCursor(ctx context.Context) (minID int64, exists bool, err error) {
	wrapper := dbWrapperType{}

	const query = "SELECT `id` FROM `deleted_records` ORDER BY `id` ASC LIMIT 1"
	if err := wrapper.Get(ctx, &minID, query); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return minID, true, nil
}

// PruneDeletedRecords deletes tombstones older than the given retention horizon (deleted_at <
// before) and returns the number of rows removed. A zero/zero-value `before` is a no-op so callers
// can disable pruning by passing the zero time. Pruning by deleted_at (not id) keeps the retention
// window expressed in wall-clock terms, matching the advertised horizon in serverCapabilities.
func (db *Database) PruneDeletedRecords(ctx context.Context, before time.Time) (int64, error) {
	if before.IsZero() {
		return 0, nil
	}

	q := dialect.Delete(goqu.T(deletedRecordTable)).
		Where(goqu.C("deleted_at").Lt(before.UTC().Format(TimestampFormat)))

	result, err := exec(ctx, q)
	if err != nil {
		return 0, fmt.Errorf("pruning deleted_records before %s: %w", before.Format(TimestampFormat), err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, nil
}
