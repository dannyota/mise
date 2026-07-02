//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"danny.vn/mise/internal/testdb"
	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/store"
)

// graphReadFixture is one node (fromRef, in group-std) with two outgoing
// edges at different tiers: groupEdgeID (group-std -> my-reg, access_tier =
// group-confidential) and localEdgeID (group-std -> local-policy,
// access_tier = local-confidential) — proving GetNode's tier filtering is
// per-edge, not merely per-node: mise_group must see groupEdgeID but not
// localEdgeID from the very same GetNode call. Seeded as the pool's
// connecting owner role (bypasses RLS on write), mirroring
// newGraphRLSFixture's style (graph_rls_test.go) — raw SQL, not
// GraphStore.WriteExtractedEdge, since (like that fixture) this one needs
// each row's id back to assert per-role visibility, and needs two edges to
// share one from_document_id, which WriteExtractedEdge's ExtractedEdge shape
// doesn't let a caller pin directly.
type graphReadFixture struct {
	fromRef                          graph.NodeRef
	groupEdgeID, localEdgeID         uuid.UUID
	groupEvidenceID, localEvidenceID uuid.UUID
	myRegRefID, localPolicyRefID     uuid.UUID
}

// newGraphReadFixture seeds the fixture described above. doc_refs are
// inserted before their edges since relation_edge_set_to_corpus
// (migrations/009_graph_tables.sql) reads the doc_ref row to resolve
// to_corpus_id — same ordering newGraphRLSFixture uses.
func newGraphReadFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) graphReadFixture {
	t.Helper()

	fromDocID := uuid.New()
	fromRef := graph.NodeRef{CorpusID: "group-std", DocumentID: fromDocID}

	myRegRef := insertDocRef(t, ctx, pool, "my-reg", "my-reg:read-"+uuid.NewString())
	localRef := insertDocRef(t, ctx, pool, "local-policy", "local-policy:read-"+uuid.NewString())

	groupEdgeID, groupTier := insertEdgeFrom(t, ctx, pool, "group-std", fromDocID, "my-reg", myRegRef.ID)
	if groupTier != graph.TierGroupConfidential {
		t.Fatalf("fixture group-std->my-reg edge access_tier = %q, want %q", groupTier, graph.TierGroupConfidential)
	}
	localEdgeID, localTier := insertEdgeFrom(t, ctx, pool, "group-std", fromDocID, "local-policy", localRef.ID)
	if localTier != graph.TierLocalConfidential {
		t.Fatalf("fixture group-std->local-policy edge access_tier = %q, want %q", localTier, graph.TierLocalConfidential)
	}

	return graphReadFixture{
		fromRef:          fromRef,
		groupEdgeID:      groupEdgeID,
		localEdgeID:      localEdgeID,
		groupEvidenceID:  insertEvidenceForRLS(t, ctx, pool, groupEdgeID),
		localEvidenceID:  insertEvidenceForRLS(t, ctx, pool, localEdgeID),
		myRegRefID:       myRegRef.ID,
		localPolicyRefID: localRef.ID,
	}
}

// insertEdgeFrom inserts a graph.relation_edge row from a caller-supplied
// fromDocID and returns its id and Postgres-computed access_tier. Unlike
// graph_rls_test.go's insertEdgeForRLS (which always mints its own
// uuid.New() from_document_id), GetNode's fixture needs two edges sharing
// the SAME from_document_id — one node's two outgoing edges — so this
// helper takes fromDocID explicitly. edge_type is fixed to "satisfies" —
// irrelevant to tier filtering, which keys only on access_tier.
func insertEdgeFrom(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool, fromCorpusID string, fromDocID uuid.UUID,
	toCorpusID string, toRefID uuid.UUID,
) (uuid.UUID, graph.Tier) {
	t.Helper()
	const q = `
		INSERT INTO graph.relation_edge (from_corpus_id, from_document_id, to_ref_id, to_corpus_id, edge_type)
		VALUES ($1, $2, $3, $4, 'satisfies')
		RETURNING id, access_tier`
	var id uuid.UUID
	var tier string
	err := pool.QueryRow(ctx, q, fromCorpusID, fromDocID, toRefID, toCorpusID).Scan(&id, &tier)
	if err != nil {
		t.Fatalf("inserting graph.relation_edge (from %s/%s to %s): %v", fromCorpusID, fromDocID, toCorpusID, err)
	}
	return id, graph.Tier(tier)
}

// insertSectionScopedEdge inserts a graph.relation_edge row with a non-NULL
// from_section_id — TestGetNodeSectionIDNarrowsToThatSectionOnly's fixture
// needs a section-scoped edge to sit alongside a document-level one
// (insertEdgeFrom's from_section_id is always NULL) on the same document.
func insertSectionScopedEdge(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	fromCorpusID string, fromDocID, fromSectionID uuid.UUID, toCorpusID string, toRefID uuid.UUID,
) uuid.UUID {
	t.Helper()
	const q = `
		INSERT INTO graph.relation_edge
			(from_corpus_id, from_document_id, from_section_id, to_ref_id, to_corpus_id, edge_type)
		VALUES ($1, $2, $3, $4, $5, 'satisfies')
		RETURNING id`
	var id uuid.UUID
	err := pool.QueryRow(ctx, q, fromCorpusID, fromDocID, fromSectionID, toRefID, toCorpusID).Scan(&id)
	if err != nil {
		t.Fatalf("inserting section-scoped graph.relation_edge: %v", err)
	}
	return id
}

// TestGetNodeRLSTierFiltering is Task 8's headline suite: GetNode's tier
// filtering must hold edge by edge, not just node by node, on the very same
// fromRef — mirroring TestGraphRLSDenySuite's "one fixture, one t.Run per
// role" shape (graph_rls_test.go) so this driver stays under the
// cognitive-complexity lint budget.
func TestGetNodeRLSTierFiltering(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	fx := newGraphReadFixture(t, ctx, pool)
	repo := store.NewGraphRepo(pool)

	t.Run("mise_local sees both tiers, with to_ref resolved and evidence attached", func(t *testing.T) {
		assertGetNodeLocalSeesBothEdges(t, ctx, repo, fx)
	})
	t.Run("mise_group sees the group-confidential edge but not the local-confidential one", func(t *testing.T) {
		assertGetNodeGroupSeesGroupNotLocal(t, ctx, repo, fx)
	})
	t.Run("mise_public sees no edges at all: ErrNodeNotFound", func(t *testing.T) {
		assertGetNodePublicNotFound(t, ctx, repo, fx)
	})
}

// assertGetNodeLocalSeesBothEdges is TestGetNodeRLSTierFiltering's mise_local
// case: both edges come back, each with its to_ref (ToRefID/ToCorpusID)
// resolved and its own evidence row attached under the right edge id.
func assertGetNodeLocalSeesBothEdges(t *testing.T, ctx context.Context, repo *store.GraphRepo, fx graphReadFixture) {
	t.Helper()
	view, err := repo.GetNode(ctx, "mise_local", fx.fromRef)
	if err != nil {
		t.Fatalf("GetNode(mise_local) error = %v", err)
	}
	if len(view.Edges) != 2 {
		t.Fatalf("GetNode(mise_local) edges = %d, want 2", len(view.Edges))
	}

	byID := make(map[uuid.UUID]graph.Edge, len(view.Edges))
	for _, e := range view.Edges {
		byID[e.ID] = e
	}

	groupEdge, ok := byID[fx.groupEdgeID]
	if !ok {
		t.Fatalf("GetNode(mise_local) missing group-confidential edge %v", fx.groupEdgeID)
	}
	if groupEdge.ToRefID != fx.myRegRefID || groupEdge.ToCorpusID != "my-reg" {
		t.Errorf("group edge to_ref = (%v, %s), want (%v, my-reg)", groupEdge.ToRefID, groupEdge.ToCorpusID, fx.myRegRefID)
	}
	if groupEdge.AccessTier != graph.TierGroupConfidential {
		t.Errorf("group edge access_tier = %q, want %q", groupEdge.AccessTier, graph.TierGroupConfidential)
	}

	localEdge, ok := byID[fx.localEdgeID]
	if !ok {
		t.Fatalf("GetNode(mise_local) missing local-confidential edge %v", fx.localEdgeID)
	}
	if localEdge.ToRefID != fx.localPolicyRefID || localEdge.ToCorpusID != "local-policy" {
		t.Errorf("local edge to_ref = (%v, %s), want (%v, local-policy)",
			localEdge.ToRefID, localEdge.ToCorpusID, fx.localPolicyRefID)
	}
	if localEdge.AccessTier != graph.TierLocalConfidential {
		t.Errorf("local edge access_tier = %q, want %q", localEdge.AccessTier, graph.TierLocalConfidential)
	}

	groupEv := view.Evidence[fx.groupEdgeID]
	if len(groupEv) != 1 || groupEv[0].ID != fx.groupEvidenceID {
		t.Errorf("Evidence[group edge] = %+v, want exactly one row with id %v", groupEv, fx.groupEvidenceID)
	}
	localEv := view.Evidence[fx.localEdgeID]
	if len(localEv) != 1 || localEv[0].ID != fx.localEvidenceID {
		t.Errorf("Evidence[local edge] = %+v, want exactly one row with id %v", localEv, fx.localEvidenceID)
	}
}

// assertGetNodeGroupSeesGroupNotLocal is TestGetNodeRLSTierFiltering's
// mise_group case: the group-confidential edge is visible, the
// local-confidential one is not — and its evidence must not leak through
// the Evidence map either.
func assertGetNodeGroupSeesGroupNotLocal(
	t *testing.T, ctx context.Context, repo *store.GraphRepo, fx graphReadFixture,
) {
	t.Helper()
	view, err := repo.GetNode(ctx, "mise_group", fx.fromRef)
	if err != nil {
		t.Fatalf("GetNode(mise_group) error = %v", err)
	}
	if len(view.Edges) != 1 {
		t.Fatalf("GetNode(mise_group) edges = %d, want 1 (only the group-confidential edge)", len(view.Edges))
	}
	if view.Edges[0].ID != fx.groupEdgeID {
		t.Errorf("GetNode(mise_group) edge = %v, want %v (group-confidential)", view.Edges[0].ID, fx.groupEdgeID)
	}
	if _, ok := view.Evidence[fx.localEdgeID]; ok {
		t.Error("GetNode(mise_group) Evidence map leaks the local-confidential edge's evidence")
	}
}

// assertGetNodePublicNotFound is TestGetNodeRLSTierFiltering's mise_public
// case: both of fromRef's edges are confidential (group- and
// local-confidential), so mise_public must see neither — indistinguishable
// from fromRef not existing at all (ErrNodeNotFound).
func assertGetNodePublicNotFound(t *testing.T, ctx context.Context, repo *store.GraphRepo, fx graphReadFixture) {
	t.Helper()
	_, err := repo.GetNode(ctx, "mise_public", fx.fromRef)
	if !errors.Is(err, store.ErrNodeNotFound) {
		t.Fatalf("GetNode(mise_public) error = %v, want errors.Is(_, store.ErrNodeNotFound)", err)
	}
}

// TestGetNodeOnGenuinelyAbsentNodeReturnsErrNodeNotFound is the other half of
// the "indistinguishable" invariant assertGetNodePublicNotFound exercises: a
// document mise has never even heard of must fold to the exact same
// ErrNodeNotFound as a node whose edges are merely hidden by tier.
func TestGetNodeOnGenuinelyAbsentNodeReturnsErrNodeNotFound(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	repo := store.NewGraphRepo(pool)

	ref := graph.NodeRef{CorpusID: "vn-reg", DocumentID: uuid.New()}
	_, err := repo.GetNode(ctx, "mise_local", ref)
	if !errors.Is(err, store.ErrNodeNotFound) {
		t.Fatalf("GetNode(mise_local) on an absent node error = %v, want errors.Is(_, store.ErrNodeNotFound)", err)
	}
}

// TestGetNodeRejectsInvalidRole proves role validation runs before any query
// (mirroring resolveRole's use in Search/GetDocument): an unrecognized role
// string must fail outright, not silently fold into ErrNodeNotFound.
func TestGetNodeRejectsInvalidRole(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	repo := store.NewGraphRepo(pool)

	ref := graph.NodeRef{CorpusID: "vn-reg", DocumentID: uuid.New()}
	_, err := repo.GetNode(ctx, "not-a-real-role", ref)
	if err == nil {
		t.Fatal("GetNode() with an invalid role error = nil, want error")
	}
	if errors.Is(err, store.ErrNodeNotFound) {
		t.Error("GetNode() with an invalid role should fail role validation, not fold into ErrNodeNotFound")
	}
}

// TestGetNodeSectionIDNarrowsToThatSectionOnly pins GetNode's from_section_id
// matching rule: ref.SectionID nil matches every edge from the document
// (section-scoped or not); non-nil narrows to exactly that section's edges,
// excluding document-level ones. Both edges are public-tier (vn-reg ->
// my-reg) so tier filtering never enters into it — this test is purely
// about the from_section_id predicate. The two edges target DIFFERENT
// doc_refs: relation_edge_uq is (from_corpus_id, from_document_id, to_ref_id,
// edge_type) — it does not include from_section_id, so two edges from the
// same document to the SAME target would collide regardless of section.
func TestGetNodeSectionIDNarrowsToThatSectionOnly(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	repo := store.NewGraphRepo(pool)

	fromDocID := uuid.New()
	sectionID := uuid.New()
	docLevelRef := insertDocRef(t, ctx, pool, "my-reg", "my-reg:sec-doc-"+uuid.NewString())
	sectionRef := insertDocRef(t, ctx, pool, "my-reg", "my-reg:sec-scoped-"+uuid.NewString())

	if _, tier := insertEdgeFrom(t, ctx, pool, "vn-reg", fromDocID, "my-reg", docLevelRef.ID); tier != graph.TierPublic {
		t.Fatalf("fixture document-level edge access_tier = %q, want %q", tier, graph.TierPublic)
	}
	sectionEdgeID := insertSectionScopedEdge(t, ctx, pool, "vn-reg", fromDocID, sectionID, "my-reg", sectionRef.ID)

	whole, err := repo.GetNode(ctx, "mise_public", graph.NodeRef{CorpusID: "vn-reg", DocumentID: fromDocID})
	if err != nil {
		t.Fatalf("GetNode(SectionID=nil) error = %v", err)
	}
	if len(whole.Edges) != 2 {
		t.Fatalf("GetNode(SectionID=nil) edges = %d, want 2 (document-level + section-scoped)", len(whole.Edges))
	}

	scopedRef := graph.NodeRef{CorpusID: "vn-reg", DocumentID: fromDocID, SectionID: &sectionID}
	scoped, err := repo.GetNode(ctx, "mise_public", scopedRef)
	if err != nil {
		t.Fatalf("GetNode(SectionID=%v) error = %v", sectionID, err)
	}
	if len(scoped.Edges) != 1 || scoped.Edges[0].ID != sectionEdgeID {
		t.Fatalf("GetNode(SectionID=%v) edges = %+v, want exactly [%v]", sectionID, scoped.Edges, sectionEdgeID)
	}
}

// TestGetNodeReadsBackWhatWriteExtractedEdgeWrote is the write/read
// round-trip across the milestone's two halves: GraphStore.WriteExtractedEdge
// (Task 6, owner-side) writes one edge plus its extracted-evidence row;
// GetNode (this task, RLS-scoped) must read the identical edge/evidence back
// for a role whose tier admits it. edge.From is local-sop (always
// local-confidential — corpus_tier's ELSE/max-rank case), so mise_local is
// the role that must see it.
func TestGetNodeReadsBackWhatWriteExtractedEdgeWrote(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	g := store.NewGraphStore(pool)
	repo := store.NewGraphRepo(pool)
	edge := extractedFixture(t)

	edgeID, err := g.WriteExtractedEdge(ctx, edge)
	if err != nil {
		t.Fatalf("WriteExtractedEdge() error = %v", err)
	}

	view, err := repo.GetNode(ctx, "mise_local", edge.From)
	if err != nil {
		t.Fatalf("GetNode(mise_local) error = %v", err)
	}
	if len(view.Edges) != 1 || view.Edges[0].ID != edgeID {
		t.Fatalf("GetNode(mise_local) edges = %+v, want exactly [%v]", view.Edges, edgeID)
	}
	if view.Edges[0].EdgeType != graph.EdgeDerives {
		t.Errorf("edge_type = %q, want %q", view.Edges[0].EdgeType, graph.EdgeDerives)
	}

	ev := view.Evidence[edgeID]
	if len(ev) != 1 {
		t.Fatalf("Evidence[%v] = %+v, want exactly one row", edgeID, ev)
	}
	if ev[0].EvidenceKind != graph.EvidenceExtracted || ev[0].QuotedFromSpan != edge.QuotedFromSpan {
		t.Errorf("evidence = %+v, want evidence_kind=%q quoted_from_span=%q",
			ev[0], graph.EvidenceExtracted, edge.QuotedFromSpan)
	}
}
