package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
)

const deletedRecordTable = "deleted_records"

// DeletedRecord is a tombstone for a deleted top-level entity. It lives in the sqlite package
// (rather than pkg/models) deliberately: gqlgen autobinds pkg/models, so a models.DeletedRecord
// would bind to the GraphQL DeletedRecord type and force its field names to match. Keeping the
// read model here lets gqlgen generate its own GraphQL struct, and the resolver converts.
type DeletedRecord struct {
	RecordType string
	RecordID   int
	DeletedAt  time.Time
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
func (db *Database) GetDeletedSince(ctx context.Context, since time.Time) ([]DeletedRecord, error) {
	wrapper := dbWrapperType{}

	const query = "SELECT `record_type`, `record_id`, `deleted_at` FROM `deleted_records` WHERE `deleted_at` > ? ORDER BY `deleted_at` ASC, `id` ASC"

	// compare against the same canonical UTC RFC3339 text the rows are stored in
	rows, err := wrapper.QueryxContext(ctx, query, since.UTC().Format(TimestampFormat))
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	defer rows.Close()

	var ret []DeletedRecord
	for rows.Next() {
		var (
			r            DeletedRecord
			deletedAtStr string
		)
		if err := rows.Scan(&r.RecordType, &r.RecordID, &deletedAtStr); err != nil {
			return nil, err
		}
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
