package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"danny.vn/mise/pkg/graph"
)

// fakeChainSource is walkChain's unit-test double for the chainSource seam:
// an in-memory adjacency keyed by nodeKey, standing in for GetNode's
// tier-filtered Postgres read and resolveDocRef's doc_ref lookup, so the
// bound/cycle-guard logic in walkChain — Chain's whole point — is exercised
// with no database. A ref/refID absent from the relevant map behaves
// exactly like GetNode/resolveDocRef's real ErrNodeNotFound: "a hop the role
// can't see" and "this node doesn't exist at all" are indistinguishable
// there too (graph_read.go's ErrNodeNotFound doc comment), so the fake
// doesn't need to model them separately.
type fakeChainSource struct {
	edges    map[string]graph.Edge
	targets  map[uuid.UUID]docRefTarget
	evidence map[uuid.UUID][]graph.Evidence
}

func newFakeChainSource() *fakeChainSource {
	return &fakeChainSource{
		edges:    map[string]graph.Edge{},
		targets:  map[uuid.UUID]docRefTarget{},
		evidence: map[uuid.UUID][]graph.Evidence{},
	}
}

// GetNode satisfies chainSource: ref's one fixture edge (if any), plus its
// evidence.
func (f *fakeChainSource) GetNode(_ context.Context, _ string, ref graph.NodeRef) (NodeView, error) {
	edge, ok := f.edges[nodeKey(ref)]
	if !ok {
		return NodeView{}, fmt.Errorf("fake GetNode(%s): %w", nodeKey(ref), ErrNodeNotFound)
	}
	return NodeView{
		Ref:      ref,
		Edges:    []graph.Edge{edge},
		Evidence: map[uuid.UUID][]graph.Evidence{edge.ID: f.evidence[edge.ID]},
	}, nil
}

// resolveDocRef satisfies chainSource: refID's fixture target, if any.
func (f *fakeChainSource) resolveDocRef(_ context.Context, _ string, refID uuid.UUID) (docRefTarget, error) {
	t, ok := f.targets[refID]
	if !ok {
		return docRefTarget{}, fmt.Errorf("fake resolveDocRef(%s): %w", refID, ErrNodeNotFound)
	}
	return t, nil
}

// link records one up-edge from "from" to a freshly minted node in
// corpusID, and returns that node's NodeRef so callers can chain further
// links from it — the unit tests' way of building a linear fixture chain
// with no database. promoted/confidence/groundingScore seed a single
// evidence row, so a test can assert those values survive into the Hop.
func (f *fakeChainSource) link(
	from graph.NodeRef, corpusID, edgeType string, promoted bool, confidence, groundingScore float64,
) graph.NodeRef {
	to := graph.NodeRef{CorpusID: corpusID, DocumentID: uuid.New()}
	f.linkTo(from, to, corpusID, edgeType, promoted, confidence, groundingScore)
	return to
}

// linkTo records one up-edge from "from" to the caller-supplied "to" ref —
// unlike link (which always mints a fresh target), this lets a test point
// an edge at an EXISTING ref, the shape a cycle fixture needs (e.g. wiring a
// node back to the walk's own start).
func (f *fakeChainSource) linkTo(
	from, to graph.NodeRef, corpusID, edgeType string, promoted bool, confidence, groundingScore float64,
) {
	refID, edgeID := uuid.New(), uuid.New()
	f.edges[nodeKey(from)] = graph.Edge{
		ID: edgeID, From: from, ToRefID: refID, ToCorpusID: corpusID,
		EdgeType: graph.EdgeType(edgeType), Direction: directionUp, Promoted: promoted,
	}
	docID := to.DocumentID
	f.targets[refID] = docRefTarget{
		CorpusID: corpusID, DocID: &docID, SecID: to.SectionID,
		Label: corpusID + "-label", RefKey: corpusID + "-ref",
	}
	f.evidence[edgeID] = []graph.Evidence{{
		ID: uuid.New(), EdgeID: edgeID, EvidenceKind: graph.EvidenceModelClassification,
		Confidence: confidence, GroundingScore: groundingScore,
	}}
}

// TestWalkChainLinearChainReturnsOrderedHops is the straightforward case: a
// 4-hop linear chain (mirroring SOP -> Policy -> Group -> law's authority
// direction) returns exactly those 4 hops, in order, each carrying its own
// edge_type/corpus/citation, and the promoted/confidence/grounding_score the
// fixture seeded.
func TestWalkChainLinearChainReturnsOrderedHops(t *testing.T) {
	start := graph.NodeRef{CorpusID: "local-sop", DocumentID: uuid.New()}
	f := newFakeChainSource()
	n1 := f.link(start, "local-policy", "derives", false, 1.0, 0)
	n2 := f.link(n1, "group-std", "implements", false, 0.8, 0.7)
	n3 := f.link(n2, "my-reg", "satisfies", true, 0.95, 0.9)
	_ = f.link(n3, "vn-reg", "covers", false, 0.5, 0.4)

	hops, err := walkChain(context.Background(), f, "mise_local", start, 0)
	if err != nil {
		t.Fatalf("walkChain() error = %v", err)
	}
	if len(hops) != 4 {
		t.Fatalf("walkChain() hops = %d, want 4", len(hops))
	}

	wantCorpus := []string{"local-policy", "group-std", "my-reg", "vn-reg"}
	wantEdgeType := []string{"derives", "implements", "satisfies", "covers"}
	for i, hop := range hops {
		if hop.CorpusID != wantCorpus[i] {
			t.Errorf("hops[%d].CorpusID = %q, want %q", i, hop.CorpusID, wantCorpus[i])
		}
		if hop.EdgeType != wantEdgeType[i] {
			t.Errorf("hops[%d].EdgeType = %q, want %q", i, hop.EdgeType, wantEdgeType[i])
		}
		if hop.Citation == "" {
			t.Errorf("hops[%d].Citation is empty, want the target's label/ref_key", i)
		}
	}
	if !hops[2].Promoted {
		t.Error("hops[2].Promoted = false, want true (the my-reg hop was linked promoted)")
	}
	if hops[2].Confidence != 0.95 || hops[2].GroundingScore != 0.9 {
		t.Errorf("hops[2] confidence/grounding = %v/%v, want 0.95/0.9", hops[2].Confidence, hops[2].GroundingScore)
	}
}

// TestWalkChainTruncatesAtMaxChainDepth proves the depth bound: a chain far
// longer than MaxChainDepth still returns at most MaxChainDepth hops — the
// walk simply stops, no error — when maxDepth is left at its default (0,
// which clampDepth folds UP to MaxChainDepth, not down to 0).
func TestWalkChainTruncatesAtMaxChainDepth(t *testing.T) {
	start := graph.NodeRef{CorpusID: "corpus-0", DocumentID: uuid.New()}
	f := newFakeChainSource()
	current := start
	const built = MaxChainDepth + 5
	for i := 1; i <= built; i++ {
		current = f.link(current, fmt.Sprintf("corpus-%d", i), "derives", false, 1.0, 1.0)
	}

	hops, err := walkChain(context.Background(), f, "mise_local", start, 0)
	if err != nil {
		t.Fatalf("walkChain() error = %v", err)
	}
	if len(hops) != MaxChainDepth {
		t.Fatalf("walkChain() hops = %d, want %d (MaxChainDepth)", len(hops), MaxChainDepth)
	}
	wantLast := fmt.Sprintf("corpus-%d", MaxChainDepth)
	if hops[MaxChainDepth-1].CorpusID != wantLast {
		t.Errorf("last hop CorpusID = %q, want %q (stopped exactly at the cap)",
			hops[MaxChainDepth-1].CorpusID, wantLast)
	}
}

// TestWalkChainRespectsSmallerExplicitMaxDepth proves a caller-supplied
// maxDepth below the cap is honored as-is (not silently widened to
// MaxChainDepth) — clampDepth only ever pulls a too-large value DOWN or an
// unset one UP to the cap, never the reverse.
func TestWalkChainRespectsSmallerExplicitMaxDepth(t *testing.T) {
	start := graph.NodeRef{CorpusID: "corpus-0", DocumentID: uuid.New()}
	f := newFakeChainSource()
	n1 := f.link(start, "corpus-1", "derives", false, 1, 1)
	n2 := f.link(n1, "corpus-2", "derives", false, 1, 1)
	f.link(n2, "corpus-3", "derives", false, 1, 1)

	hops, err := walkChain(context.Background(), f, "mise_local", start, 2)
	if err != nil {
		t.Fatalf("walkChain() error = %v", err)
	}
	if len(hops) != 2 {
		t.Fatalf("walkChain(maxDepth=2) hops = %d, want 2", len(hops))
	}
}

// TestWalkChainCycleTerminatesViaVisitedGuard is the headline test: a
// 2-node cycle (A -> B -> A) must never run away. The walk appends exactly
// one hop (A -> B) and then stops the moment B's own edge would revisit A —
// well short of MaxChainDepth, and with no error.
func TestWalkChainCycleTerminatesViaVisitedGuard(t *testing.T) {
	a := graph.NodeRef{CorpusID: "local-sop", DocumentID: uuid.New()}
	f := newFakeChainSource()
	b := f.link(a, "local-policy", "derives", false, 1.0, 1.0)
	f.linkTo(b, a, "local-sop", "derives", false, 1.0, 1.0) // B -> A closes the cycle

	hops, err := walkChain(context.Background(), f, "mise_local", a, 0)
	if err != nil {
		t.Fatalf("walkChain() error = %v", err)
	}
	if len(hops) != 1 {
		t.Fatalf("walkChain() on a 2-node cycle returned %d hops, want exactly 1 (A->B, then stop)", len(hops))
	}
	if hops[0].CorpusID != "local-policy" {
		t.Errorf("hops[0].CorpusID = %q, want %q", hops[0].CorpusID, "local-policy")
	}
}

// TestWalkChainSelfLoopTerminatesImmediately is the cycle guard's extreme
// case: an edge from start directly back to itself must produce zero hops,
// not one — the very first candidate next-node is already visited (start
// itself, seeded into the visited set up front).
func TestWalkChainSelfLoopTerminatesImmediately(t *testing.T) {
	a := graph.NodeRef{CorpusID: "local-sop", DocumentID: uuid.New()}
	f := newFakeChainSource()
	f.linkTo(a, a, "local-sop", "derives", false, 1.0, 1.0)

	hops, err := walkChain(context.Background(), f, "mise_local", a, 0)
	if err != nil {
		t.Fatalf("walkChain() error = %v", err)
	}
	if len(hops) != 0 {
		t.Fatalf("walkChain() on a self-loop returned %d hops, want 0", len(hops))
	}
}

// TestWalkChainStartInvisibleToRoleEndsCleanly proves "a hop the role can't
// see ends the walk cleanly, not an error" holds even at the very first
// step: a start node absent from the fake (GetNode's ErrNodeNotFound
// equivalent) yields zero hops and a nil error.
func TestWalkChainStartInvisibleToRoleEndsCleanly(t *testing.T) {
	start := graph.NodeRef{CorpusID: "local-sop", DocumentID: uuid.New()}
	f := newFakeChainSource() // nothing linked: GetNode(start) reports ErrNodeNotFound

	hops, err := walkChain(context.Background(), f, "mise_public", start, 0)
	if err != nil {
		t.Fatalf("walkChain() error = %v, want nil", err)
	}
	if len(hops) != 0 {
		t.Fatalf("walkChain() hops = %d, want 0", len(hops))
	}
}

// TestWalkChainNoUpEdgeEndsChain proves a node with edges, but none of
// direction "up", ends the chain the same clean way (firstUpEdge's ok=false
// path) — distinct from GetNode returning ErrNodeNotFound outright.
func TestWalkChainNoUpEdgeEndsChain(t *testing.T) {
	start := graph.NodeRef{CorpusID: "local-sop", DocumentID: uuid.New()}
	f := newFakeChainSource()
	f.edges[nodeKey(start)] = graph.Edge{ID: uuid.New(), From: start, Direction: "down"}

	hops, err := walkChain(context.Background(), f, "mise_local", start, 0)
	if err != nil {
		t.Fatalf("walkChain() error = %v", err)
	}
	if len(hops) != 0 {
		t.Fatalf("walkChain() hops = %d, want 0 (no up-edge to follow)", len(hops))
	}
}

// TestWalkChainUnresolvedDocRefStubEndsChain proves an edge whose target
// doc_ref is still an unresolved stub (DocID nil — the citation hasn't been
// matched to an ingested document yet) ends the walk cleanly rather than
// dereferencing a nil DocumentID.
func TestWalkChainUnresolvedDocRefStubEndsChain(t *testing.T) {
	start := graph.NodeRef{CorpusID: "local-sop", DocumentID: uuid.New()}
	f := newFakeChainSource()
	refID, edgeID := uuid.New(), uuid.New()
	f.edges[nodeKey(start)] = graph.Edge{
		ID: edgeID, From: start, ToRefID: refID, ToCorpusID: "local-policy",
		EdgeType: graph.EdgeDerives, Direction: directionUp,
	}
	f.targets[refID] = docRefTarget{CorpusID: "local-policy", DocID: nil, RefKey: "unresolved-ref"}

	hops, err := walkChain(context.Background(), f, "mise_local", start, 0)
	if err != nil {
		t.Fatalf("walkChain() error = %v", err)
	}
	if len(hops) != 0 {
		t.Fatalf("walkChain() hops = %d, want 0 (unresolved doc_ref stub)", len(hops))
	}
}

// TestBestEvidencePicksZeroConfidenceRowsGroundingScoreToo guards the
// bestEvidence edge case: a single evidence row whose Confidence is exactly
// 0 must still contribute its own GroundingScore — a naive "seed from the
// zero value, keep only strictly-greater Confidence" comparison would tie
// against that seed and silently drop it.
func TestBestEvidencePicksZeroConfidenceRowsGroundingScoreToo(t *testing.T) {
	ev := []graph.Evidence{{Confidence: 0, GroundingScore: 0.7}}
	got := bestEvidence(ev)
	if got.GroundingScore != 0.7 {
		t.Errorf("bestEvidence(single zero-confidence row).GroundingScore = %v, want 0.7", got.GroundingScore)
	}
}

// TestClampDepth pins clampDepth's bound: non-positive or above-cap inputs
// fold to MaxChainDepth; anything in between passes through unchanged.
func TestClampDepth(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, MaxChainDepth},
		{-1, MaxChainDepth},
		{-100, MaxChainDepth},
		{1, 1},
		{3, 3},
		{MaxChainDepth, MaxChainDepth},
		{MaxChainDepth + 1, MaxChainDepth},
		{1000, MaxChainDepth},
	}
	for _, tc := range cases {
		if got := clampDepth(tc.in); got != tc.want {
			t.Errorf("clampDepth(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestChainRejectsInvalidRole proves Chain validates role before doing any
// work, mirroring GetNode/Search's own fail-fast role check — an
// unrecognized role must error outright, never silently produce zero hops.
// A nil pool is safe here: resolveRole fails before Chain ever touches it.
func TestChainRejectsInvalidRole(t *testing.T) {
	repo := NewGraphRepo(nil)
	_, err := repo.Chain(context.Background(), "not-a-real-role", graph.NodeRef{}, 0)
	if err == nil {
		t.Fatal("Chain() with an invalid role error = nil, want error")
	}
}
