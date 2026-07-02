package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/graph"
)

// ErrUnregisteredCorpus reports that a from_corpus_id given to
// WriteExtractedEdge isn't a corpus.Get-registered corpus.Descriptor —
// relation_edge.from_corpus_id has no schema-level guard of its own (unlike
// to_corpus_id, which the relation_edge_set_to_corpus BEFORE-INSERT trigger
// derives from the FK'd doc_ref — migrations/009_graph_tables.sql), so this
// check is the only thing standing between an untrusted from_corpus_id and
// the graph (T1 security review, R2 residual).
var ErrUnregisteredCorpus = errors.New("corpus is not registered")

// GraphStore is the compliance graph's write path: doc_ref
// resolution/creation and Method-A-extracted edge/evidence writes. It runs
// owner-side — no SET LOCAL ROLE, the same trust level as Corpus's write
// methods — because RLS (graph.relation_edge/relation_evidence's per-tier
// policies) is a read concern; the read path lives in the graph read
// repository (GraphRepo), not here.
type GraphStore struct {
	pool *pgxpool.Pool
}

// NewGraphStore returns a GraphStore backed by pool.
func NewGraphStore(pool *pgxpool.Pool) *GraphStore {
	return &GraphStore{pool: pool}
}

// buildRefKey derives graph.doc_ref's unique ref_key from a corpus id and a
// bare citation number: corpus-scoped so identical numbers in different
// corpora never collide, and upper-cased so citation-casing variants (a
// second document spelling the same number differently) still land on one
// row.
func buildRefKey(corpusID, number string) string {
	return corpusID + ":" + strings.ToUpper(strings.TrimSpace(number))
}

// EnsureDocRef upserts graph.doc_ref by ref_key (buildRefKey(corpusID,
// refKey)) and returns its id — re-calling with the same corpusID/refKey
// always resolves to the same row. Pass docID/secID nil to create or keep
// an unresolved stub (document_id/section_id NULL — doc_ref_unresolved_idx,
// migrations/009_graph_tables.sql); pass them non-nil to resolve it.
//
// The ON CONFLICT clause's COALESCE is the anti-regression guard: once a
// ref is resolved, a later call that doesn't know that (e.g. a second
// document citing the same target before its own extraction pass has seen
// the resolution) can never blank document_id/section_id back to NULL.
// label/src_ref are excluded from the UPDATE SET entirely — the first call
// to create a ref wins and later calls never overwrite it — since only
// document_id/section_id represent resolution progress worth protecting;
// label/src_ref are descriptive-only, and flip-flopping them across
// whichever document happens to cite the same target next would be a
// regression of its own kind.
func (g *GraphStore) EnsureDocRef(
	ctx context.Context, corpusID, refKey, label string, docID, secID *uuid.UUID, src json.RawMessage,
) (uuid.UUID, error) {
	key := buildRefKey(corpusID, refKey)
	if src == nil {
		src = json.RawMessage(`{}`)
	}

	const q = `
INSERT INTO graph.doc_ref (corpus_id, ref_key, document_id, section_id, label, src_ref)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (ref_key) DO UPDATE
SET document_id = COALESCE(doc_ref.document_id, EXCLUDED.document_id),
    section_id  = COALESCE(doc_ref.section_id, EXCLUDED.section_id),
    updated_at  = now()
RETURNING id`

	var id uuid.UUID
	if err := g.pool.QueryRow(ctx, q, corpusID, key, docID, secID, label, src).Scan(&id); err != nil {
		return uuid.UUID{}, fmt.Errorf("ensuring doc_ref %s: %w", key, err)
	}
	return id, nil
}

// ResolveDocRefs retroactively resolves corpusID's unresolved doc_ref stubs:
// for every number/docID pair in byNumber, it flips the matching stub's
// document_id from NULL to docID (buildRefKey(corpusID, number) is the same
// scoping/casing EnsureDocRef uses, so a number an earlier extraction pass
// cited is found here unchanged). It never touches relation_edge — edges
// already point at the doc_ref's stable id (to_ref_id), so a resolution
// needs no edge rewrite (PLAN.md's "no edge rewrites" decision). Returns
// how many stubs were actually flipped; already-resolved rows and numbers
// absent from byNumber are silently no-ops, so re-running is safe.
func (g *GraphStore) ResolveDocRefs(ctx context.Context, corpusID string, byNumber map[string]uuid.UUID) (int, error) {
	if len(byNumber) == 0 {
		return 0, nil
	}

	tx, err := g.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("beginning doc_ref resolution for corpus %s: %w", corpusID, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
UPDATE graph.doc_ref
SET document_id = $3, updated_at = now()
WHERE corpus_id = $1 AND ref_key = $2 AND document_id IS NULL`

	var resolved int
	for number, docID := range byNumber {
		key := buildRefKey(corpusID, number)
		tag, err := tx.Exec(ctx, q, corpusID, key, docID)
		if err != nil {
			return 0, fmt.Errorf("resolving doc_ref %s: %w", key, err)
		}
		resolved += int(tag.RowsAffected())
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("committing doc_ref resolution for corpus %s: %w", corpusID, err)
	}
	return resolved, nil
}

// WriteExtractedEdge persists one Method-A-extracted candidate edge: it
// resolves or creates the doc_ref for e.Target (EnsureDocRef), inserts the
// relation_edge row (idempotent via relation_edge_uq), and records the
// backing relation_evidence row (evidence_kind='extracted', idempotent via
// relation_evidence_uq). created_by carries the attestation owner string
// the caller already resolved via OrgRole — WriteExtractedEdge only stores
// it. Re-running with the same e is a no-op past the first call: both
// unique constraints turn a repeat insert into DO NOTHING, and the id
// returned is always the same row's.
//
// e.From.CorpusID must name a registered corpus.Descriptor. WriteExtractedEdge
// runs owner-side (no SET LOCAL ROLE, like Corpus's write methods) and is
// the only guard between an untrusted from_corpus_id and the graph: unlike
// to_corpus_id (trigger-authoritative — relation_edge_set_to_corpus reads it
// from the FK'd doc_ref), nothing in the schema validates from_corpus_id
// (T1 security review, R2 residual). Callers must build e.From.CorpusID
// from a validated source corpus.Descriptor, never from raw request input.
func (g *GraphStore) WriteExtractedEdge(ctx context.Context, e graph.ExtractedEdge) (uuid.UUID, error) {
	if _, ok := corpus.Get(corpus.ID(e.From.CorpusID)); !ok {
		return uuid.UUID{}, fmt.Errorf("graph: from_corpus_id %q: %w", e.From.CorpusID, ErrUnregisteredCorpus)
	}

	var toDocID, toSecID *uuid.UUID
	if !e.Target.IsStub {
		toDocID = &e.Target.Target.DocumentID
		toSecID = e.Target.Target.SectionID
	}
	toRefID, err := g.EnsureDocRef(
		ctx, e.Target.ToCorpusID, e.Target.RefKey, e.Target.Label, toDocID, toSecID, e.Target.SrcRef)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("resolving target doc_ref for extracted edge: %w", err)
	}

	direction := e.Direction
	if direction == "" {
		direction = "up"
	}

	edgeID, err := g.upsertRelationEdge(ctx, e.From.CorpusID, e.From.DocumentID, e.From.SectionID,
		toRefID, e.Target.ToCorpusID, e.EdgeType, direction)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("writing extracted relation_edge: %w", err)
	}

	if err := g.insertExtractedEvidence(ctx, edgeID, e.QuotedFromSpan, e.QuotedToSpan, e.CreatedBy); err != nil {
		return uuid.UUID{}, fmt.Errorf("writing extracted relation_evidence for edge %s: %w", edgeID, err)
	}
	return edgeID, nil
}

// upsertRelationEdge inserts one graph.relation_edge row and returns its id,
// or — when relation_edge_uq (from_corpus_id, from_document_id, to_ref_id,
// edge_type) already has a matching row — fetches that row's id instead.
// promoted is always false: Method A only ever writes candidate edges.
func (g *GraphStore) upsertRelationEdge(
	ctx context.Context, fromCorpusID string, fromDocID uuid.UUID, fromSecID *uuid.UUID,
	toRefID uuid.UUID, toCorpusID, edgeType, direction string,
) (uuid.UUID, error) {
	const insertQ = `
INSERT INTO graph.relation_edge
	(from_corpus_id, from_document_id, from_section_id, to_ref_id, to_corpus_id, edge_type, direction, promoted)
VALUES ($1, $2, $3, $4, $5, $6, $7, false)
ON CONFLICT (from_corpus_id, from_document_id, to_ref_id, edge_type) DO NOTHING
RETURNING id`

	var id uuid.UUID
	err := g.pool.QueryRow(ctx, insertQ, fromCorpusID, fromDocID, fromSecID, toRefID, toCorpusID, edgeType, direction).
		Scan(&id)
	switch {
	case err == nil:
		return id, nil
	case errors.Is(err, pgx.ErrNoRows):
		return g.findRelationEdge(ctx, fromCorpusID, fromDocID, toRefID, edgeType)
	default:
		return uuid.UUID{}, fmt.Errorf("inserting relation_edge: %w", err)
	}
}

// findRelationEdge looks up an existing relation_edge row by
// relation_edge_uq's natural key — upsertRelationEdge's fallback once its
// INSERT ... DO NOTHING confirms the row already existed.
func (g *GraphStore) findRelationEdge(
	ctx context.Context, fromCorpusID string, fromDocID, toRefID uuid.UUID, edgeType string,
) (uuid.UUID, error) {
	const q = `
SELECT id FROM graph.relation_edge
WHERE from_corpus_id = $1 AND from_document_id = $2 AND to_ref_id = $3 AND edge_type = $4`

	var id uuid.UUID
	if err := g.pool.QueryRow(ctx, q, fromCorpusID, fromDocID, toRefID, edgeType).Scan(&id); err != nil {
		return uuid.UUID{}, fmt.Errorf("finding existing relation_edge: %w", err)
	}
	return id, nil
}

// insertExtractedEvidence inserts edgeID's evidence_kind='extracted' row —
// confidence is always 1.0 (Method A is deterministic, not a probabilistic
// classification) — skipping the insert (ON CONFLICT DO NOTHING) if
// relation_evidence_uq (edge_id, evidence_kind) already has one.
func (g *GraphStore) insertExtractedEvidence(
	ctx context.Context, edgeID uuid.UUID, fromSpan, toSpan, createdBy string,
) error {
	const q = `
INSERT INTO graph.relation_evidence (edge_id, evidence_kind, confidence, quoted_from_span, quoted_to_span, created_by)
VALUES ($1, 'extracted', 1.0, $2, $3, $4)
ON CONFLICT (edge_id, evidence_kind) DO NOTHING`

	if _, err := g.pool.Exec(ctx, q, edgeID, fromSpan, toSpan, createdBy); err != nil {
		return fmt.Errorf("inserting relation_evidence: %w", err)
	}
	return nil
}
