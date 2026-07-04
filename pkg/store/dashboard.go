package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DashboardStats holds the aggregate counts the dashboard summary endpoint
// returns: open conflicts, staleness alerts, and the review queue depth
// (unpromoted edges awaiting human review).
type DashboardStats struct {
	OpenConflicts    int
	StalenessAlerts  int
	ReviewQueueDepth int
}

// DashboardStore is the dashboard summary read path. Reads run inside a
// SET LOCAL ROLE transaction scoped to the caller's resolved tier.
type DashboardStore struct {
	pool *pgxpool.Pool
}

// NewDashboardStore returns a DashboardStore backed by pool.
func NewDashboardStore(pool *pgxpool.Pool) *DashboardStore {
	return &DashboardStore{pool: pool}
}

// GetStats returns aggregate dashboard counts scoped to role's RLS visibility:
// open conflict findings, open staleness findings, and unpromoted edges.
func (s *DashboardStore) GetStats(ctx context.Context, role string) (DashboardStats, error) {
	validRole, err := resolveRole(role)
	if err != nil {
		return DashboardStats{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return DashboardStats{}, fmt.Errorf("beginning GetStats read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	roleQ := `SET LOCAL ROLE ` + pgx.Identifier{validRole}.Sanitize()
	if _, err := tx.Exec(ctx, roleQ); err != nil {
		return DashboardStats{}, fmt.Errorf("setting local role %q: %w", validRole, err)
	}

	var stats DashboardStats

	err = tx.QueryRow(ctx,
		`SELECT count(*) FROM graph.finding WHERE kind = 'conflict' AND status = 'open'`,
	).Scan(&stats.OpenConflicts)
	if err != nil {
		return DashboardStats{}, fmt.Errorf("counting open conflicts: %w", err)
	}

	err = tx.QueryRow(ctx,
		`SELECT count(*) FROM graph.finding WHERE kind = 'staleness' AND status = 'open'`,
	).Scan(&stats.StalenessAlerts)
	if err != nil {
		return DashboardStats{}, fmt.Errorf("counting staleness alerts: %w", err)
	}

	err = tx.QueryRow(ctx,
		`SELECT count(*) FROM graph.relation_edge WHERE promoted = false`,
	).Scan(&stats.ReviewQueueDepth)
	if err != nil {
		return DashboardStats{}, fmt.Errorf("counting review queue depth: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return DashboardStats{}, fmt.Errorf("committing GetStats read: %w", err)
	}
	return stats, nil
}
