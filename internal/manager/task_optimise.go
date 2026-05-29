package manager

import (
	"context"
	"fmt"
	"time"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/job"
	"github.com/stashapp/stash/pkg/logger"
)

type Optimiser interface {
	Analyze(ctx context.Context) error
	Vacuum(ctx context.Context) error
}

// TombstonePruner prunes deletedSince tombstones older than the retention horizon. The optimise
// job runs this so the NG deletedSince feed stays bounded; it is config-gated and only runs as
// part of the user-triggered/scheduled database-optimise maintenance task (never aggressively).
type TombstonePruner interface {
	PruneDeletedRecords(ctx context.Context, before time.Time) (int64, error)
}

type OptimiseDatabaseJob struct {
	Optimiser Optimiser
	Pruner    TombstonePruner
}

func (j *OptimiseDatabaseJob) Execute(ctx context.Context, progress *job.Progress) error {
	logger.Info("Optimising database")
	progress.SetTotal(3)

	start := time.Now()

	var err error

	progress.ExecuteTask("Pruning deletion tombstones", func() {
		err = j.pruneTombstones(ctx)
		progress.Increment()
	})
	if job.IsCancelled(ctx) {
		logger.Info("Stopping due to user request")
		return nil
	}
	if err != nil {
		// A prune failure should not abort the optimise; log and continue.
		logger.Errorf("error pruning deletion tombstones: %v", err)
	}

	progress.ExecuteTask("Analyzing database", func() {
		err = j.Optimiser.Analyze(ctx)
		progress.Increment()
	})
	if job.IsCancelled(ctx) {
		logger.Info("Stopping due to user request")
		return nil
	}
	if err != nil {
		return fmt.Errorf("error analyzing database: %w", err)
	}

	progress.ExecuteTask("Vacuuming database", func() {
		err = j.Optimiser.Vacuum(ctx)
		progress.Increment()
	})
	if job.IsCancelled(ctx) {
		logger.Info("Stopping due to user request")
		return nil
	}
	if err != nil {
		return fmt.Errorf("error vacuuming database: %w", err)
	}

	elapsed := time.Since(start)
	logger.Infof("Finished optimising database after %s", elapsed)
	return nil
}

// pruneTombstones removes deletedSince tombstones older than the configured retention window.
// A retention of <= 0 (or a nil pruner) disables pruning, so the feed is kept indefinitely.
func (j *OptimiseDatabaseJob) pruneTombstones(ctx context.Context) error {
	if j.Pruner == nil {
		return nil
	}

	days := config.GetInstance().GetDeletedSinceRetentionDays()
	if days <= 0 {
		logger.Debug("Tombstone retention disabled (deleted_since_retention_days <= 0); skipping prune")
		return nil
	}

	before := time.Now().UTC().AddDate(0, 0, -days)
	n, err := j.Pruner.PruneDeletedRecords(ctx, before)
	if err != nil {
		return err
	}
	if n > 0 {
		logger.Infof("Pruned %d deletion tombstone(s) older than %d days", n, days)
	}
	return nil
}
