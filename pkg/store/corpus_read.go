package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pgvector/pgvector-go"
	pgxvec "github.com/pgvector/pgvector-go/pgx"
)

// ErrDocumentNotFound is returned by GetDocument when docID has no row
// visible to the acting role. "Doesn't exist" and "exists, but this role
// can't see it" are deliberately indistinguishable: an RLS-scoped SELECT can
// fail either by returning zero rows (the row's access_tier fails the
// policy predicate) or by erroring outright with SQLSTATE 42501,
// insufficient_privilege (the role has no GRANT USAGE on a confidential
// corpus's schema at all — migrations/004_rls_roles.sql). Both collapse to
// this one error so neither leaks which case happened.
var ErrDocumentNotFound = errors.New("document not found")

// DocumentDetail is the full read shape GetDocument returns.
type DocumentDetail struct {
	Doc      Document
	Sections []Section
	Events   []AmendmentEvent
}

// GetDocument reads docID and its sections/events in one transaction. When
// role is non-empty, the transaction runs SET LOCAL ROLE first, so every
// read in it is subject to that role's RLS policies (migrations/004). role
// must always come from the mise_public/mise_group/mise_local set the
// caller's resolved access tier maps to — never raw request input.
func (c *Corpus) GetDocument(ctx context.Context, role string, docID uuid.UUID) (DocumentDetail, error) {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return DocumentDetail{}, fmt.Errorf("beginning GetDocument read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Sections carry a vector(1536) column; register the codec on this exact
	// connection before scanning it (pooled connections aren't guaranteed to
	// share a type map — see ReplaceSections).
	if err := pgxvec.RegisterTypes(ctx, tx.Conn()); err != nil {
		return DocumentDetail{}, fmt.Errorf("registering pgvector types: %w", err)
	}

	if role != "" {
		// SET LOCAL ROLE can't take a query parameter for the role name;
		// role is documented as coming only from the fixed RLS-role set
		// above, but it's quoted via pgx.Identifier as defense in depth.
		roleQ := `SET LOCAL ROLE ` + pgx.Identifier{role}.Sanitize()
		if _, err := tx.Exec(ctx, roleQ); err != nil {
			return DocumentDetail{}, fmt.Errorf("setting local role %q: %w", role, err)
		}
	}

	doc, err := c.scanDocument(ctx, tx, docID)
	if err != nil {
		return DocumentDetail{}, err
	}
	sections, err := c.scanSections(ctx, tx, docID)
	if err != nil {
		return DocumentDetail{}, err
	}
	events, err := c.scanEvents(ctx, tx, docID)
	if err != nil {
		return DocumentDetail{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return DocumentDetail{}, fmt.Errorf("committing GetDocument read: %w", err)
	}
	return DocumentDetail{Doc: doc, Sections: sections, Events: events}, nil
}

// GetValidity returns docID's current validity_status. It is an owner-role
// read (no SET ROLE) for the ingest write path, which must see the target of
// an amendment event regardless of tier — use GetDocument for RLS-scoped
// reads. A missing row reports ErrDocumentNotFound.
func (c *Corpus) GetValidity(ctx context.Context, docID uuid.UUID) (string, error) {
	q := `SELECT validity_status FROM ` + c.qualify("document") + ` WHERE id = $1`
	var status string
	err := c.pool.QueryRow(ctx, q, docID).Scan(&status)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return "", fmt.Errorf("getting validity for document %s: %w (%w)", docID, ErrDocumentNotFound, err)
	case err != nil:
		return "", fmt.Errorf("getting validity for document %s: %w", docID, err)
	}
	return status, nil
}

const documentSelectCols = `id, corpus_id, title, doc_number, citation_scheme, citation_path, language,
	validity_status, issuing_authority, signer_name, version, source_url, source_system,
	content_type, access_tier, issued_date, effective_date, expiry_date, ingest_run_id, observed_at`

// scanDocument reads docID's document row. A missing or RLS-hidden row (see
// ErrDocumentNotFound) is reported through that sentinel, not a bare
// pgx.ErrNoRows/PgError.
func (c *Corpus) scanDocument(ctx context.Context, tx pgx.Tx, docID uuid.UUID) (Document, error) {
	q := `SELECT ` + documentSelectCols + ` FROM ` + c.qualify("document") + ` WHERE id = $1`

	var d Document
	var docNumber, citationScheme, citationPath, issuingAuthority, signerName *string
	var version, sourceURL, sourceSystem, contentType *string
	var ingestRunID *uuid.UUID

	err := tx.QueryRow(ctx, q, docID).Scan(
		&d.ID, &d.CorpusID, &d.Title, &docNumber, &citationScheme, &citationPath, &d.Language,
		&d.ValidityStatus, &issuingAuthority, &signerName, &version, &sourceURL, &sourceSystem,
		&contentType, &d.AccessTier, &d.IssuedDate, &d.EffectiveDate, &d.ExpiryDate, &ingestRunID, &d.ObservedAt,
	)
	switch {
	case isNotFound(err):
		// Multi-wrap (%w twice) so errors.Is(_, ErrDocumentNotFound) still
		// holds while the underlying cause (pgx.ErrNoRows, or the
		// *pgconn.PgError for SQLSTATE 42501) survives in the chain —
		// losing it here would make a missing GRANT on a future schema
		// masquerade as a clean not-found with zero diagnostics.
		return Document{}, fmt.Errorf("getting document %s: %w (%w)", docID, ErrDocumentNotFound, err)
	case err != nil:
		return Document{}, fmt.Errorf("getting document %s: %w", docID, err)
	}

	d.DocNumber, d.CitationScheme, d.CitationPath = derefOr(docNumber), derefOr(citationScheme), derefOr(citationPath)
	d.IssuingAuthority, d.SignerName, d.Version = derefOr(issuingAuthority), derefOr(signerName), derefOr(version)
	d.SourceURL, d.SourceSystem, d.ContentType = derefOr(sourceURL), derefOr(sourceSystem), derefOr(contentType)
	if ingestRunID != nil {
		d.IngestRunID = *ingestRunID
	}
	return d, nil
}

// scanSections reads docID's sections, ordered by position, id.
func (c *Corpus) scanSections(ctx context.Context, tx pgx.Tx, docID uuid.UUID) ([]Section, error) {
	const cols = `id, document_id, corpus_id, citation_path, heading_path, position, body, embedding,
		validity_status, access_tier, effective_date`
	q := `SELECT ` + cols + ` FROM ` + c.qualify("section") + ` WHERE document_id = $1 ORDER BY position, id`

	rows, err := tx.Query(ctx, q, docID)
	if err != nil {
		return nil, fmt.Errorf("querying sections for document %s: %w", docID, err)
	}
	defer rows.Close()

	var out []Section
	for rows.Next() {
		var s Section
		var citationPath, headingPath *string
		var emb *pgvector.Vector
		err := rows.Scan(&s.ID, &s.DocumentID, &s.CorpusID, &citationPath, &headingPath, &s.Position, &s.Body,
			&emb, &s.ValidityStatus, &s.AccessTier, &s.EffectiveDate)
		if err != nil {
			return nil, fmt.Errorf("scanning section row for document %s: %w", docID, err)
		}
		s.CitationPath, s.HeadingPath = derefOr(citationPath), derefOr(headingPath)
		if emb != nil {
			s.Embedding = emb.Slice()
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading section rows for document %s: %w", docID, err)
	}
	return out, nil
}

// scanEvents reads docID's amendment events, ordered by event_date.
func (c *Corpus) scanEvents(ctx context.Context, tx pgx.Tx, docID uuid.UUID) ([]AmendmentEvent, error) {
	const cols = `target_doc_id, amending_doc_id, clause, event_date`
	q := `SELECT ` + cols + ` FROM ` + c.qualify("amendment_event") + ` WHERE target_doc_id = $1 ORDER BY event_date`

	rows, err := tx.Query(ctx, q, docID)
	if err != nil {
		return nil, fmt.Errorf("querying amendment events for document %s: %w", docID, err)
	}
	defer rows.Close()

	var out []AmendmentEvent
	for rows.Next() {
		var e AmendmentEvent
		var clause *string
		if err := rows.Scan(&e.TargetDocID, &e.AmendingDocID, &clause, &e.EventDate); err != nil {
			return nil, fmt.Errorf("scanning amendment event row for document %s: %w", docID, err)
		}
		e.Clause = derefOr(clause)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading amendment event rows for document %s: %w", docID, err)
	}
	return out, nil
}

// derefOr returns *p, or "" if p is nil — folds a scanned nullable text
// column back into models.go's plain string fields (empty string is mise's
// "not set" convention for these, mirroring nullIfEmpty on the write side).
func derefOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// isNotFound reports whether err is either "no such row" or "this role
// can't even see the schema" (SQLSTATE 42501, insufficient_privilege) — see
// ErrDocumentNotFound for why both fold into the same outcome.
func isNotFound(err error) bool {
	if errors.Is(err, pgx.ErrNoRows) {
		return true
	}
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && pgErr.Code == "42501"
}
