package mcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/store"
)

// stubGraphRepo is a GraphRepoIface test double: it records the args each
// method was last called with and returns fixed view/hops/err pairs —
// mirrors stubSearcher/stubDocGetter (tools_test.go).
type stubGraphRepo struct {
	getNodeCalls int
	getNodeRole  string
	getNodeRef   graph.NodeRef
	view         store.NodeView
	getNodeErr   error

	chainCalls int
	chainRole  string
	chainStart graph.NodeRef
	chainDepth int
	hops       []store.Hop
	chainErr   error
}

func (s *stubGraphRepo) GetNode(_ context.Context, role string, ref graph.NodeRef) (store.NodeView, error) {
	s.getNodeCalls++
	s.getNodeRole, s.getNodeRef = role, ref
	return s.view, s.getNodeErr
}

func (s *stubGraphRepo) Chain(_ context.Context, role string, start graph.NodeRef, maxDepth int) ([]store.Hop, error) {
	s.chainCalls++
	s.chainRole, s.chainStart, s.chainDepth = role, start, maxDepth
	return s.hops, s.chainErr
}

// --- graph tool: defaults + validation -----------------------------------

// TestGraphHandlerAppliesDefaults proves depth defaults to store.
// MaxChainDepth and direction defaults to "up" when both are omitted — the
// direction default is only observable through the edges it lets through,
// since direction is a client-side filter over GetNode's edges, never a
// parameter passed to the repo itself.
func TestGraphHandlerAppliesDefaults(t *testing.T) {
	ref := graph.NodeRef{CorpusID: string(corpus.VNReg), DocumentID: uuid.New()}
	upEdge := graph.Edge{ID: uuid.New(), From: ref, EdgeType: graph.EdgeSatisfies, Direction: "up"}
	downEdge := graph.Edge{ID: uuid.New(), From: ref, EdgeType: graph.EdgeSatisfies, Direction: "down"}
	stub := &stubGraphRepo{view: store.NodeView{Ref: ref, Edges: []graph.Edge{upEdge, downEdge}}}
	h := newGraphHandler(stub, "mise_public")

	_, out, err := h(context.Background(), nil, GraphInput{NodeRef: nodeRefWire(ref)})
	if err != nil {
		t.Fatalf("handler error = %v, want nil", err)
	}
	if stub.chainDepth != store.MaxChainDepth {
		t.Errorf("Chain depth = %d, want %d (default)", stub.chainDepth, store.MaxChainDepth)
	}
	if len(out.Edges) != 1 || out.Edges[0].ID != upEdge.ID.String() {
		t.Errorf("out.Edges = %+v, want exactly the up-direction edge (default direction=up)", out.Edges)
	}
}

// TestGraphHandlerHonoursExplicitDepthAndDirection proves an explicit depth
// and direction override the defaults.
func TestGraphHandlerHonoursExplicitDepthAndDirection(t *testing.T) {
	ref := graph.NodeRef{CorpusID: string(corpus.VNReg), DocumentID: uuid.New()}
	upEdge := graph.Edge{ID: uuid.New(), From: ref, EdgeType: graph.EdgeSatisfies, Direction: "up"}
	downEdge := graph.Edge{ID: uuid.New(), From: ref, EdgeType: graph.EdgeSatisfies, Direction: "down"}
	stub := &stubGraphRepo{view: store.NodeView{Ref: ref, Edges: []graph.Edge{upEdge, downEdge}}}
	h := newGraphHandler(stub, "mise_public")

	_, out, err := h(context.Background(), nil, GraphInput{NodeRef: nodeRefWire(ref), Direction: "down", Depth: 3})
	if err != nil {
		t.Fatalf("handler error = %v, want nil", err)
	}
	if stub.chainDepth != 3 {
		t.Errorf("Chain depth = %d, want 3 (explicit)", stub.chainDepth)
	}
	if len(out.Edges) != 1 || out.Edges[0].ID != downEdge.ID.String() {
		t.Errorf("out.Edges = %+v, want exactly the down-direction edge (explicit direction=down)", out.Edges)
	}
}

// TestGraphHandlerFiltersByEdgeTypes proves edge_types narrows the returned
// edges to only the requested types.
func TestGraphHandlerFiltersByEdgeTypes(t *testing.T) {
	ref := graph.NodeRef{CorpusID: string(corpus.LocalPolicy), DocumentID: uuid.New()}
	satisfies := graph.Edge{ID: uuid.New(), From: ref, EdgeType: graph.EdgeSatisfies, Direction: "up"}
	implements := graph.Edge{ID: uuid.New(), From: ref, EdgeType: graph.EdgeImplements, Direction: "up"}
	stub := &stubGraphRepo{view: store.NodeView{Ref: ref, Edges: []graph.Edge{satisfies, implements}}}
	h := newGraphHandler(stub, "mise_local")

	in := GraphInput{NodeRef: nodeRefWire(ref), EdgeTypes: []string{"implements"}}
	_, out, err := h(context.Background(), nil, in)
	if err != nil {
		t.Fatalf("handler error = %v, want nil", err)
	}
	if len(out.Edges) != 1 || out.Edges[0].EdgeType != "implements" {
		t.Errorf("out.Edges = %+v, want exactly the implements edge", out.Edges)
	}
}

func TestGraphHandlerRejectsInvalidNodeRef(t *testing.T) {
	tests := []struct {
		name    string
		nodeRef string
	}{
		{"empty", ""},
		{"missing document_id", string(corpus.VNReg)},
		{"bad document_id uuid", string(corpus.VNReg) + "/not-a-uuid"},
		{"too many parts", string(corpus.VNReg) + "/" + uuid.NewString() + "/" + uuid.NewString() + "/extra"},
		{"bad section_id uuid", string(corpus.VNReg) + "/" + uuid.NewString() + "/not-a-uuid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &stubGraphRepo{}
			h := newGraphHandler(stub, "mise_public")
			_, _, err := h(context.Background(), nil, GraphInput{NodeRef: tt.nodeRef})
			if err == nil {
				t.Fatalf("handler error = nil, want error for node_ref %q", tt.nodeRef)
			}
			if stub.getNodeCalls != 0 || stub.chainCalls != 0 {
				t.Errorf("repo called (GetNode=%d, Chain=%d), want 0 (validation should short-circuit)",
					stub.getNodeCalls, stub.chainCalls)
			}
		})
	}
}

func TestGraphHandlerRejectsUnknownCorpus(t *testing.T) {
	stub := &stubGraphRepo{}
	h := newGraphHandler(stub, "mise_public")

	_, _, err := h(context.Background(), nil, GraphInput{NodeRef: "not-a-corpus/" + uuid.NewString()})
	if err == nil {
		t.Fatal("handler error = nil, want error for unknown corpus")
	}
	if stub.getNodeCalls != 0 {
		t.Errorf("GetNode called %d times, want 0 (validation should short-circuit)", stub.getNodeCalls)
	}
}

func TestGraphHandlerRejectsInvalidDirection(t *testing.T) {
	stub := &stubGraphRepo{}
	h := newGraphHandler(stub, "mise_public")

	ref := graph.NodeRef{CorpusID: string(corpus.VNReg), DocumentID: uuid.New()}
	in := GraphInput{NodeRef: nodeRefWire(ref), Direction: "sideways"}
	_, _, err := h(context.Background(), nil, in)
	if err == nil {
		t.Fatal("handler error = nil, want error for invalid direction")
	}
	if stub.getNodeCalls != 0 {
		t.Errorf("GetNode called %d times, want 0 (validation should short-circuit)", stub.getNodeCalls)
	}
}

func TestGraphHandlerPropagatesGetNodeError(t *testing.T) {
	stub := &stubGraphRepo{getNodeErr: errors.New("boom")}
	h := newGraphHandler(stub, "mise_public")

	ref := graph.NodeRef{CorpusID: string(corpus.VNReg), DocumentID: uuid.New()}
	_, _, err := h(context.Background(), nil, GraphInput{NodeRef: nodeRefWire(ref)})
	if err == nil {
		t.Fatal("handler error = nil, want the wrapped GetNode error")
	}
	if stub.chainCalls != 0 {
		t.Errorf("Chain called %d times, want 0 (should not run after GetNode fails)", stub.chainCalls)
	}
}

// TestGraphHandlerPropagatesNodeNotFound mirrors
// TestDocumentHandlerPropagatesNotFound (tools_test.go): store.
// ErrNodeNotFound propagates as a typed, wrapped error, never silently
// swallowed into an empty result.
func TestGraphHandlerPropagatesNodeNotFound(t *testing.T) {
	stub := &stubGraphRepo{getNodeErr: store.ErrNodeNotFound}
	h := newGraphHandler(stub, "mise_public")

	ref := graph.NodeRef{CorpusID: string(corpus.VNReg), DocumentID: uuid.New()}
	_, _, err := h(context.Background(), nil, GraphInput{NodeRef: nodeRefWire(ref)})
	if !errors.Is(err, store.ErrNodeNotFound) {
		t.Errorf("handler error = %v, want errors.Is(_, store.ErrNodeNotFound)", err)
	}
}

func TestGraphHandlerPropagatesChainError(t *testing.T) {
	ref := graph.NodeRef{CorpusID: string(corpus.VNReg), DocumentID: uuid.New()}
	stub := &stubGraphRepo{view: store.NodeView{Ref: ref}, chainErr: errors.New("boom")}
	h := newGraphHandler(stub, "mise_public")

	_, _, err := h(context.Background(), nil, GraphInput{NodeRef: nodeRefWire(ref)})
	if err == nil {
		t.Fatal("handler error = nil, want the wrapped Chain error")
	}
}

// --- graph tool: mapping ---------------------------------------------------

// TestGraphHandlerEmptyResultsProduceEmptySlicesNotNil mirrors
// TestSearchHandlerEmptyHitsProducesEmptySliceNotNil /
// TestDocumentHandlerEmptySectionsAndAmendmentsProduceEmptySlicesNotNil
// (tools_test.go): a repo returning no edges/hops (nil slices, no error)
// must still marshal to `[]`, never `null`.
func TestGraphHandlerEmptyResultsProduceEmptySlicesNotNil(t *testing.T) {
	ref := graph.NodeRef{CorpusID: string(corpus.VNReg), DocumentID: uuid.New()}
	stub := &stubGraphRepo{view: store.NodeView{Ref: ref}}
	h := newGraphHandler(stub, "mise_public")

	_, out, err := h(context.Background(), nil, GraphInput{NodeRef: nodeRefWire(ref)})
	if err != nil {
		t.Fatalf("handler error = %v, want nil", err)
	}
	if out.Edges == nil {
		t.Error("out.Edges = nil, want a non-nil empty slice (must marshal to [] not null)")
	}
	if out.Chain == nil {
		t.Error("out.Chain = nil, want a non-nil empty slice (must marshal to [] not null)")
	}
	if len(out.Nodes) != 1 {
		t.Errorf("len(out.Nodes) = %d, want 1 (the queried node)", len(out.Nodes))
	}
}

// TestGraphHandlerMapsNodeViewAndChainToOutput is the main mapping test:
// NodeView's edges (with multi-row evidence, so bestGraphEvidence's
// highest-Confidence pick is exercised) and Chain's hops both map onto
// GraphOutput's wire form, ids/dates stringified.
func TestGraphHandlerMapsNodeViewAndChainToOutput(t *testing.T) {
	fromRef := graph.NodeRef{CorpusID: string(corpus.LocalPolicy), DocumentID: uuid.New()}
	edgeID := uuid.New()
	toRefID := uuid.New()
	created, err := time.Parse(time.RFC3339, "2026-01-15T10:00:00Z")
	if err != nil {
		t.Fatalf("parsing fixture time: %v", err)
	}
	edge := graph.Edge{
		ID: edgeID, From: fromRef, ToRefID: toRefID, ToCorpusID: string(corpus.MYReg),
		EdgeType: graph.EdgeSatisfies, Direction: "up", Promoted: true,
		AccessTier: graph.TierGroupConfidential, CreatedAt: created,
	}
	evidence := map[uuid.UUID][]graph.Evidence{
		edgeID: {
			{Confidence: 0.4, GroundingScore: 0.5},
			{Confidence: 0.92, GroundingScore: 0.88}, // higher confidence: must win
		},
	}
	view := store.NodeView{Ref: fromRef, Edges: []graph.Edge{edge}, Evidence: evidence}

	hopRef := graph.NodeRef{CorpusID: string(corpus.MYReg), DocumentID: uuid.New()}
	hops := []store.Hop{{
		Ref: hopRef, EdgeType: "satisfies", CorpusID: string(corpus.MYReg),
		Citation: "Part IV", Promoted: true, Confidence: 0.92, GroundingScore: 0.88,
	}}

	stub := &stubGraphRepo{view: view, hops: hops}
	h := newGraphHandler(stub, "mise_local")

	_, out, err := h(context.Background(), nil, GraphInput{NodeRef: nodeRefWire(fromRef)})
	if err != nil {
		t.Fatalf("handler error = %v, want nil", err)
	}

	if stub.getNodeRole != "mise_local" || stub.chainRole != "mise_local" {
		t.Errorf("role not threaded through: GetNode role=%q Chain role=%q, want mise_local",
			stub.getNodeRole, stub.chainRole)
	}
	if stub.chainStart != fromRef {
		t.Errorf("Chain start = %+v, want the same parsed ref GetNode received: %+v", stub.chainStart, fromRef)
	}

	if len(out.Nodes) != 1 || out.Nodes[0].CorpusID != fromRef.CorpusID ||
		out.Nodes[0].DocumentID != fromRef.DocumentID.String() {
		t.Errorf("out.Nodes = %+v, want [{%s %s}]", out.Nodes, fromRef.CorpusID, fromRef.DocumentID)
	}

	if len(out.Edges) != 1 {
		t.Fatalf("len(out.Edges) = %d, want 1", len(out.Edges))
	}
	gotEdge := out.Edges[0]
	if gotEdge.ID != edgeID.String() || gotEdge.ToRefID != toRefID.String() || gotEdge.ToCorpusID != string(corpus.MYReg) {
		t.Errorf("out.Edges[0] ids = %+v, want edge %s -> %s/%s", gotEdge, edgeID, corpus.MYReg, toRefID)
	}
	if !gotEdge.Promoted {
		t.Error("out.Edges[0].Promoted = false, want true")
	}
	if gotEdge.Confidence != 0.92 || gotEdge.GroundingScore != 0.88 {
		t.Errorf("out.Edges[0] confidence/grounding = %v/%v, want 0.92/0.88 (the higher-confidence evidence row)",
			gotEdge.Confidence, gotEdge.GroundingScore)
	}
	if gotEdge.AccessTier != string(graph.TierGroupConfidential) {
		t.Errorf("out.Edges[0].AccessTier = %q, want %q", gotEdge.AccessTier, graph.TierGroupConfidential)
	}
	if gotEdge.CreatedAt == "" {
		t.Error("out.Edges[0].CreatedAt is empty, want an RFC3339 timestamp")
	}

	if len(out.Chain) != 1 {
		t.Fatalf("len(out.Chain) = %d, want 1", len(out.Chain))
	}
	gotHop := out.Chain[0]
	if gotHop.CorpusID != string(corpus.MYReg) || gotHop.DocumentID != hopRef.DocumentID.String() {
		t.Errorf("out.Chain[0] ref = %+v, want %+v", gotHop, hopRef)
	}
	if gotHop.Citation != "Part IV" || gotHop.EdgeType != "satisfies" {
		t.Errorf("out.Chain[0] = %+v, want Citation=Part IV EdgeType=satisfies", gotHop)
	}
	if gotHop.Confidence != 0.92 || gotHop.GroundingScore != 0.88 {
		t.Errorf("out.Chain[0] confidence/grounding = %v/%v, want 0.92/0.88", gotHop.Confidence, gotHop.GroundingScore)
	}
}

// TestGraphHandlerSectionScopedNodeRefRoundTrips proves the optional
// [/<section_id>] wire segment parses and round-trips through to both the
// repo call and the mapped node/edge output.
func TestGraphHandlerSectionScopedNodeRefRoundTrips(t *testing.T) {
	secID := uuid.New()
	ref := graph.NodeRef{CorpusID: string(corpus.VNReg), DocumentID: uuid.New(), SectionID: &secID}
	stub := &stubGraphRepo{view: store.NodeView{Ref: ref}}
	h := newGraphHandler(stub, "mise_public")

	_, out, err := h(context.Background(), nil, GraphInput{NodeRef: nodeRefWire(ref)})
	if err != nil {
		t.Fatalf("handler error = %v, want nil", err)
	}
	if stub.getNodeRef.SectionID == nil || *stub.getNodeRef.SectionID != secID {
		t.Errorf("GetNode ref.SectionID = %v, want %s", stub.getNodeRef.SectionID, secID)
	}
	if len(out.Nodes) != 1 || out.Nodes[0].SectionID == nil || *out.Nodes[0].SectionID != secID.String() {
		t.Errorf("out.Nodes[0].SectionID = %v, want %s", out.Nodes[0].SectionID, secID)
	}
}

// --- wiring: New()/WithGraph registers exactly the graph tool -------------

func TestNewRegistersGraphToolOnlyWithWithGraph(t *testing.T) {
	ctx := context.Background()

	wired := New(WithGraph(&stubGraphRepo{}, "mise_public"))
	names := connectAndListToolNames(t, ctx, wired)
	want := map[string]bool{"graph": true}
	if len(names) != len(want) {
		t.Fatalf("New(WithGraph(...)) tools = %v, want exactly %v", names, want)
	}
	for _, n := range names {
		if !want[n] {
			t.Errorf("unexpected tool %q registered", n)
		}
	}
}

func TestNewRegistersAllThreeToolsWithEvidenceAndGraph(t *testing.T) {
	ctx := context.Background()
	wired := New(
		WithEvidence(&stubSearcher{}, &stubDocGetter{}, "mise_public"),
		WithGraph(&stubGraphRepo{}, "mise_public"),
	)
	names := connectAndListToolNames(t, ctx, wired)
	want := map[string]bool{"search": true, "document": true, "graph": true}
	if len(names) != len(want) {
		t.Fatalf("New(WithEvidence, WithGraph) tools = %v, want exactly %v", names, want)
	}
	for _, n := range names {
		if !want[n] {
			t.Errorf("unexpected tool %q registered", n)
		}
	}
}

// --- test helpers -----------------------------------------------------------

// nodeRefWire renders ref as the graph tool's wire node_ref
// "<corpus_id>/<document_id>[/<section_id>]" — the inverse of parseNodeRef.
func nodeRefWire(ref graph.NodeRef) string {
	s := ref.CorpusID + "/" + ref.DocumentID.String()
	if ref.SectionID != nil {
		s += "/" + ref.SectionID.String()
	}
	return s
}
