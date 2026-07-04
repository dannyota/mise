package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"danny.vn/mise/pkg/corpus"
)

// CorpusStats holds per-corpus aggregates computed by GetStats.
type CorpusStats struct {
	CorpusID      string
	DocumentCount int
	LastIngest    string // RFC3339 timestamp or "" if no completed run.
	Status        string // "healthy", "ingesting", or "error".
}

// DashboardStats holds the aggregate counts the dashboard summary endpoint
// returns: coverage percentage, open conflicts, staleness alerts, the review
// queue depth (unpromoted edges awaiting human review), and per-corpus status.
type DashboardStats struct {
	CoveragePct      float64
	OpenConflicts    int
	StalenessAlerts  int
	ReviewQueueDepth int
	Corpora          []CorpusStats
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
// coverage percentage, open conflict findings, open staleness findings,
// unpromoted edges, and per-corpus document counts + ingest status.
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

	// Open conflicts.
	err = tx.QueryRow(ctx,
		`SELECT count(*) FROM graph.finding WHERE kind = 'conflict' AND status = 'open'`,
	).Scan(&stats.OpenConflicts)
	if err != nil {
		return DashboardStats{}, fmt.Errorf("counting open conflicts: %w", err)
	}

	// Staleness alerts.
	err = tx.QueryRow(ctx,
		`SELECT count(*) FROM graph.finding WHERE kind = 'staleness' AND status = 'open'`,
	).Scan(&stats.StalenessAlerts)
	if err != nil {
		return DashboardStats{}, fmt.Errorf("counting staleness alerts: %w", err)
	}

	// Review queue depth: unpromoted edges awaiting human review.
	err = tx.QueryRow(ctx,
		`SELECT count(*) FROM graph.relation_edge WHERE promoted = false`,
	).Scan(&stats.ReviewQueueDepth)
	if err != nil {
		return DashboardStats{}, fmt.Errorf("counting review queue depth: %w", err)
	}

	// Coverage percentage: promoted 'satisfies' edges / obligation-target nodes.
	// Obligation targets are doc_refs pointing to law corpora (vn_reg, my_reg).
	stats.CoveragePct, err = computeCoverage(ctx, tx)
	if err != nil {
		return DashboardStats{}, fmt.Errorf("computing coverage: %w", err)
	}

	// Per-corpus document count (tier-filtered via the active SET LOCAL ROLE).
	allCorpora := corpus.All()
	stats.Corpora = make([]CorpusStats, len(allCorpora))
	for i, desc := range allCorpora {
		cs := CorpusStats{CorpusID: string(desc.ID)}
		cs.DocumentCount = corpusDocCount(ctx, tx, desc.SchemaName)
		cs.LastIngest, cs.Status = corpusIngestStatus(ctx, tx, string(desc.ID))
		stats.Corpora[i] = cs
	}

	if err := tx.Commit(ctx); err != nil {
		return DashboardStats{}, fmt.Errorf("committing GetStats read: %w", err)
	}
	return stats, nil
}

// computeCoverage returns the ratio of promoted 'satisfies' edges to the total
// number of doc_ref nodes targeting law corpora (vn_reg, my_reg). Returns 0.0
// when no obligation targets exist.
func computeCoverage(ctx context.Context, tx pgx.Tx) (float64, error) {
	var totalObligations int
	err := tx.QueryRow(ctx,
		`SELECT count(*) FROM graph.doc_ref WHERE corpus_id IN ('vn-reg', 'my-reg')`,
	).Scan(&totalObligations)
	if err != nil {
		return 0, err
	}
	if totalObligations == 0 {
		return 0, nil
	}

	var promoted int
	err = tx.QueryRow(ctx,
		`SELECT count(*) FROM graph.relation_edge
		  WHERE edge_type = 'satisfies' AND promoted = true`,
	).Scan(&promoted)
	if err != nil {
		return 0, err
	}

	pct := float64(promoted) / float64(totalObligations) * 100
	if pct > 100 {
		pct = 100
	}
	return pct, nil
}

// corpusDocCount returns the document count for a corpus schema. Returns 0 on
// permission denied (the role lacks GRANT USAGE on that schema).
func corpusDocCount(ctx context.Context, tx pgx.Tx, schema string) int {
	q := `SELECT count(*) FROM ` + pgx.Identifier{schema, "document"}.Sanitize()
	var n int
	if err := tx.QueryRow(ctx, q).Scan(&n); err != nil {
		// Permission denied → fold to 0 (same approach as store.Search).
		return 0
	}
	return n
}

// corpusIngestStatus returns (lastIngest, status) for a corpus by reading the
// ingest.run table. lastIngest is the RFC3339 timestamp of the most recent
// completed run, or "" if none. status is "ingesting" if the latest run is
// still running, "error" if the latest run failed, or "healthy" otherwise.
func corpusIngestStatus(ctx context.Context, tx pgx.Tx, corpusID string) (string, string) {
	// Latest run overall (by started_at desc) for status.
	var latestStatus string
	err := tx.QueryRow(ctx,
		`SELECT status FROM ingest.run
		  WHERE corpus_id = $1 ORDER BY started_at DESC LIMIT 1`, corpusID,
	).Scan(&latestStatus)
	if err != nil {
		// No runs or permission denied → healthy with no last ingest.
		return "", "healthy"
	}

	// Latest completed run for the last_ingest timestamp.
	var lastIngest string
	err = tx.QueryRow(ctx,
		`SELECT to_char(started_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		  FROM ingest.run
		  WHERE corpus_id = $1 AND status = 'completed'
		  ORDER BY started_at DESC LIMIT 1`, corpusID,
	).Scan(&lastIngest)
	if errors.Is(err, pgx.ErrNoRows) {
		lastIngest = ""
	} else if err != nil {
		lastIngest = ""
	}

	switch latestStatus {
	case "running":
		return lastIngest, "ingesting"
	case "failed":
		return lastIngest, "error"
	default:
		return lastIngest, "healthy"
	}
}

// isDashboardPermissionDenied reports whether err is Postgres SQLSTATE 42501.
// Kept package-private; mirrors isPermissionDenied in search.go.
var _ = isDashboardPermissionDenied // suppress unused lint if helper not called inline

func isDashboardPermissionDenied(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42501" {
		return true
	}
	return false
}
