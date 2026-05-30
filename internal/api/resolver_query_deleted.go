package api

import (
	"context"
	"strconv"
	"time"

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
	reader := r.deletedRecordReader

	afterID := parseDeletedSinceAfter(after)

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
		page, err = reader.GetDeletedSincePage(ctx, since, afterID, lim)
		if err != nil {
			return err
		}
		minID, hasAny, err = reader.MinDeletedRecordCursor(ctx)
		return err
	}); err != nil {
		return nil, err
	}

	return buildDeletedSinceResult(page, afterID, minID, hasAny), nil
}

// parseDeletedSinceAfter decodes the opaque `after` cursor — the tombstone table's autoincrement id
// rendered as a decimal string — into an id, or nil when it is absent, empty, or malformed. A
// malformed cursor degrades to "no cursor" (the resolver then falls back to `since`/the beginning)
// rather than erroring, so a corrupted client watermark is non-fatal.
func parseDeletedSinceAfter(after *string) *int64 {
	if after == nil || *after == "" {
		return nil
	}
	id, err := strconv.ParseInt(*after, 10, 64)
	if err != nil {
		return nil
	}
	return &id
}

// buildDeletedSinceResult maps a store page to the GraphQL DeletedSinceResult. It converts each
// tombstone row, resolves the resume cursor (the page's last id when it advanced, otherwise the
// caller's own cursor so an empty page never resets the watermark to 0 and replays the whole feed),
// and sets `pruned` when the caller's cursor predates the oldest still-retained tombstone (so it must
// full-resync). minID/hasAny come from MinDeletedRecordCursor; with no tombstones at all nothing is
// pruned relative to the caller.
func buildDeletedSinceResult(page sqlite.DeletedSincePage, afterID *int64, minID int64, hasAny bool) *DeletedSinceResult {
	records := make([]*DeletedRecord, len(page.Records))
	for i, rec := range page.Records {
		records[i] = &DeletedRecord{
			Entity:    rec.RecordType,
			ID:        strconv.Itoa(rec.RecordID),
			DeletedAt: rec.DeletedAt,
			Cursor:    strconv.FormatInt(rec.Cursor, 10),
		}
	}

	nextCursor := ""
	switch {
	case page.NextCursor != 0:
		nextCursor = strconv.FormatInt(page.NextCursor, 10)
	case afterID != nil:
		nextCursor = strconv.FormatInt(*afterID, 10)
	}

	pruned := afterID != nil && hasAny && *afterID < minID

	return &DeletedSinceResult{
		Records: records,
		Cursor:  nextCursor,
		HasMore: page.HasMore,
		Pruned:  pruned,
	}
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
