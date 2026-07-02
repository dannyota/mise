//go:build integration

package store_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"danny.vn/mise/internal/testdb"
	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/store"
)

// e2eFixture holds the IDs and extracted edges for the full extraction-to-chain
// pipeline test. Three internal documents form a control chain:
// local-sop -derives-> local-policy -implements-> group-std.
type e2eFixture struct {
	sopDocID, policyDocID, groupDocID uuid.UUID
	sopEdges, policyEdges             []graph.ExtractedEdge
}

// newE2EFixture builds doc-control headers for three documents, extracts edges
// via Method A (ExtractEdges + ResolveRef), and returns the fixture. No
// database interaction — this is the pure-extraction phase.
func newE2EFixture(t *testing.T) e2eFixture {
	t.Helper()

	sopDocID, policyDocID, groupDocID := uuid.New(), uuid.New(), uuid.New()

	lookup := func(target corpus.ID, number string) (uuid.UUID, bool) {
		switch {
		case target == corpus.LocalPolicy && number == "POL-001":
			return policyDocID, true
		case target == corpus.GroupStd && number == "GRP-STD-001":
			return groupDocID, true
		default:
			return uuid.UUID{}, false
		}
	}

	sopHeader := graph.DocControlHeader{
		Corpus:           corpus.LocalSOP,
		DocID:            sopDocID,
		DocNumber:        "SOP-001",
		AttestationOwner: "ops-team",
		ControlRefs: []graph.RawControlRef{{
			Relation:     "derives",
			TargetNumber: "POL-001",
			TargetTitle:  "Internal Policy",
			QuotedSpan:   "Derived from POL-001",
		}},
	}
	policyHeader := graph.DocControlHeader{
		Corpus:           corpus.LocalPolicy,
		DocID:            policyDocID,
		DocNumber:        "POL-001",
		AttestationOwner: "risk-team",
		ControlRefs: []graph.RawControlRef{{
			Relation:     "implements",
			TargetNumber: "GRP-STD-001",
			TargetTitle:  "Group Standard",
			QuotedSpan:   "Implements GRP-STD-001",
		}},
	}

	sopDesc, ok := corpus.Get(corpus.LocalSOP)
	if !ok {
		t.Fatal("corpus.Get(LocalSOP) not registered")
	}
	policyDesc, ok := corpus.Get(corpus.LocalPolicy)
	if !ok {
		t.Fatal("corpus.Get(LocalPolicy) not registered")
	}

	return e2eFixture{
		sopDocID:    sopDocID,
		policyDocID: policyDocID,
		groupDocID:  groupDocID,
		sopEdges:    graph.ExtractEdges(sopDesc, sopHeader, makeResolve(sopDesc.ID, lookup)),
		policyEdges: graph.ExtractEdges(policyDesc, policyHeader, makeResolve(policyDesc.ID, lookup)),
	}
}

// TestExtractionToChainE2E proves the full Method A pipeline end-to-end:
// extraction -> write -> chain walk, including idempotency. Each phase is its
// own top-level helper to stay under the cognitive-complexity budget.
func TestExtractionToChainE2E(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	fx := newE2EFixture(t)

	t.Run("extraction_output", func(t *testing.T) {
		assertE2EExtraction(t, fx)
	})

	graphStore := store.NewGraphStore(pool)
	sopEdgeID, policyEdgeID := writeE2EEdges(t, ctx, graphStore, fx)

	t.Run("chain_walk", func(t *testing.T) {
		assertE2EChainWalk(t, ctx, pool, fx)
	})
	t.Run("idempotency", func(t *testing.T) {
		assertE2EIdempotency(t, ctx, pool, graphStore, fx, sopEdgeID)
	})
	t.Run("evidence_kind", func(t *testing.T) {
		assertE2EEvidenceKind(t, ctx, pool, sopEdgeID)
	})
	_ = policyEdgeID // used only to prove both writes succeed
}

// assertE2EExtraction verifies the extraction phase produced the expected edges.
func assertE2EExtraction(t *testing.T, fx e2eFixture) {
	t.Helper()
	if len(fx.sopEdges) != 1 {
		t.Fatalf("sopEdges: got %d edges, want 1", len(fx.sopEdges))
	}
	assertEdge(t, fx.sopEdges[0], "derives", "up", "local-sop")

	if len(fx.policyEdges) != 1 {
		t.Fatalf("policyEdges: got %d edges, want 1", len(fx.policyEdges))
	}
	assertEdge(t, fx.policyEdges[0], "implements", "up", "local-policy")
}

// writeE2EEdges writes both extracted edges and returns their IDs.
func writeE2EEdges(
	t *testing.T, ctx context.Context, gs *store.GraphStore, fx e2eFixture,
) (sopEdgeID, policyEdgeID uuid.UUID) {
	t.Helper()
	var err error
	sopEdgeID, err = gs.WriteExtractedEdge(ctx, fx.sopEdges[0])
	if err != nil {
		t.Fatalf("WriteExtractedEdge(sop): %v", err)
	}
	policyEdgeID, err = gs.WriteExtractedEdge(ctx, fx.policyEdges[0])
	if err != nil {
		t.Fatalf("WriteExtractedEdge(policy): %v", err)
	}
	if sopEdgeID == uuid.Nil {
		t.Error("sopEdgeID is nil")
	}
	if policyEdgeID == uuid.Nil {
		t.Error("policyEdgeID is nil")
	}
	return sopEdgeID, policyEdgeID
}

// assertE2EChainWalk walks the chain from the SOP and verifies hop order, edge
// types, promotion status, and confidence.
func assertE2EChainWalk(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fx e2eFixture) {
	t.Helper()
	repo := store.NewGraphRepo(pool)
	hops, err := repo.Chain(ctx, "mise_local", graph.NodeRef{
		CorpusID:   "local-sop",
		DocumentID: fx.sopDocID,
	}, 8)
	if err != nil {
		t.Fatalf("Chain: %v", err)
	}
	if len(hops) != 2 {
		t.Fatalf("got %d hops, want 2", len(hops))
	}

	assertHop(t, hops[0], "local-policy", "derives")
	assertHop(t, hops[1], "group-std", "implements")
}

// assertHop checks one hop's corpus, edge type, promotion, and confidence.
func assertHop(t *testing.T, hop store.Hop, wantCorpus, wantEdgeType string) {
	t.Helper()
	if hop.CorpusID != wantCorpus {
		t.Errorf("hop.CorpusID = %q, want %q", hop.CorpusID, wantCorpus)
	}
	if hop.EdgeType != wantEdgeType {
		t.Errorf("hop.EdgeType = %q, want %q", hop.EdgeType, wantEdgeType)
	}
	if hop.Promoted {
		t.Errorf("hop.Promoted = true, want false (corpus %s)", wantCorpus)
	}
	if hop.Confidence != 1.0 {
		t.Errorf("hop.Confidence = %v, want 1.0 (corpus %s)", hop.Confidence, wantCorpus)
	}
}

// assertE2EIdempotency re-extracts and re-writes the SOP edge, then asserts
// the same edge ID is returned and row counts remain 1.
func assertE2EIdempotency(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool, gs *store.GraphStore,
	fx e2eFixture, originalEdgeID uuid.UUID,
) {
	t.Helper()
	sopDesc, _ := corpus.Get(corpus.LocalSOP)
	lookup := func(target corpus.ID, number string) (uuid.UUID, bool) {
		if target == corpus.LocalPolicy && number == "POL-001" {
			return fx.policyDocID, true
		}
		return uuid.UUID{}, false
	}
	sopHeader := graph.DocControlHeader{
		Corpus:           corpus.LocalSOP,
		DocID:            fx.sopDocID,
		DocNumber:        "SOP-001",
		AttestationOwner: "ops-team",
		ControlRefs: []graph.RawControlRef{{
			Relation:     "derives",
			TargetNumber: "POL-001",
			TargetTitle:  "Internal Policy",
			QuotedSpan:   "Derived from POL-001",
		}},
	}
	edges := graph.ExtractEdges(sopDesc, sopHeader, makeResolve(sopDesc.ID, lookup))
	if len(edges) != 1 {
		t.Fatalf("re-extraction: got %d edges, want 1", len(edges))
	}

	edgeID2, err := gs.WriteExtractedEdge(ctx, edges[0])
	if err != nil {
		t.Fatalf("re-write WriteExtractedEdge: %v", err)
	}
	if edgeID2 != originalEdgeID {
		t.Errorf("idempotent edge id: got %v, want %v", edgeID2, originalEdgeID)
	}

	assertRowCount(t, ctx, pool,
		`SELECT count(*) FROM graph.relation_edge WHERE from_corpus_id = 'local-sop' AND from_document_id = $1`,
		1, fx.sopDocID)
	assertRowCount(t, ctx, pool,
		`SELECT count(*) FROM graph.relation_evidence WHERE edge_id = $1`,
		1, originalEdgeID)
}

// assertE2EEvidenceKind verifies the evidence row's kind is 'extracted'.
func assertE2EEvidenceKind(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool, edgeID uuid.UUID,
) {
	t.Helper()
	var kind string
	err := pool.QueryRow(ctx,
		`SELECT evidence_kind FROM graph.relation_evidence WHERE edge_id = $1`,
		edgeID).Scan(&kind)
	if err != nil {
		t.Fatalf("querying evidence_kind: %v", err)
	}
	if kind != "extracted" {
		t.Errorf("evidence_kind = %q, want %q", kind, "extracted")
	}
}

// makeResolve builds the resolve closure ExtractEdges expects, closing over
// graph.ResolveRef with the given source corpus and lookup function.
func makeResolve(
	from corpus.ID, lookup func(corpus.ID, string) (uuid.UUID, bool),
) func(graph.RawControlRef) (graph.ResolvedRef, bool) {
	return func(ref graph.RawControlRef) (graph.ResolvedRef, bool) {
		return graph.ResolveRef(from, ref, lookup)
	}
}

// assertEdge checks an ExtractedEdge's edge type, direction, and from corpus.
func assertEdge(t *testing.T, e graph.ExtractedEdge, wantType, wantDir, wantCorpus string) {
	t.Helper()
	if e.EdgeType != wantType {
		t.Errorf("EdgeType = %q, want %q", e.EdgeType, wantType)
	}
	if e.Direction != wantDir {
		t.Errorf("Direction = %q, want %q", e.Direction, wantDir)
	}
	if e.From.CorpusID != wantCorpus {
		t.Errorf("From.CorpusID = %q, want %q", e.From.CorpusID, wantCorpus)
	}
}

// assertRowCount runs a count query and asserts it equals want.
func assertRowCount(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, want int, args ...any,
) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, query, args...).Scan(&got); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if got != want {
		t.Errorf("row count = %d, want %d", got, want)
	}
}
