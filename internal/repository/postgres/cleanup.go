package postgres

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CleanupService handles periodic database cleanup tasks.
type CleanupService struct {
	pool *pgxpool.Pool
}

// NewCleanupService creates a new cleanup service.
func NewCleanupService(pool *pgxpool.Pool) *CleanupService {
	return &CleanupService{pool: pool}
}

// StartPeriodicCleanup runs cleanup tasks in the background.
// It removes soft-deleted records older than the retention period.
func (s *CleanupService) StartPeriodicCleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	slog.Info("Starting periodic database cleanup", "interval", interval)

	// Run initial cleanup
	s.runCleanup(ctx)

	for {
		select {
		case <-ticker.C:
			s.runCleanup(ctx)
		case <-ctx.Done():
			slog.Info("Stopping periodic cleanup")
			return
		}
	}
}

// runCleanup performs the actual cleanup operations.
func (s *CleanupService) runCleanup(ctx context.Context) {
	cleanupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Clean up soft-deleted members older than 30 days
	retentionPeriod := 30 * 24 * time.Hour
	deletedCount, err := s.CleanupOldDeletedMembers(cleanupCtx, retentionPeriod)
	if err != nil {
		slog.Error("Failed to cleanup old deleted members", "error", err)
		return
	}

	if deletedCount > 0 {
		slog.Info("Database cleanup completed", "deleted_records", deletedCount)
	}
}

// CleanupOldDeletedMembers removes soft-deleted group member records
// that are older than the specified retention period.
func (s *CleanupService) CleanupOldDeletedMembers(ctx context.Context, olderThan time.Duration) (int64, error) {
	query := `
		DELETE FROM group_members
		WHERE deleted_at IS NOT NULL
		  AND deleted_at < NOW() - $1::interval`

	result, err := s.pool.Exec(ctx, query, olderThan)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected(), nil
}

// CleanupOldWarnLogs removes warn logs older than the specified retention period.
// Useful for compliance and keeping the database size manageable.
func (s *CleanupService) CleanupOldWarnLogs(ctx context.Context, olderThan time.Duration) (int64, error) {
	query := `
		DELETE FROM warn_logs
		WHERE created_at < NOW() - $1::interval`

	result, err := s.pool.Exec(ctx, query, olderThan)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected(), nil
}

// VacuumAnalyze runs VACUUM ANALYZE to optimize database performance.
// Should be run periodically to reclaim space and update statistics.
func (s *CleanupService) VacuumAnalyze(ctx context.Context) error {
	tables := []string{"group_members", "warn_logs", "groups", "users"}

	for _, table := range tables {
		_, err := s.pool.Exec(ctx, "VACUUM ANALYZE "+table)
		if err != nil {
			slog.Warn("Failed to vacuum table", "table", table, "error", err)
			// Continue with other tables even if one fails
		}
	}

	slog.Info("Database vacuum completed")
	return nil
}
