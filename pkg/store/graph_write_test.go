//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"danny.vn/mise/internal/testdb"
	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/store"
)

// fetchDocRefByID re-selects one graph.doc_ref row by id, reusing
// graph_schema_test.go's scanDocRef so both files agree on the column
// order a doc_ref row scans into.
func fetchDocRefByID(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) graph.DocRef {
	t.Helper()
	const q = `
		SELECT id, corpus_id, ref_key, label, document_id, section_id, created_at, updated_at
		FROM graph.doc_ref WHERE id = $1`
	r, err := scanDocRef(pool.QueryRow(ctx, q, id))
	if err != nil {
		t.Fatalf("fetching doc_ref %s: %v", id, err)
	}
	return r
}

// fetchRelationEdge scans one graph.relation_edge row into a graph.Edge —
// a fuller read than graph_schema_test.go's insertEdge helper (which only
// returns access_tier), needed here to assert WriteExtractedEdge's output
// (promoted, from/to identity) as a whole.
func fetchRelationEdge(t *testing.T, ctx context.Context, pool *pgxpool.Pool, edgeID uuid.UUID) graph.Edge {
	t.Helper()
	const q = `
		SELECT id, from_corpus_id, from_document_id, from_section_id, to_ref_id, to_corpus_id,
		       edge_type, direction, promoted, access_tier, created_at
		FROM graph.relation_edge WHERE id = $1`
	var e graph.Edge
	var edgeType, tier string
	err := pool.QueryRow(ctx, q, edgeID).Scan(
		&e.ID, &e.From.CorpusID, &e.From.DocumentID, &e.From.SectionID, &e.ToRefID, &e.ToCorpusID,
		&edgeType, &e.Direction, &e.Promoted, &tier, &e.CreatedAt,
	)
	if err != nil {
		t.Fatalf("fetching relation_edge %s: %v", edgeID, err)
	}
	e.EdgeType = graph.EdgeType(edgeType)
	e.AccessTier = graph.Tier(tier)
	return e
}

// fetchExtractedEvidence scans the evidence_kind='extracted' row for edgeID,
// including the quoted-span/created_by columns graph_schema_test.go's
// scanEvidence doesn't read.
func fetchExtractedEvidence(t *testing.T, ctx context.Context, pool *pgxpool.Pool, edgeID uuid.UUID) graph.Evidence {
	t.Helper()
	const q = `
		SELECT id, edge_id, evidence_kind, confidence, quoted_from_span, quoted_to_span, created_by
		FROM graph.relation_evidence WHERE edge_id = $1 AND evidence_kind = 'extracted'`
	var e graph.Evidence
	var kind string
	err := pool.QueryRow(ctx, q, edgeID).Scan(
		&e.ID, &e.EdgeID, &kind, &e.Confidence, &e.QuotedFromSpan, &e.QuotedToSpan, &e.CreatedBy,
	)
	if err != nil {
		t.Fatalf("fetching extracted relation_evidence for edge %s: %v", edgeID, err)
	}
	e.EvidenceKind = graph.EvidenceKind(kind)
	return e
}

// countRelationEdgeRows counts relation_edge rows matching the natural key
// relation_edge_uq enforces — the idempotency oracle: a re-run of
// WriteExtractedEdge must never push this above 1.
func countRelationEdgeRows(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	fromCorpusID string, fromDocID, toRefID uuid.UUID, edgeType string,
) int {
	t.Helper()
	const q = `
		SELECT count(*) FROM graph.relation_edge
		WHERE from_corpus_id = $1 AND from_document_id = $2 AND to_ref_id = $3 AND edge_type = $4`
	var n int
	if err := pool.QueryRow(ctx, q, fromCorpusID, fromDocID, toRefID, edgeType).Scan(&n); err != nil {
		t.Fatalf("counting relation_edge rows: %v", err)
	}
	return n
}

// countExtractedEvidenceRows counts the evidence_kind='extracted' rows for
// edgeID — the evidence-side idempotency oracle.
func countExtractedEvidenceRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, edgeID uuid.UUID) int {
	t.Helper()
	const q = `SELECT count(*) FROM graph.relation_evidence WHERE edge_id = $1 AND evidence_kind = 'extracted'`
	var n int
	if err := pool.QueryRow(ctx, q, edgeID).Scan(&n); err != nil {
		t.Fatalf("counting relation_evidence rows: %v", err)
	}
	return n
}

// TestEnsureDocRefIdempotentAndNeverRegressesResolution pins EnsureDocRef's
// two headline properties: re-calling with identical args always returns
// the same id (no duplicate row), and once a ref is resolved
// (document_id set), a later call that doesn't know the resolution — e.g. a
// second citing document repeating the same still-looks-unresolved
// reference — can never blank it back to a stub. It also locks in the
// ref_key convention (corpusID + ":" + upper(refKey)) the brief specifies.
func TestEnsureDocRefIdempotentAndNeverRegressesResolution(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	g := store.NewGraphStore(pool)

	corpusID := string(corpus.LocalPolicy)
	refKey := "ensure-" + uuid.NewString()
	label := "Data Protection Policy"
	src := json.RawMessage(`{"target_number":"` + refKey + `"}`)

	id1, err := g.EnsureDocRef(ctx, corpusID, refKey, label, nil, nil, src)
	if err != nil {
		t.Fatalf("first EnsureDocRef() error = %v", err)
	}

	id2, err := g.EnsureDocRef(ctx, corpusID, refKey, label, nil, nil, src)
	if err != nil {
		t.Fatalf("second EnsureDocRef() (identical stub call) error = %v", err)
	}
	if id2 != id1 {
		t.Errorf("EnsureDocRef() re-call id = %v, want %v (same row)", id2, id1)
	}

	wantRefKey := corpusID + ":" + strings.ToUpper(refKey)
	stub := fetchDocRefByID(t, ctx, pool, id1)
	if stub.RefKey != wantRefKey {
		t.Errorf("doc_ref.ref_key = %q, want %q (corpusID + \":\" + upper(refKey))", stub.RefKey, wantRefKey)
	}
	if stub.DocumentID != nil {
		t.Errorf("doc_ref.document_id after stub calls = %v, want nil", stub.DocumentID)
	}

	docID := uuid.New()
	id3, err := g.EnsureDocRef(ctx, corpusID, refKey, label, &docID, nil, src)
	if err != nil {
		t.Fatalf("resolving EnsureDocRef() error = %v", err)
	}
	if id3 != id1 {
		t.Errorf("EnsureDocRef() resolving call id = %v, want %v (same row)", id3, id1)
	}
	resolved := fetchDocRefByID(t, ctx, pool, id1)
	if resolved.DocumentID == nil || *resolved.DocumentID != docID {
		t.Fatalf("doc_ref.document_id after resolving call = %v, want %v", resolved.DocumentID, docID)
	}

	// A later stub-shaped call for the SAME ref_key (docID/secID nil again)
	// must not regress the now-resolved document_id back to NULL.
	id4, err := g.EnsureDocRef(ctx, corpusID, refKey, label, nil, nil, src)
	if err != nil {
		t.Fatalf("post-resolution stub-shaped EnsureDocRef() error = %v", err)
	}
	if id4 != id1 {
		t.Errorf("EnsureDocRef() post-resolution call id = %v, want %v (same row)", id4, id1)
	}
	stillResolved := fetchDocRefByID(t, ctx, pool, id1)
	if stillResolved.DocumentID == nil || *stillResolved.DocumentID != docID {
		t.Errorf("doc_ref.document_id regressed to %v after a stub-shaped re-call, want it to stay %v",
			stillResolved.DocumentID, docID)
	}
}

// extractedFixture returns a graph.ExtractedEdge from local-sop to a fresh
// local-policy stub target, unique per call (fresh From.DocumentID and
// Target.RefKey) so tests that write their own fixture never collide with
// each other on testdb's shared singleton container.
func extractedFixture(t *testing.T) graph.ExtractedEdge {
	t.Helper()
	refKey := "extracted-" + uuid.NewString()
	return graph.ExtractedEdge{
		From:           graph.NodeRef{CorpusID: string(corpus.LocalSOP), DocumentID: uuid.New()},
		EdgeType:       string(graph.EdgeDerives),
		Direction:      "up",
		QuotedFromSpan: "This SOP implements the group Data Handling Policy",
		QuotedToSpan:   "",
		CreatedBy:      "jane.doe@example.com",
		Target: graph.ResolvedRef{
			ToCorpusID: string(corpus.LocalPolicy),
			IsStub:     true,
			RefKey:     refKey,
			Label:      "Data Handling Policy",
			SrcRef:     json.RawMessage(`{"target_number":"` + refKey + `","relation":"derives"}`),
		},
	}
}

// TestWriteExtractedEdgeIdempotentWritesOneEdgeAndOneEvidence is the
// milestone's headline idempotency assertion: writing the same
// ExtractedEdge twice must produce exactly one relation_edge row
// (promoted=false) and exactly one relation_evidence row
// (evidence_kind=extracted, carrying the quoted spans and the passed
// attestation owner) — re-extraction must never duplicate either. It also
// asserts the T1-review R2 invariant that from_corpus_id in the persisted
// row is exactly what the caller passed (truthful propagation), not
// something the writer could silently substitute.
func TestWriteExtractedEdgeIdempotentWritesOneEdgeAndOneEvidence(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	g := store.NewGraphStore(pool)
	edge := extractedFixture(t)

	id1, err := g.WriteExtractedEdge(ctx, edge)
	if err != nil {
		t.Fatalf("first WriteExtractedEdge() error = %v", err)
	}
	id2, err := g.WriteExtractedEdge(ctx, edge)
	if err != nil {
		t.Fatalf("second WriteExtractedEdge() (re-run) error = %v", err)
	}
	if id2 != id1 {
		t.Errorf("WriteExtractedEdge() re-run id = %v, want %v (same edge row)", id2, id1)
	}

	got := fetchRelationEdge(t, ctx, pool, id1)
	if got.From.CorpusID != string(corpus.LocalSOP) {
		t.Errorf("relation_edge.from_corpus_id = %q, want %q (must match the passed source descriptor)",
			got.From.CorpusID, string(corpus.LocalSOP))
	}
	if got.From.DocumentID != edge.From.DocumentID {
		t.Errorf("relation_edge.from_document_id = %v, want %v", got.From.DocumentID, edge.From.DocumentID)
	}
	if got.ToCorpusID != string(corpus.LocalPolicy) {
		t.Errorf("relation_edge.to_corpus_id = %q, want %q", got.ToCorpusID, string(corpus.LocalPolicy))
	}
	if got.EdgeType != graph.EdgeDerives {
		t.Errorf("relation_edge.edge_type = %q, want %q", got.EdgeType, graph.EdgeDerives)
	}
	if got.Direction != "up" {
		t.Errorf("relation_edge.direction = %q, want %q", got.Direction, "up")
	}
	if got.Promoted {
		t.Error("relation_edge.promoted = true, want false (Method A never auto-promotes)")
	}

	edgeRowCount := countRelationEdgeRows(
		t, ctx, pool, edge.From.CorpusID, edge.From.DocumentID, got.ToRefID, edge.EdgeType)
	if edgeRowCount != 1 {
		t.Errorf("relation_edge row count after 2 writes = %d, want 1", edgeRowCount)
	}

	ev := fetchExtractedEvidence(t, ctx, pool, id1)
	if ev.QuotedFromSpan != edge.QuotedFromSpan {
		t.Errorf("relation_evidence.quoted_from_span = %q, want %q", ev.QuotedFromSpan, edge.QuotedFromSpan)
	}
	if ev.QuotedToSpan != edge.QuotedToSpan {
		t.Errorf("relation_evidence.quoted_to_span = %q, want %q", ev.QuotedToSpan, edge.QuotedToSpan)
	}
	if ev.CreatedBy != edge.CreatedBy {
		t.Errorf("relation_evidence.created_by = %q, want %q (the passed attestation owner)", ev.CreatedBy, edge.CreatedBy)
	}
	if ev.EvidenceKind != graph.EvidenceExtracted {
		t.Errorf("relation_evidence.evidence_kind = %q, want %q", ev.EvidenceKind, graph.EvidenceExtracted)
	}

	if n := countExtractedEvidenceRows(t, ctx, pool, id1); n != 1 {
		t.Errorf("relation_evidence row count after 2 writes = %d, want 1", n)
	}
}

// TestWriteExtractedEdgeLocalSopToLocalPolicyAccessTierIsLocalConfidential
// is the milestone's cross-tier confidentiality check for the writer
// specifically (graph_schema_test.go already pins the DB-generated rule
// itself): a local-sop-sourced edge into local-policy must come back
// local-confidential end to end through WriteExtractedEdge, not just via a
// raw INSERT.
func TestWriteExtractedEdgeLocalSopToLocalPolicyAccessTierIsLocalConfidential(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	g := store.NewGraphStore(pool)
	edge := extractedFixture(t)

	edgeID, err := g.WriteExtractedEdge(ctx, edge)
	if err != nil {
		t.Fatalf("WriteExtractedEdge() error = %v", err)
	}

	got := fetchRelationEdge(t, ctx, pool, edgeID)
	if got.AccessTier != graph.TierLocalConfidential {
		t.Errorf("access_tier = %q, want %q", got.AccessTier, graph.TierLocalConfidential)
	}
}

// TestResolveDocRefsFlipsStubDocumentIDWithoutChangingEdge proves the
// retroactive-resolution path: ResolveDocRefs must flip the stub's
// document_id from NULL to the resolved id when the target's cited number
// appears in the map, while leaving the relation_edge row that already
// points at it (via to_ref_id) completely untouched — resolution never
// rewrites edges (PLAN.md's "no edge rewrites" decision).
func TestResolveDocRefsFlipsStubDocumentIDWithoutChangingEdge(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	g := store.NewGraphStore(pool)
	edge := extractedFixture(t)

	edgeID, err := g.WriteExtractedEdge(ctx, edge)
	if err != nil {
		t.Fatalf("WriteExtractedEdge() error = %v", err)
	}
	before := fetchRelationEdge(t, ctx, pool, edgeID)

	beforeRef := fetchDocRefByID(t, ctx, pool, before.ToRefID)
	if beforeRef.DocumentID != nil {
		t.Fatalf("doc_ref.document_id before ResolveDocRefs = %v, want nil (still a stub)", beforeRef.DocumentID)
	}

	realDocID := uuid.New()
	n, err := g.ResolveDocRefs(ctx, string(corpus.LocalPolicy), map[string]uuid.UUID{edge.Target.RefKey: realDocID})
	if err != nil {
		t.Fatalf("ResolveDocRefs() error = %v", err)
	}
	if n != 1 {
		t.Errorf("ResolveDocRefs() resolved count = %d, want 1", n)
	}

	afterRef := fetchDocRefByID(t, ctx, pool, before.ToRefID)
	if afterRef.DocumentID == nil || *afterRef.DocumentID != realDocID {
		t.Errorf("doc_ref.document_id after ResolveDocRefs() = %v, want %v", afterRef.DocumentID, realDocID)
	}

	after := fetchRelationEdge(t, ctx, pool, edgeID)
	if before.ID != after.ID || before.ToRefID != after.ToRefID || before.ToCorpusID != after.ToCorpusID ||
		before.EdgeType != after.EdgeType || before.Direction != after.Direction ||
		before.Promoted != after.Promoted || before.AccessTier != after.AccessTier ||
		before.From.CorpusID != after.From.CorpusID || before.From.DocumentID != after.From.DocumentID ||
		!before.CreatedAt.Equal(after.CreatedAt) {
		t.Errorf("relation_edge row changed after ResolveDocRefs(): before=%+v after=%+v", before, after)
	}

	// Idempotent: re-running must not error and must not re-count an
	// already-resolved row.
	n2, err := g.ResolveDocRefs(ctx, string(corpus.LocalPolicy), map[string]uuid.UUID{edge.Target.RefKey: realDocID})
	if err != nil {
		t.Fatalf("second ResolveDocRefs() error = %v", err)
	}
	if n2 != 0 {
		t.Errorf("second ResolveDocRefs() resolved count = %d, want 0 (already resolved)", n2)
	}
}

// TestWriteExtractedEdgeRejectsUnregisteredFromCorpus is the T1-review R2
// residual guard: WriteExtractedEdge runs owner-side with no SET LOCAL
// ROLE, so it is the only check standing between an untrusted
// from_corpus_id and the graph (to_corpus_id has the BEFORE-INSERT trigger;
// from_corpus_id does not). A from_corpus_id that isn't a registered
// corpus.Descriptor must be rejected before any row is written.
func TestWriteExtractedEdgeRejectsUnregisteredFromCorpus(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	g := store.NewGraphStore(pool)
	edge := extractedFixture(t)
	// Unique per run (not the bare literal graph_schema_test.go's own
	// fail-closed-tier fixture uses) so the verification count below can
	// never pick up an unrelated row sharing this bogus corpus id on
	// testdb's shared singleton container.
	bogusCorpus := "nonexistent-corpus-" + uuid.NewString()
	edge.From.CorpusID = bogusCorpus

	_, err := g.WriteExtractedEdge(ctx, edge)
	if err == nil {
		t.Fatal("WriteExtractedEdge() with an unregistered from_corpus_id error = nil, want error")
	}
	if !errors.Is(err, store.ErrUnregisteredCorpus) {
		t.Errorf("WriteExtractedEdge() error = %v, want errors.Is(_, store.ErrUnregisteredCorpus)", err)
	}

	// Confirm the rejection is truly pre-write: the doc_ref stub created by
	// EnsureDocRef would use the same ref_key regardless of the from-side
	// corpus, so a real relation_edge row keyed on the bogus corpus is what
	// a fail-open bug would leave behind. Scoped by from_document_id too
	// (unique to this fixture) so this can only ever match a row this test
	// itself could have produced.
	const q = `SELECT count(*) FROM graph.relation_edge WHERE from_corpus_id = $1 AND from_document_id = $2`
	var n int
	if err := pool.QueryRow(ctx, q, bogusCorpus, edge.From.DocumentID).Scan(&n); err != nil {
		t.Fatalf("counting relation_edge rows for the rejected corpus: %v", err)
	}
	if n != 0 {
		t.Errorf("relation_edge rows with from_corpus_id = %s = %d, want 0", bogusCorpus, n)
	}
}
