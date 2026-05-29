package api

import (
	"context"
	"strconv"
	"time"

	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/pkg/sqlite"
)

// DeletedSince returns tombstones for top-level entities deleted after the given time, so sync
// clients can reconcile removals incrementally. The read store lives on *sqlite.Database (see
// pkg/sqlite/deleted_record.go); we convert its rows to the generated GraphQL type here.
func (r *queryResolver) DeletedSince(ctx context.Context, since time.Time) ([]*DeletedRecord, error) {
	db := manager.GetInstance().Database

	var records []sqlite.DeletedRecord
	if err := r.withReadTxn(ctx, func(ctx context.Context) error {
		var err error
		records, err = db.GetDeletedSince(ctx, since)
		return err
	}); err != nil {
		return nil, err
	}

	ret := make([]*DeletedRecord, len(records))
	for i, rec := range records {
		ret[i] = &DeletedRecord{
			Entity:    rec.RecordType,
			ID:        strconv.Itoa(rec.RecordID),
			DeletedAt: rec.DeletedAt,
		}
	}

	return ret, nil
}
