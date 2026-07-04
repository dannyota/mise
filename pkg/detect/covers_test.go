package detect

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/google/uuid"

	"danny.vn/mise/pkg/graph"
)

// --- fakes ----------------------------------------------------------------

// fakeChainWalker returns a pre-configured chain for each start node,
// keyed by the node's document ID.
type fakeChainWalker struct {
	chains map[uuid.UUID][]Hop
}

func (f *fakeChainWalker) Chain(_ context.Context, _ string, start graph.NodeRef, _ int) ([]Hop, error) {
	hops, ok := f.chains[start.DocumentID]
	if !ok {
		return nil, nil
	}
	return hops, nil
}

// fakeEdgeWriter records every WriteExtractedEdge call and returns a
// deterministic edge ID.
type fakeEdgeWriter struct {
	written []graph.ExtractedEdge
}

func (f *fakeEdgeWriter) WriteExtractedEdge(_ context.Context, e graph.ExtractedEdge) (uuid.UUID, error) {
	f.written = append(f.written, e)
	return uuid.New(), nil
}

// --- helpers --------------------------------------------------------------

func sopRef() graph.NodeRef {
	return graph.NodeRef{CorpusID: "local-sop", DocumentID: uuid.New()}
}

func makeHop(corpusID string, promoted bool) Hop {
	return Hop{
		Ref:      graph.NodeRef{CorpusID: corpusID, DocumentID: uuid.New()},
		EdgeType: "derives",
		CorpusID: corpusID,
		Promoted: promoted,
	}
}

// --- tests ----------------------------------------------------------------

// TestComputeCoversPromotedChainReachesLaw verifies the happy path: a fully
// promoted SOP→Policy→Group→Law chain produces exactly one covers edge.
func TestComputeCoversPromotedChainReachesLaw(t *testing.T) {
	start := sopRef()

	walker := &fakeChainWalker{chains: map[uuid.UUID][]Hop{
		start.DocumentID: {
			makeHop("local-policy", true),
			makeHop("group-std", true),
			makeHop("vn-reg", true),
		},
	}}
	writer := &fakeEdgeWriter{}

	results, err := ComputeCovers(context.Background(), walker, writer, "mise_local",
		[]StartNode{{Ref: start, AttestationOwner: "system"}}, 0)
	if err != nil {
		t.Fatalf("ComputeCovers() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("ComputeCovers() results = %d, want 1", len(results))
	}
	if results[0].From != start {
		t.Errorf("result.From = %v, want %v", results[0].From, start)
	}
	if len(writer.written) != 1 {
		t.Fatalf("writer.written = %d, want 1", len(writer.written))
	}
	e := writer.written[0]
	if e.EdgeType != string(graph.EdgeCovers) {
		t.Errorf("written edge type = %q, want %q", e.EdgeType, graph.EdgeCovers)
	}
	if e.Target.ToCorpusID != "vn-reg" {
		t.Errorf("written edge target corpus = %q, want vn-reg", e.Target.ToCorpusID)
	}
	if e.Direction != "up" {
		t.Errorf("written edge direction = %q, want up", e.Direction)
	}
}

// TestComputeCoversUnpromotedChainNoCoverage verifies that an unpromoted
// intermediate edge prevents the covers edge from being written.
func TestComputeCoversUnpromotedChainNoCoverage(t *testing.T) {
	start := sopRef()

	walker := &fakeChainWalker{chains: map[uuid.UUID][]Hop{
		start.DocumentID: {
			makeHop("local-policy", true),
			makeHop("group-std", false), // not promoted
			makeHop("vn-reg", true),
		},
	}}
	writer := &fakeEdgeWriter{}

	results, err := ComputeCovers(context.Background(), walker, writer, "mise_local",
		[]StartNode{{Ref: start, AttestationOwner: "system"}}, 0)
	if err != nil {
		t.Fatalf("ComputeCovers() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("ComputeCovers() results = %d, want 0 (unpromoted chain)", len(results))
	}
	if len(writer.written) != 0 {
		t.Errorf("writer.written = %d, want 0", len(writer.written))
	}
}

// TestComputeCoversChainNotReachingLawNoCoverage verifies that a fully
// promoted chain that terminates at an internal corpus (not a law corpus)
// produces no covers edge.
func TestComputeCoversChainNotReachingLawNoCoverage(t *testing.T) {
	start := sopRef()

	walker := &fakeChainWalker{chains: map[uuid.UUID][]Hop{
		start.DocumentID: {
			makeHop("local-policy", true),
			makeHop("group-std", true), // stops here, never reaches law
		},
	}}
	writer := &fakeEdgeWriter{}

	results, err := ComputeCovers(context.Background(), walker, writer, "mise_local",
		[]StartNode{{Ref: start, AttestationOwner: "system"}}, 0)
	if err != nil {
		t.Fatalf("ComputeCovers() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("ComputeCovers() results = %d, want 0 (chain doesn't reach law)", len(results))
	}
	if len(writer.written) != 0 {
		t.Errorf("writer.written = %d, want 0", len(writer.written))
	}
}

// TestComputeCoversEmptyChainNoCoverage verifies that a start node whose
// chain walk returns zero hops (no outgoing edges) produces no covers edge.
func TestComputeCoversEmptyChainNoCoverage(t *testing.T) {
	start := sopRef()

	walker := &fakeChainWalker{chains: map[uuid.UUID][]Hop{}}
	writer := &fakeEdgeWriter{}

	results, err := ComputeCovers(context.Background(), walker, writer, "mise_local",
		[]StartNode{{Ref: start, AttestationOwner: "system"}}, 0)
	if err != nil {
		t.Fatalf("ComputeCovers() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("ComputeCovers() results = %d, want 0 (empty chain)", len(results))
	}
}

// TestComputeCoversMultipleStarts verifies that ComputeCovers processes
// multiple start nodes independently: one qualifying, one not.
func TestComputeCoversMultipleStarts(t *testing.T) {
	good := sopRef()
	bad := sopRef()

	walker := &fakeChainWalker{chains: map[uuid.UUID][]Hop{
		good.DocumentID: {
			makeHop("local-policy", true),
			makeHop("my-reg", true),
		},
		bad.DocumentID: {
			makeHop("local-policy", false), // not promoted
			makeHop("vn-reg", true),
		},
	}}
	writer := &fakeEdgeWriter{}

	results, err := ComputeCovers(context.Background(), walker, writer, "mise_local",
		[]StartNode{
			{Ref: good, AttestationOwner: "system"},
			{Ref: bad, AttestationOwner: "system"},
		}, 0)
	if err != nil {
		t.Fatalf("ComputeCovers() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("ComputeCovers() results = %d, want 1 (only the promoted chain)", len(results))
	}
	if results[0].From != good {
		t.Errorf("result.From = %v, want %v (the promoted start)", results[0].From, good)
	}
}

// TestComputeCoversMyRegLaw verifies that my-reg (Malaysian law) is also
// recognized as a law corpus.
func TestComputeCoversMyRegLaw(t *testing.T) {
	start := sopRef()

	walker := &fakeChainWalker{chains: map[uuid.UUID][]Hop{
		start.DocumentID: {
			makeHop("local-policy", true),
			makeHop("my-reg", true),
		},
	}}
	writer := &fakeEdgeWriter{}

	results, err := ComputeCovers(context.Background(), walker, writer, "mise_local",
		[]StartNode{{Ref: start, AttestationOwner: "system"}}, 0)
	if err != nil {
		t.Fatalf("ComputeCovers() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("ComputeCovers() results = %d, want 1", len(results))
	}
	if writer.written[0].Target.ToCorpusID != "my-reg" {
		t.Errorf("target corpus = %q, want my-reg", writer.written[0].Target.ToCorpusID)
	}
}

// --- isLawCorpus ----------------------------------------------------------

func TestIsLawCorpus(t *testing.T) {
	cases := []struct {
		corpus string
		want   bool
	}{
		{"vn-reg", true},
		{"my-reg", true},
		{"group-std", false},
		{"local-policy", false},
		{"local-sop", false},
		{"nonexistent", false},
	}
	for _, tc := range cases {
		t.Run(tc.corpus, func(t *testing.T) {
			if got := isLawCorpus(tc.corpus); got != tc.want {
				t.Errorf("isLawCorpus(%q) = %v, want %v", tc.corpus, got, tc.want)
			}
		})
	}
}

// --- depguard: no vertex import (DEC 7) -----------------------------------

// bannedImportMarkers are substrings that must never appear in an import
// path pulled in by covers.go. The covers edge is computed from the promoted
// chain alone — no model/judge call (DEC 7: no direct SOP→law judge call).
var bannedImportMarkers = []string{
	"vertex",
}

// TestCoversGoImportsNoVertexPackage is a DEC-7 guard: covers.go must never
// import danny.vn/mise/pkg/vertex (the model/judge client). The covers
// relation is computed from the promoted edge chain — a model classification
// on the SOP→law path is forbidden.
func TestCoversGoImportsNoVertexPackage(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "covers.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parser.ParseFile(covers.go) = %v", err)
	}

	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		for _, marker := range bannedImportMarkers {
			if strings.Contains(path, marker) {
				t.Errorf("covers.go imports %q (matches banned marker %q) — "+
					"pkg/detect (covers) must not import the model/judge client (DEC 7)", path, marker)
				break
			}
		}
	}
}

// TestCoversTestGoImportsNoVertexPackage extends the DEC-7 guard to the test
// file itself — even test helpers must not pull in the vertex client.
func TestCoversTestGoImportsNoVertexPackage(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "covers_test.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parser.ParseFile(covers_test.go) = %v", err)
	}

	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		for _, marker := range bannedImportMarkers {
			if strings.Contains(path, marker) {
				t.Errorf("covers_test.go imports %q (matches banned marker %q) — "+
					"pkg/detect tests must not import the model/judge client (DEC 7)", path, marker)
				break
			}
		}
	}
}

// --- computeOne edge-case guard: first hop unpromoted ---------------------

// TestComputeCoversFirstHopUnpromotedNoCoverage verifies that even if the
// terminal hop is law and promoted, an unpromoted FIRST hop still blocks
// the covers edge.
func TestComputeCoversFirstHopUnpromotedNoCoverage(t *testing.T) {
	start := sopRef()

	walker := &fakeChainWalker{chains: map[uuid.UUID][]Hop{
		start.DocumentID: {
			makeHop("local-policy", false), // first hop unpromoted
			makeHop("group-std", true),
			makeHop("vn-reg", true),
		},
	}}
	writer := &fakeEdgeWriter{}

	results, err := ComputeCovers(context.Background(), walker, writer, "mise_local",
		[]StartNode{{Ref: start, AttestationOwner: "system"}}, 0)
	if err != nil {
		t.Fatalf("ComputeCovers() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("ComputeCovers() results = %d, want 0 (first hop unpromoted)", len(results))
	}
}

// --- computeOne written edge shape ----------------------------------------

// TestComputeCoversWrittenEdgeShape verifies the shape of the ExtractedEdge
// written by ComputeCovers: edge_type=covers, direction=up,
// target=terminal law node, from=start SOP node.
func TestComputeCoversWrittenEdgeShape(t *testing.T) {
	start := sopRef()
	lawHop := makeHop("vn-reg", true)

	walker := &fakeChainWalker{chains: map[uuid.UUID][]Hop{
		start.DocumentID: {
			makeHop("local-policy", true),
			lawHop,
		},
	}}
	writer := &fakeEdgeWriter{}

	_, err := ComputeCovers(context.Background(), walker, writer, "mise_local",
		[]StartNode{{Ref: start, AttestationOwner: "ingest-system"}}, 0)
	if err != nil {
		t.Fatalf("ComputeCovers() error = %v", err)
	}
	if len(writer.written) != 1 {
		t.Fatalf("writer.written = %d, want 1", len(writer.written))
	}

	e := writer.written[0]
	if e.From != start {
		t.Errorf("From = %v, want %v", e.From, start)
	}
	if e.EdgeType != string(graph.EdgeCovers) {
		t.Errorf("EdgeType = %q, want %q", e.EdgeType, graph.EdgeCovers)
	}
	if e.Direction != "up" {
		t.Errorf("Direction = %q, want up", e.Direction)
	}
	if e.CreatedBy != "ingest-system" {
		t.Errorf("CreatedBy = %q, want ingest-system", e.CreatedBy)
	}
	if e.Target.ToCorpusID != "vn-reg" {
		t.Errorf("Target.ToCorpusID = %q, want vn-reg", e.Target.ToCorpusID)
	}
	if e.Target.IsStub {
		t.Error("Target.IsStub = true, want false (terminal hop is resolved)")
	}
	if e.Target.Target.DocumentID != lawHop.Ref.DocumentID {
		t.Errorf("Target.Target.DocumentID = %s, want %s",
			e.Target.Target.DocumentID, lawHop.Ref.DocumentID)
	}
}

// --- error propagation ----------------------------------------------------

// fakeErrorChainWalker returns an error for a specific start node.
type fakeErrorChainWalker struct {
	errFor uuid.UUID
}

func (f *fakeErrorChainWalker) Chain(_ context.Context, _ string, start graph.NodeRef, _ int) ([]Hop, error) {
	if start.DocumentID == f.errFor {
		return nil, errors.New("fake chain error")
	}
	return nil, nil
}

// TestComputeCoversChainErrorPropagates verifies that a chain walk error
// propagates out of ComputeCovers with wrapping context.
func TestComputeCoversChainErrorPropagates(t *testing.T) {
	start := sopRef()
	walker := &fakeErrorChainWalker{errFor: start.DocumentID}
	writer := &fakeEdgeWriter{}

	_, err := ComputeCovers(context.Background(), walker, writer, "mise_local",
		[]StartNode{{Ref: start, AttestationOwner: "system"}}, 0)
	if err == nil {
		t.Fatal("ComputeCovers() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "fake chain error") {
		t.Errorf("error = %q, want it to contain the chain error", err.Error())
	}
}
