package api

import (
	"context"
	"strconv"
	"time"

	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/sqlite"
)

// DeletedSince returns a page of deletion tombstones for top-level entities so sync clients can
// reconcile removals incrementally. The read store lives on *sqlite.Database (see
// pkg/sqlite/deleted_record.go); we convert its rows to the generated GraphQL types here.
//
// Anchoring precedence (matches the SDL doc on the deletedSince field):
//   - `after` (opaque cursor) is the resumable path — exact, immune to same-second collisions.
//   - `since` (timestamp) is the initial anchor / back-compat path only.
//   - if both are supplied, `after` wins.
//
// The result's `pruned` flag tells a caller whose `after` predates the retention horizon that it
// must full-resync rather than trust this incremental feed.
func (r *queryResolver) DeletedSince(ctx context.Context, since *time.Time, after *string, limit *int) (*DeletedSinceResult, error) {
	db := manager.GetInstance().Database

	// Parse the opaque cursor. It is the autoincrement id rendered as a decimal string; an empty
	// string means "no cursor" (fall back to since / beginning). A malformed cursor is treated as
	// no cursor rather than an error so a corrupted client watermark degrades to a full re-read
	// rather than a hard failure.
	var afterID *int64
	if after != nil && *after != "" {
		if id, err := strconv.ParseInt(*after, 10, 64); err == nil {
			afterID = &id
		}
	}

	lim := 0
	if limit != nil {
		lim = *limit
	}

	var (
		page   sqlite.DeletedSincePage
		minID  int64
		hasAny bool
	)
	if err := r.withReadTxn(ctx, func(ctx context.Context) error {
		var err error
		page, err = db.GetDeletedSincePage(ctx, since, afterID, lim)
		if err != nil {
			return err
		}
		minID, hasAny, err = db.MinDeletedRecordCursor(ctx)
		return err
	}); err != nil {
		return nil, err
	}

	records := make([]*DeletedRecord, len(page.Records))
	for i, rec := range page.Records {
		records[i] = &DeletedRecord{
			Entity:    rec.RecordType,
			ID:        strconv.Itoa(rec.RecordID),
			DeletedAt: rec.DeletedAt,
			Cursor:    strconv.FormatInt(rec.Cursor, 10),
		}
	}

	// Resume cursor to persist. If this page advanced, use its last id; otherwise hold the
	// caller's existing cursor (so an empty page does not reset the watermark to 0 and replay the
	// whole feed next time).
	nextCursor := ""
	switch {
	case page.NextCursor != 0:
		nextCursor = strconv.FormatInt(page.NextCursor, 10)
	case afterID != nil:
		nextCursor = strconv.FormatInt(*afterID, 10)
	}

	// pruned: the caller supplied a cursor that is older than the oldest tombstone still retained,
	// meaning tombstones were pruned out from under it. It must full-resync. (If there are no
	// tombstones at all, nothing was pruned relative to this caller.)
	pruned := afterID != nil && hasAny && *afterID < minID

	return &DeletedSinceResult{
		Records: records,
		Cursor:  nextCursor,
		HasMore: page.HasMore,
		Pruned:  pruned,
	}, nil
}

// serverCapabilitiesDeletedSinceRetentionDays returns the advertised retention horizon (days) for
// the deletedSince feed, sourced from config. Kept as a small helper so the ServerCapabilities
// resolver in resolver.go stays declarative. A negative configured value is normalised to 0
// (retention disabled / tombstones kept indefinitely) for the advertised contract.
func serverCapabilitiesDeletedSinceRetentionDays() int {
	days := config.GetInstance().GetDeletedSinceRetentionDays()
	if days < 0 {
		return 0
	}
	return days
}
