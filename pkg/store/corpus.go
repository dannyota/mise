package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	pgxvec "github.com/pgvector/pgvector-go/pgx"

	"danny.vn/mise/pkg/corpus"
)

// Corpus is the per-corpus store: schema-qualified reads/writes for one
// corpus.Descriptor's document/section/amendment_event tables (schema-per-
// corpus — migrations/002_document_tables.sql). Reads that must respect RLS
// (GetDocument) live in corpus_read.go; this file is the write path.
type Corpus struct {
	pool   *pgxpool.Pool
	schema string // raw, e.g. "vn_reg" — validated once by NewCorpus against corpus.All()
}

// NewCorpus returns a Corpus bound to desc's schema. It rejects any
// desc.SchemaName that isn't one of the registered corpus schemas
// (corpus.All()) — schema names are spliced directly into every query this
// type issues, so this is the one place that has to validate them; callers
// must build desc via corpus.Get/corpus.All, never from raw request input.
func NewCorpus(pool *pgxpool.Pool, desc corpus.Descriptor) (*Corpus, error) {
	registered := false
	for _, d := range corpus.All() {
		if d.SchemaName == desc.SchemaName {
			registered = true
			break
		}
	}
	if !registered {
		return nil, fmt.Errorf("store: %q is not a registered corpus schema", desc.SchemaName)
	}
	return &Corpus{pool: pool, schema: desc.SchemaName}, nil
}

// qualify returns c's schema-qualified, quoted table identifier
// (`"schema"."table"`), safe to splice into a query string built by string
// concatenation. c.schema can only be set by NewCorpus, which already
// validated it against the corpus registry, so every raw-SQL query below
// routes through this to turn it into SQL. The bulk-insert paths (CopyFrom)
// use c.schema directly through a pgx.Identifier instead — same validated
// field, just handed to pgx's own quoting rather than string concatenation.
func (c *Corpus) qualify(table string) string {
	return pgx.Identifier{c.schema, table}.Sanitize()
}

// rowQuerier is satisfied by both *pgxpool.Pool and pgx.Tx — findByColumn
// runs the same lookup whether or not it's inside a transaction.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// findByColumn is the shared select-id-by-natural-key lookup behind
// findExisting and FindDocIDByNumber. column is always a fixed literal from
// those callers, never caller input.
func (c *Corpus) findByColumn(ctx context.Context, q rowQuerier, column, value string) (uuid.UUID, bool, error) {
	query := `SELECT id FROM ` + c.qualify("document") + ` WHERE ` + column + ` = $1`
	var id uuid.UUID
	err := q.QueryRow(ctx, query, value).Scan(&id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return uuid.UUID{}, false, nil
	case err != nil:
		return uuid.UUID{}, false, fmt.Errorf("looking up document by %s: %w", column, err)
	}
	return id, true, nil
}

// FindDocIDByNumber returns the id of the document whose doc_number matches,
// and whether one was found.
func (c *Corpus) FindDocIDByNumber(ctx context.Context, docNumber string) (uuid.UUID, bool, error) {
	return c.findByColumn(ctx, c.pool, "doc_number", docNumber)
}

// TransitionValidity atomically reads docID's validity_status under a row
// lock, applies next to it, and writes the result back when it differs from
// what next saw. next must be pure. Returns the resulting status, whether or
// not it changed.
//
// This is the only safe way to change validity_status downstream of ingest:
// two ProcessDoc activities racing an amendment event onto the same target
// document must never interleave a plain read-in-Go-then-write — one's write
// would silently overwrite the other's, corrupting the current-law
// invariant. SELECT ... FOR UPDATE serializes them instead — the second
// transaction blocks until the first commits, then reads its result, so next
// always applies to the true current value.
func (c *Corpus) TransitionValidity(
	ctx context.Context, docID uuid.UUID, next func(current string) string,
) (string, error) {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("beginning validity transition for document %s: %w", docID, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := `SELECT validity_status FROM ` + c.qualify("document") + ` WHERE id = $1 FOR UPDATE`
	var cur string
	err = tx.QueryRow(ctx, q, docID).Scan(&cur)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return "", fmt.Errorf("transitioning validity for document %s: %w (%w)", docID, ErrDocumentNotFound, err)
	case err != nil:
		return "", fmt.Errorf("transitioning validity for document %s: %w", docID, err)
	}

	n := next(cur)
	if n != cur {
		updateQ := `UPDATE ` + c.qualify("document") + ` SET validity_status = $2, updated_at = now() WHERE id = $1`
		if _, err := tx.Exec(ctx, updateQ, docID, n); err != nil {
			return "", fmt.Errorf("updating validity for document %s: %w", docID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("committing validity transition for document %s: %w", docID, err)
	}
	return n, nil
}

// documentInsertCols, documentInsertPlaceholders, and documentUpdateSet
// share one column order with documentArgs, so the insert and update paths
// in UpsertDocument stay in sync by construction.
const (
	documentInsertCols = `corpus_id, title, doc_number, citation_scheme, citation_path, language,
		validity_status, issuing_authority, signer_name, version, source_url, source_system,
		content_type, access_tier, issued_date, effective_date, expiry_date, ingest_run_id, observed_at`
	documentInsertPlaceholders = `$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19`
	documentUpdateSet          = `corpus_id=$2, title=$3, doc_number=$4, citation_scheme=$5, citation_path=$6,
		language=$7, validity_status=$8, issuing_authority=$9, signer_name=$10, version=$11,
		source_url=$12, source_system=$13, content_type=$14, access_tier=$15, issued_date=$16,
		effective_date=$17, expiry_date=$18, ingest_run_id=$19, observed_at=$20, updated_at=now()`
)

// UpsertDocument resolves d to an existing row (by doc_number, then by
// source_url) and updates it, or inserts a new row when neither matches. It
// returns the row's id either way.
//
// Select-then-insert races under concurrency: two callers (e.g. mise's vbpl
// and vanban ingest sources, which can carry the same VN doc_number) can
// both run upsertOnce, both see "not found", and both attempt the insert.
// The loser fails migration 006's partial unique index on doc_number/
// source_url with SQLSTATE 23505 (unique_violation) because the winner
// already committed by the time the loser's insert is evaluated. Rather
// than surface that race as an error, UpsertDocument retries once, in a
// fresh transaction (the first is aborted by the failed insert): findExisting
// is re-run and, once it resolves the winner's now-committed row, the update
// path converges onto it. If the retry still finds nothing — a genuinely
// unexpected state, not the ordinary race — the original insert error is
// returned instead of masking it behind a confusing second failure.
func (c *Corpus) UpsertDocument(ctx context.Context, d Document) (uuid.UUID, error) {
	id, err := c.upsertOnce(ctx, d)
	if err == nil || !isUniqueViolation(err) {
		return id, err
	}

	retryID, found, retryErr := c.retryUpdateAfterCollision(ctx, d)
	switch {
	case retryErr != nil:
		return uuid.UUID{}, retryErr
	case !found:
		return uuid.UUID{}, err
	default:
		return retryID, nil
	}
}

// upsertOnce runs one find-then-insert-or-update pass in its own
// transaction — UpsertDocument's normal path, and also its post-collision
// retry's insert-side attempt (see retryUpdateAfterCollision).
func (c *Corpus) upsertOnce(ctx context.Context, d Document) (uuid.UUID, error) {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("beginning document upsert: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id, found, err := c.findExisting(ctx, tx, d)
	if err != nil {
		return uuid.UUID{}, err
	}

	if found {
		if err := c.updateDocument(ctx, tx, id, d); err != nil {
			return uuid.UUID{}, err
		}
	} else {
		id, err = c.insertDocument(ctx, tx, d)
		if err != nil {
			return uuid.UUID{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.UUID{}, fmt.Errorf("committing document upsert: %w", err)
	}
	return id, nil
}

// retryUpdateAfterCollision re-resolves d's natural key in a fresh
// transaction after upsertOnce lost an insert race, and updates that row if
// found. Called only from UpsertDocument's unique_violation retry path.
func (c *Corpus) retryUpdateAfterCollision(ctx context.Context, d Document) (uuid.UUID, bool, error) {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return uuid.UUID{}, false, fmt.Errorf("beginning upsert collision retry: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id, found, err := c.findExisting(ctx, tx, d)
	if err != nil || !found {
		return uuid.UUID{}, false, err
	}
	if err := c.updateDocument(ctx, tx, id, d); err != nil {
		return uuid.UUID{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.UUID{}, false, fmt.Errorf("committing upsert collision retry: %w", err)
	}
	return id, true, nil
}

// isUniqueViolation reports whether err is Postgres SQLSTATE 23505 — a
// losing concurrent insert against migration 006's partial unique index on
// document.doc_number/source_url.
func isUniqueViolation(err error) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && pgErr.Code == "23505"
}

// findExisting looks up d's existing row by doc_number, then by
// source_url — the two natural keys migration 006's partial unique indexes
// enforce — and reports whether either lookup found a row.
func (c *Corpus) findExisting(ctx context.Context, tx pgx.Tx, d Document) (uuid.UUID, bool, error) {
	if d.DocNumber != "" {
		id, ok, err := c.findByColumn(ctx, tx, "doc_number", d.DocNumber)
		if err != nil || ok {
			return id, ok, err
		}
	}
	if d.SourceURL != "" {
		return c.findByColumn(ctx, tx, "source_url", d.SourceURL)
	}
	return uuid.UUID{}, false, nil
}

// insertDocument inserts d as a new row and returns its generated id.
func (c *Corpus) insertDocument(ctx context.Context, tx pgx.Tx, d Document) (uuid.UUID, error) {
	q := `INSERT INTO ` + c.qualify("document") + ` (` + documentInsertCols + `)
		VALUES (` + documentInsertPlaceholders + `)
		RETURNING id`
	var id uuid.UUID
	if err := tx.QueryRow(ctx, q, documentArgs(d)...).Scan(&id); err != nil {
		return uuid.UUID{}, fmt.Errorf("inserting document: %w", err)
	}
	return id, nil
}

// updateDocument overwrites id's row with d's fields.
func (c *Corpus) updateDocument(ctx context.Context, tx pgx.Tx, id uuid.UUID, d Document) error {
	q := `UPDATE ` + c.qualify("document") + ` SET ` + documentUpdateSet + ` WHERE id=$1`
	args := append([]any{id}, documentArgs(d)...)
	if _, err := tx.Exec(ctx, q, args...); err != nil {
		return fmt.Errorf("updating document %s: %w", id, err)
	}
	return nil
}

// documentArgs returns d's writable columns in the fixed order
// documentInsertCols/documentUpdateSet expect. Empty DocNumber/SourceURL and
// other optional text fields become SQL NULL: migration 006's unique
// indexes on doc_number/source_url are partial (`WHERE ... IS NOT NULL`), so
// storing "" instead of NULL would collide across every document missing
// one of these fields.
func documentArgs(d Document) []any {
	return []any{
		d.CorpusID, d.Title, nullIfEmpty(d.DocNumber), nullIfEmpty(d.CitationScheme), nullIfEmpty(d.CitationPath),
		d.Language, d.ValidityStatus, nullIfEmpty(d.IssuingAuthority), nullIfEmpty(d.SignerName), nullIfEmpty(d.Version),
		nullIfEmpty(d.SourceURL), nullIfEmpty(d.SourceSystem), nullIfEmpty(d.ContentType), d.AccessTier,
		d.IssuedDate, d.EffectiveDate, d.ExpiryDate, d.IngestRunID, d.ObservedAt,
	}
}

// nullIfEmpty returns s as a query argument, or nil (SQL NULL) when s is
// empty — mise's "not set" convention for optional text columns.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// sectionColumns is the column order ReplaceSections' bulk insert writes;
// body_tsv (migration 006) is a generated column and never appears here.
var sectionColumns = []string{
	"document_id", "corpus_id", "citation_path", "heading_path", "position", "body",
	"embedding", "validity_status", "access_tier", "effective_date",
}

// ReplaceSections atomically replaces every section of docID with secs: a
// delete followed by a bulk insert (pgx.CopyFrom), in one transaction. Every
// non-nil Embedding must be exactly 1536-d (the shared embed space,
// pkg/corpus's sharedEmbed) — checked before any write happens, so a batch
// with one bad embedding never touches the database. Each written row's
// position is secs' slice index at write time (any Section.Position the
// caller set is ignored), so GetDocument can read sections back in exactly
// this order.
func (c *Corpus) ReplaceSections(ctx context.Context, docID uuid.UUID, secs []Section) error {
	for i, s := range secs {
		if s.Embedding != nil && len(s.Embedding) != 1536 {
			return fmt.Errorf("section %d for document %s: embedding has %d dimensions, want 1536",
				i, docID, len(s.Embedding))
		}
	}

	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning section replace: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := pgxvec.RegisterTypes(ctx, tx.Conn()); err != nil {
		return fmt.Errorf("registering pgvector types: %w", err)
	}

	deleteQ := `DELETE FROM ` + c.qualify("section") + ` WHERE document_id = $1`
	if _, err := tx.Exec(ctx, deleteQ, docID); err != nil {
		return fmt.Errorf("deleting existing sections for document %s: %w", docID, err)
	}

	if len(secs) > 0 {
		src := pgx.CopyFromSlice(len(secs), func(i int) ([]any, error) {
			s := secs[i]
			var emb any
			if s.Embedding != nil {
				emb = pgvector.NewVector(s.Embedding)
			}
			return []any{
				docID, s.CorpusID, nullIfEmpty(s.CitationPath), nullIfEmpty(s.HeadingPath), i, s.Body,
				emb, s.ValidityStatus, s.AccessTier, s.EffectiveDate,
			}, nil
		})
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{c.schema, "section"}, sectionColumns, src); err != nil {
			return fmt.Errorf("bulk inserting sections for document %s: %w", docID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing section replace for document %s: %w", docID, err)
	}
	return nil
}

// InsertAmendmentEvents inserts evs, skipping (ON CONFLICT DO NOTHING) any
// event that exactly matches one already recorded — migration 007's
// per-schema unique index on (target_doc_id, amending_doc_id, clause,
// event_date). A changed document re-indexed by the ingest pipeline
// re-derives its relation events from the source's current Relations and
// re-inserts them (applyRelations has no read-before-write check); without
// this dedup, that duplicates the row on every genuine re-index whose
// relations haven't actually changed. Batched over one transaction so a
// partial failure never leaves some of evs committed and others not — the
// same all-or-nothing guarantee the prior pgx.CopyFrom implementation had
// (COPY can't express ON CONFLICT, hence the switch to a batch of inserts).
func (c *Corpus) InsertAmendmentEvents(ctx context.Context, evs []AmendmentEvent) error {
	if len(evs) == 0 {
		return nil
	}
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning amendment event insert: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := `INSERT INTO ` + c.qualify("amendment_event") +
		` (target_doc_id, amending_doc_id, clause, event_date, kind) VALUES ($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`
	batch := &pgx.Batch{}
	for _, e := range evs {
		batch.Queue(q, e.TargetDocID, e.AmendingDocID, nullIfEmpty(e.Clause), e.EventDate, e.Kind)
	}
	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("bulk inserting amendment events: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing amendment event insert: %w", err)
	}
	return nil
}

// DueEvents returns every amendment event of this corpus schema whose
// event_date is at or before now, ordered by event_date — the candidate set
// for a periodic sweep (pkg/pipeline's ApplyDueEvents) that re-drives
// TransitionValidity for events whose target was not yet in the store, or
// whose future date had not yet arrived, the last time an amending
// document's indexing pass tried to apply them. It deliberately does not
// pre-filter by "would this actually change the target's current status" —
// target_doc_id is NOT NULL REFERENCES document(id), so every row's target
// exists, and TransitionValidity is a no-op write when the status doesn't
// change, so the caller can safely call it for every row this returns.
func (c *Corpus) DueEvents(ctx context.Context, now time.Time) ([]AmendmentEvent, error) {
	q := `SELECT target_doc_id, amending_doc_id, clause, event_date, kind FROM ` + c.qualify("amendment_event") +
		` WHERE event_date <= $1 ORDER BY event_date`

	rows, err := c.pool.Query(ctx, q, now)
	if err != nil {
		return nil, fmt.Errorf("querying due amendment events: %w", err)
	}
	defer rows.Close()

	var out []AmendmentEvent
	for rows.Next() {
		var e AmendmentEvent
		var clause *string
		if err := rows.Scan(&e.TargetDocID, &e.AmendingDocID, &clause, &e.EventDate, &e.Kind); err != nil {
			return nil, fmt.Errorf("scanning due amendment event row: %w", err)
		}
		e.Clause = derefOr(clause)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading due amendment event rows: %w", err)
	}
	return out, nil
}
