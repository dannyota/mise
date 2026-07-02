package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"danny.vn/mise/pkg/corpus"
)

// epoch is the zero watermark ingest.cursor rows default to (the migration's
// `DEFAULT 'epoch'`); Cursor.Get returns it for a corpus/source/keyword that
// has never been Set.
var epoch = time.Unix(0, 0).UTC()

// Ledger tracks per-document ingest bookkeeping in ingest.doc_ledger: the
// content hash last observed for a source document, its lifecycle state, and
// the document row it resolves to once normalized.
type Ledger struct {
	pool *pgxpool.Pool
}

// NewLedger returns a Ledger backed by pool.
func NewLedger(pool *pgxpool.Pool) *Ledger {
	return &Ledger{pool: pool}
}

// Upsert records a document observed during discovery/fetch. corpusID,
// sourceID, and externalID identify the row (the table's primary key); hash
// is the content hash observed this run; state is the caller's lifecycle
// label (e.g. "discovered", "fetched"). Calling Upsert again for the same key
// refreshes content_hash, state, and the observed_at/updated_at timestamps.
func (l *Ledger) Upsert(ctx context.Context, corpusID corpus.ID, sourceID, externalID, hash, state string) error {
	const q = `
INSERT INTO ingest.doc_ledger (corpus_id, source_id, external_id, content_hash, state, observed_at, updated_at)
VALUES ($1, $2, $3, $4, $5, now(), now())
ON CONFLICT (corpus_id, source_id, external_id) DO UPDATE
SET content_hash = EXCLUDED.content_hash,
    state = EXCLUDED.state,
    observed_at = now(),
    updated_at = now()`

	if _, err := l.pool.Exec(ctx, q, string(corpusID), sourceID, externalID, hash, state); err != nil {
		return fmt.Errorf("upserting doc ledger %s/%s/%s: %w", corpusID, sourceID, externalID, err)
	}
	return nil
}

// Unchanged reports whether hash matches the content_hash already recorded
// for corpusID/sourceID/externalID. A key that hasn't been observed yet
// returns false, nil — an unseen document is never "unchanged".
func (l *Ledger) Unchanged(ctx context.Context, corpusID corpus.ID, sourceID, externalID, hash string) (bool, error) {
	const q = `SELECT content_hash FROM ingest.doc_ledger WHERE corpus_id = $1 AND source_id = $2 AND external_id = $3`

	var stored string
	err := l.pool.QueryRow(ctx, q, string(corpusID), sourceID, externalID).Scan(&stored)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("reading doc ledger %s/%s/%s: %w", corpusID, sourceID, externalID, err)
	}
	return stored == hash, nil
}

// SetState updates a ledger row's lifecycle state and last error. Pass "" for
// errMsg to clear last_error (e.g. on a transition that succeeded).
func (l *Ledger) SetState(ctx context.Context, corpusID corpus.ID, sourceID, externalID, state, errMsg string) error {
	const q = `
UPDATE ingest.doc_ledger
SET state = $4, last_error = $5, updated_at = now()
WHERE corpus_id = $1 AND source_id = $2 AND external_id = $3`

	var lastError *string
	if errMsg != "" {
		lastError = new(errMsg)
	}

	if _, err := l.pool.Exec(ctx, q, string(corpusID), sourceID, externalID, state, lastError); err != nil {
		return fmt.Errorf("setting doc ledger state %s/%s/%s: %w", corpusID, sourceID, externalID, err)
	}
	return nil
}

// LinkDocument records the document row a ledger entry resolved to once
// normalize succeeds.
func (l *Ledger) LinkDocument(
	ctx context.Context, corpusID corpus.ID, sourceID, externalID string, docID uuid.UUID,
) error {
	const q = `
UPDATE ingest.doc_ledger
SET document_id = $4, updated_at = now()
WHERE corpus_id = $1 AND source_id = $2 AND external_id = $3`

	if _, err := l.pool.Exec(ctx, q, string(corpusID), sourceID, externalID, docID); err != nil {
		return fmt.Errorf("linking document for %s/%s/%s: %w", corpusID, sourceID, externalID, err)
	}
	return nil
}

// CursorStore tracks the per-source discovery watermark in ingest.cursor.
// Construct one with Cursor.
type CursorStore struct {
	pool *pgxpool.Pool
}

// Cursor returns a CursorStore backed by pool.
func Cursor(pool *pgxpool.Pool) *CursorStore {
	return &CursorStore{pool: pool}
}

// Get returns the stored watermark for corpusID/sourceID/keyword, or the
// epoch (1970-01-01 UTC) when no cursor row exists yet — the migration's
// column default for a corpus/source/keyword not yet discovered.
func (c *CursorStore) Get(ctx context.Context, corpusID corpus.ID, sourceID, keyword string) (time.Time, error) {
	const q = `SELECT watermark FROM ingest.cursor WHERE corpus_id = $1 AND source_id = $2 AND keyword = $3`

	var watermark time.Time
	err := c.pool.QueryRow(ctx, q, string(corpusID), sourceID, keyword).Scan(&watermark)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return epoch, nil
	case err != nil:
		return time.Time{}, fmt.Errorf("reading cursor %s/%s/%q: %w", corpusID, sourceID, keyword, err)
	}
	return watermark, nil
}

// Set upserts the watermark for corpusID/sourceID/keyword.
func (c *CursorStore) Set(
	ctx context.Context, corpusID corpus.ID, sourceID, keyword string, watermark time.Time,
) error {
	const q = `
INSERT INTO ingest.cursor (corpus_id, source_id, keyword, watermark, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (corpus_id, source_id, keyword) DO UPDATE
SET watermark = EXCLUDED.watermark,
    updated_at = now()`

	if _, err := c.pool.Exec(ctx, q, string(corpusID), sourceID, keyword, watermark); err != nil {
		return fmt.Errorf("setting cursor %s/%s/%q: %w", corpusID, sourceID, keyword, err)
	}
	return nil
}

// StartRun inserts a new ingest.run row for corpusID and returns its id.
func StartRun(ctx context.Context, pool *pgxpool.Pool, corpusID corpus.ID) (uuid.UUID, error) {
	const q = `INSERT INTO ingest.run (corpus_id) VALUES ($1) RETURNING id`

	var id uuid.UUID
	if err := pool.QueryRow(ctx, q, string(corpusID)).Scan(&id); err != nil {
		return uuid.UUID{}, fmt.Errorf("starting ingest run for %s: %w", corpusID, err)
	}
	return id, nil
}

// FinishRun marks an ingest.run row finished with the given status and stats
// (arbitrary counters/details, stored as jsonb). A nil stats map is stored as
// an empty JSON object, matching the column's default.
func FinishRun(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, status string, stats map[string]any) error {
	if stats == nil {
		stats = map[string]any{}
	}
	statsJSON, err := json.Marshal(stats)
	if err != nil {
		return fmt.Errorf("marshaling run stats for %s: %w", id, err)
	}

	const q = `
UPDATE ingest.run
SET finished_at = now(), status = $2, stats = $3::jsonb
WHERE id = $1`

	if _, err := pool.Exec(ctx, q, id, status, statsJSON); err != nil {
		return fmt.Errorf("finishing ingest run %s: %w", id, err)
	}
	return nil
}
