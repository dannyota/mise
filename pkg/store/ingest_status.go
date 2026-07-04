package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// IngestStatus holds the latest ingest run state for a corpus, as read from
// ingest.run. Used by the corpus admin REST endpoint (pkg/httpapi/corpus_admin.go).
type IngestStatus struct {
	Status        string // "healthy" | "ingesting" | "error"
	LastIngest    string // RFC3339 timestamp of last completed run, or ""
	DocumentCount int
	ErrorMessage  string
}

// GetIngestStatus returns the latest ingest run status for corpusID by reading
// the ingest.run table. Similar to dashboard.go's corpusIngestStatus but
// exposed as a public function for the corpus admin endpoint, including the
// error message and document count.
func GetIngestStatus(ctx context.Context, pool *pgxpool.Pool, corpusID string) (IngestStatus, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return IngestStatus{}, fmt.Errorf("beginning ingest status read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var info IngestStatus

	// Latest run overall for status determination.
	var latestStatus string
	var lastError *string
	err = tx.QueryRow(ctx,
		`SELECT COALESCE(status, 'running'), last_error FROM ingest.run
		 WHERE corpus_id = $1 ORDER BY started_at DESC LIMIT 1`, corpusID,
	).Scan(&latestStatus, &lastError)
	if errors.Is(err, pgx.ErrNoRows) {
		info.Status = "healthy"
	} else if err != nil {
		return IngestStatus{}, fmt.Errorf("reading latest run for %s: %w", corpusID, err)
	} else {
		switch latestStatus {
		case "running":
			info.Status = "ingesting"
		case "failed":
			info.Status = "error"
			if lastError != nil {
				info.ErrorMessage = *lastError
			}
		default:
			info.Status = "healthy"
		}
	}

	// Latest completed run for the last_ingest timestamp.
	err = tx.QueryRow(ctx,
		`SELECT to_char(started_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		 FROM ingest.run
		 WHERE corpus_id = $1 AND status = 'completed'
		 ORDER BY started_at DESC LIMIT 1`, corpusID,
	).Scan(&info.LastIngest)
	if errors.Is(err, pgx.ErrNoRows) {
		info.LastIngest = ""
	} else if err != nil {
		info.LastIngest = ""
	}

	// Document count across all corpus schemas matching this corpus_id.
	// Uses the ingest.doc_ledger since it tracks all documents regardless of schema.
	var count int
	err = tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM ingest.doc_ledger WHERE corpus_id = $1 AND state = 'indexed'`, corpusID,
	).Scan(&count)
	if err == nil {
		info.DocumentCount = count
	}

	if err := tx.Commit(ctx); err != nil {
		return IngestStatus{}, fmt.Errorf("committing ingest status read: %w", err)
	}
	return info, nil
}
