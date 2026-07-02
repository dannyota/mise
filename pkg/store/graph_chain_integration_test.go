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

// graphChainFixture seeds a real, 3-hop control chain: sopRef (local-sop)
// --derives--> policyRef (local-policy) --implements--> groupRef
// (group-std) --satisfies--> lawRef (my-reg). Every doc_ref is resolved
// (document_id set) up front, so GraphRepo.Chain walks to a real NodeRef at
// every hop instead of stopping at an unresolved stub — unlike
// newGraphReadFixture/newGraphRLSFixture's targets, which are deliberately
// left as stubs for their own tests.
type graphChainFixture struct {
	sopRef, policyRef, groupRef, lawRef graph.NodeRef
}

// newGraphChainFixture seeds the fixture described above via raw SQL under
// the pool's connecting owner role (bypasses RLS on write) — mirrors
// newGraphRLSFixture/newGraphReadFixture's seeding style (graph_rls_test.go,
// graph_read_test.go). doc_refs are inserted and resolved before their
// edges, exactly like those fixtures, since relation_edge_set_to_corpus
// (migrations/009_graph_tables.sql) reads the doc_ref row to derive
// to_corpus_id.
func newGraphChainFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) graphChainFixture {
	t.Helper()

	sopRef := graph.NodeRef{CorpusID: string(corpus.LocalSOP), DocumentID: uuid.New()}

	policyDocRef := insertDocRef(t, ctx, pool, string(corpus.LocalPolicy), "local-policy:chain-"+uuid.NewString())
	groupDocRef := insertDocRef(t, ctx, pool, string(corpus.GroupStd), "group-std:chain-"+uuid.NewString())
	lawDocRef := insertDocRef(t, ctx, pool, string(corpus.MYReg), "my-reg:chain-"+uuid.NewString())

	policyDocID, groupDocID, lawDocID := uuid.New(), uuid.New(), uuid.New()
	resolveDocRefRow(t, ctx, pool, policyDocRef.ID, policyDocID)
	resolveDocRefRow(t, ctx, pool, groupDocRef.ID, groupDocID)
	resolveDocRefRow(t, ctx, pool, lawDocRef.ID, lawDocID)

	insertChainEdge(t, ctx, pool, string(corpus.LocalSOP), sopRef.DocumentID,
		string(corpus.LocalPolicy), string(graph.EdgeDerives), policyDocRef.ID)
	insertChainEdge(t, ctx, pool, string(corpus.LocalPolicy), policyDocID,
		string(corpus.GroupStd), string(graph.EdgeImplements), groupDocRef.ID)
	insertChainEdge(t, ctx, pool, string(corpus.GroupStd), groupDocID,
		string(corpus.MYReg), string(graph.EdgeSatisfies), lawDocRef.ID)

	return graphChainFixture{
		sopRef:    sopRef,
		policyRef: graph.NodeRef{CorpusID: string(corpus.LocalPolicy), DocumentID: policyDocID},
		groupRef:  graph.NodeRef{CorpusID: string(corpus.GroupStd), DocumentID: groupDocID},
		lawRef:    graph.NodeRef{CorpusID: string(corpus.MYReg), DocumentID: lawDocID},
	}
}

// resolveDocRefRow flips refID's doc_ref row from an unresolved stub to
// document_id = docID — this fixture needs a REAL resolved NodeRef at every
// hop (not a stub) so Chain can walk past it. Mirrors the raw UPDATE
// TestDocRefUnresolvedThenResolved (graph_schema_test.go) runs inline.
func resolveDocRefRow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, refID, docID uuid.UUID) {
	t.Helper()
	const q = `UPDATE graph.doc_ref SET document_id = $1 WHERE id = $2`
	if _, err := pool.Exec(ctx, q, docID, refID); err != nil {
		t.Fatalf("resolving doc_ref %s to document %s: %v", refID, docID, err)
	}
}

// insertChainEdge inserts one graph.relation_edge row from a caller-chosen
// fromDocID (so a chain's next hop can share the previous hop's resolved
// target document) with an explicit edgeType — unlike insertEdgeFrom
// (graph_read_test.go), which always hardcodes edge_type to "satisfies".
// direction is left to the column's own 'up' default
// (migrations/009_graph_tables.sql), matching every other raw-SQL edge
// insert in this package's test helpers.
func insertChainEdge(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	fromCorpusID string, fromDocID uuid.UUID, toCorpusID, edgeType string, toRefID uuid.UUID,
) uuid.UUID {
	t.Helper()
	const q = `
		INSERT INTO graph.relation_edge (from_corpus_id, from_document_id, to_ref_id, to_corpus_id, edge_type)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`
	var id uuid.UUID
	err := pool.QueryRow(ctx, q, fromCorpusID, fromDocID, toRefID, toCorpusID, edgeType).Scan(&id)
	if err != nil {
		t.Fatalf("inserting chain edge (from %s/%s to %s edge_type %s): %v",
			fromCorpusID, fromDocID, toCorpusID, edgeType, err)
	}
	return id
}

// TestGraphChainWalksOrderedHopsUnderLocalRole is Task 9's integration
// headline: GraphRepo.Chain, walking a REAL seeded SOP -> Policy -> Group ->
// law chain under mise_local (which sees every tier), returns exactly those
// three hops, in order, each resolving to the real document the fixture
// wrote and carrying the edge_type that produced it.
func TestGraphChainWalksOrderedHopsUnderLocalRole(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	fx := newGraphChainFixture(t, ctx, pool)
	repo := store.NewGraphRepo(pool)

	hops, err := repo.Chain(ctx, "mise_local", fx.sopRef, 0)
	if err != nil {
		t.Fatalf("Chain(mise_local) error = %v", err)
	}
	if len(hops) != 3 {
		t.Fatalf("Chain(mise_local) hops = %d, want 3", len(hops))
	}

	wantRefs := []graph.NodeRef{fx.policyRef, fx.groupRef, fx.lawRef}
	wantEdgeTypes := []graph.EdgeType{graph.EdgeDerives, graph.EdgeImplements, graph.EdgeSatisfies}
	for i, hop := range hops {
		if hop.Ref.CorpusID != wantRefs[i].CorpusID || hop.Ref.DocumentID != wantRefs[i].DocumentID {
			t.Errorf("hops[%d].Ref = %+v, want %+v", i, hop.Ref, wantRefs[i])
		}
		if hop.CorpusID != wantRefs[i].CorpusID {
			t.Errorf("hops[%d].CorpusID = %q, want %q", i, hop.CorpusID, wantRefs[i].CorpusID)
		}
		if hop.EdgeType != string(wantEdgeTypes[i]) {
			t.Errorf("hops[%d].EdgeType = %q, want %q", i, hop.EdgeType, wantEdgeTypes[i])
		}
	}
}

// TestGraphChainGroupRoleStopsAtLocalConfidentialBoundary is Task 9's
// tier-filtering-through-the-walk assertion: every edge sourced from
// local-sop/local-policy is local-confidential regardless of its target
// (graph.corpus_tier's hardcoded local case, migrations/009_graph_tables.sql,
// combined with graph.stricter_tier always taking the stricter side), so
// mise_group's walk over the SAME fixture — starting at the same
// local-confidential sopRef — sees nothing at all: the walk stops right
// where local-confidential begins, at hop zero, with no error (a hop the
// role can't see ends the walk cleanly).
func TestGraphChainGroupRoleStopsAtLocalConfidentialBoundary(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	fx := newGraphChainFixture(t, ctx, pool)
	repo := store.NewGraphRepo(pool)

	hops, err := repo.Chain(ctx, "mise_group", fx.sopRef, 0)
	if err != nil {
		t.Fatalf("Chain(mise_group) error = %v", err)
	}
	if len(hops) != 0 {
		t.Fatalf("Chain(mise_group) hops = %d, want 0 (local-confidential begins at the very first edge)", len(hops))
	}
}

// TestGraphChainGroupRoleSeesFromGroupOnward complements the boundary test
// above: mise_group is not blocked from the whole graph, only from the
// local-confidential portion of it. Starting the SAME fixture's walk one
// level up, at groupRef (group-confidential), mise_group sees the
// group-std -> my-reg hop (access_tier = stricter(group-confidential,
// public) = group-confidential) — proving the boundary is exactly at
// local-confidential, not a blanket denial.
func TestGraphChainGroupRoleSeesFromGroupOnward(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	fx := newGraphChainFixture(t, ctx, pool)
	repo := store.NewGraphRepo(pool)

	hops, err := repo.Chain(ctx, "mise_group", fx.groupRef, 0)
	if err != nil {
		t.Fatalf("Chain(mise_group) from groupRef error = %v", err)
	}
	if len(hops) != 1 {
		t.Fatalf("Chain(mise_group) from groupRef hops = %d, want 1", len(hops))
	}
	if hops[0].Ref.CorpusID != fx.lawRef.CorpusID || hops[0].Ref.DocumentID != fx.lawRef.DocumentID {
		t.Errorf("hops[0].Ref = %+v, want %+v", hops[0].Ref, fx.lawRef)
	}
}
