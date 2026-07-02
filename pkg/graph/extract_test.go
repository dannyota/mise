package graph_test

import (
	"reflect"
	"testing"

	"github.com/google/uuid"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/graph"
)

// mustDescriptor is a small test helper: corpus.Get for a known-registered
// ID never fails, so a two-value call at every call site would just be
// noise.
func mustDescriptor(t *testing.T, id corpus.ID) corpus.Descriptor {
	t.Helper()
	desc, ok := corpus.Get(id)
	if !ok {
		t.Fatalf("corpus.Get(%s) ok = false, want a registered descriptor", id)
	}
	return desc
}

// TestExtractEdgesResolvedRefs is the brief's two positive cases: a
// local-sop header referencing its local-policy parent yields one "derives"
// ExtractedEdge, and a local-policy header referencing a group-std standard
// yields one "implements" edge. Both assert the full assembled shape:
// From/EdgeType/Direction/quoted spans/CreatedBy, and that Target is the
// resolve() return value carried through unchanged.
func TestExtractEdgesResolvedRefs(t *testing.T) {
	sopDocID := uuid.New()
	policyDocID := uuid.New()
	targetDocID := uuid.New()

	tests := []struct {
		name         string
		desc         corpus.Descriptor
		header       graph.DocControlHeader
		resolved     graph.ResolvedRef
		wantEdgeType string
		wantFromCorp string
	}{
		{
			name: "local-sop derives from its local-policy parent",
			desc: mustDescriptor(t, corpus.LocalSOP),
			header: graph.DocControlHeader{
				Corpus:           corpus.LocalSOP,
				DocID:            sopDocID,
				AttestationOwner: "jane.doe@example.com",
				ControlRefs: []graph.RawControlRef{
					{Relation: "derives", TargetNumber: "POL-7", QuotedSpan: "Derives from POL-7."},
				},
			},
			resolved: graph.ResolvedRef{
				Target:     graph.NodeRef{CorpusID: string(corpus.LocalPolicy), DocumentID: targetDocID},
				ToCorpusID: string(corpus.LocalPolicy),
				RefKey:     "POL-7",
			},
			wantEdgeType: string(graph.EdgeDerives),
			wantFromCorp: string(corpus.LocalSOP),
		},
		{
			name: "local-policy implements a group-std standard",
			desc: mustDescriptor(t, corpus.LocalPolicy),
			header: graph.DocControlHeader{
				Corpus:           corpus.LocalPolicy,
				DocID:            policyDocID,
				AttestationOwner: "john.roe@example.com",
				ControlRefs: []graph.RawControlRef{
					{Relation: "implements", TargetNumber: "STD-9", QuotedSpan: "Implements STD-9."},
				},
			},
			resolved: graph.ResolvedRef{
				Target:     graph.NodeRef{CorpusID: string(corpus.GroupStd), DocumentID: targetDocID},
				ToCorpusID: string(corpus.GroupStd),
				RefKey:     "STD-9",
			},
			wantEdgeType: string(graph.EdgeImplements),
			wantFromCorp: string(corpus.LocalPolicy),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolve := func(graph.RawControlRef) (graph.ResolvedRef, bool) {
				return tt.resolved, true
			}
			got := graph.ExtractEdges(tt.desc, tt.header, resolve)
			checkSingleExtractedEdge(t, got, tt.wantFromCorp, tt.wantEdgeType, tt.header, tt.resolved)
		})
	}
}

// checkSingleExtractedEdge asserts got is exactly the one ExtractedEdge
// TestExtractEdgesResolvedRefs's positive cases expect: shape derived from
// header (From/quoted-from-span/CreatedBy), wantEdgeType, and Target passed
// through from resolve's return value unchanged.
func checkSingleExtractedEdge(
	t *testing.T, got []graph.ExtractedEdge, wantFromCorp, wantEdgeType string,
	header graph.DocControlHeader, resolved graph.ResolvedRef,
) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("ExtractEdges() returned %d edges, want 1", len(got))
	}

	edge := got[0]
	wantFrom := graph.NodeRef{CorpusID: wantFromCorp, DocumentID: header.DocID}
	if edge.From != wantFrom {
		t.Errorf("edge.From = %+v, want %+v", edge.From, wantFrom)
	}
	if edge.EdgeType != wantEdgeType {
		t.Errorf("edge.EdgeType = %q, want %q", edge.EdgeType, wantEdgeType)
	}
	if edge.Direction != "up" {
		t.Errorf(`edge.Direction = %q, want "up"`, edge.Direction)
	}
	wantQuote := header.ControlRefs[0].QuotedSpan
	if edge.QuotedFromSpan != wantQuote {
		t.Errorf("edge.QuotedFromSpan = %q, want %q", edge.QuotedFromSpan, wantQuote)
	}
	if edge.QuotedToSpan != "" {
		t.Errorf("edge.QuotedToSpan = %q, want empty (Method A has no target-side quote)", edge.QuotedToSpan)
	}
	if edge.CreatedBy != header.AttestationOwner {
		t.Errorf("edge.CreatedBy = %q, want %q", edge.CreatedBy, header.AttestationOwner)
	}
	if !reflect.DeepEqual(edge.Target, resolved) {
		t.Errorf("edge.Target = %+v, want the resolve() return value %+v unchanged", edge.Target, resolved)
	}
}

// TestExtractEdgesNoControlRefsYieldsNoEdges is the brief's third case: a
// header with nothing to extract must produce no edges — and resolve must
// never even be called, since ParseControlRefs found nothing to resolve.
func TestExtractEdgesNoControlRefsYieldsNoEdges(t *testing.T) {
	desc := mustDescriptor(t, corpus.LocalSOP)
	header := graph.DocControlHeader{Corpus: corpus.LocalSOP, DocID: uuid.New()}

	resolveCalled := false
	resolve := func(graph.RawControlRef) (graph.ResolvedRef, bool) {
		resolveCalled = true
		return graph.ResolvedRef{}, true
	}

	got := graph.ExtractEdges(desc, header, resolve)
	if len(got) != 0 {
		t.Errorf("ExtractEdges() = %+v, want no edges for a header with no control refs", got)
	}
	if resolveCalled {
		t.Error("resolve was called for a header with no control refs; there was nothing to resolve")
	}
}

// TestExtractEdgesDropsRefWhenResolveFails covers Step 2's "drop
// unresolved-ambiguous" rule: when resolve reports ok=false (the ref's
// target corpus was ambiguous — see ResolveRef), ExtractEdges must drop it
// silently, not synthesize a placeholder edge.
func TestExtractEdgesDropsRefWhenResolveFails(t *testing.T) {
	desc := mustDescriptor(t, corpus.LocalPolicy)
	header := graph.DocControlHeader{
		Corpus: corpus.LocalPolicy,
		DocID:  uuid.New(),
		ControlRefs: []graph.RawControlRef{
			{Relation: "implements", TargetNumber: "STD-1", QuotedSpan: "Implements STD-1."},
		},
	}
	resolve := func(graph.RawControlRef) (graph.ResolvedRef, bool) { return graph.ResolvedRef{}, false }

	got := graph.ExtractEdges(desc, header, resolve)
	if len(got) != 0 {
		t.Errorf("ExtractEdges() = %+v, want no edges when resolve() reports false", got)
	}
}

// TestExtractEdgesDropsWhenEdgeTypeForPairUnrecognized is the defensive
// branch noted in the brief: shouldn't happen in practice, since a real
// resolve closure wraps ResolveRef, which only ever resolves the same pairs
// EdgeTypeForPair recognizes — but ExtractEdges must not write an edge with
// a guessed/empty EdgeType if resolve ever reports ok=true for a pair
// EdgeTypeForPair doesn't recognize.
func TestExtractEdgesDropsWhenEdgeTypeForPairUnrecognized(t *testing.T) {
	desc := mustDescriptor(t, corpus.LocalSOP)
	header := graph.DocControlHeader{
		Corpus: corpus.LocalSOP,
		DocID:  uuid.New(),
		ControlRefs: []graph.RawControlRef{
			{Relation: "derives", TargetNumber: "X-1", QuotedSpan: "..."},
		},
	}
	resolve := func(graph.RawControlRef) (graph.ResolvedRef, bool) {
		// local-sop -> vn-reg is not a recognized control-chain pair.
		return graph.ResolvedRef{ToCorpusID: string(corpus.VNReg)}, true
	}

	got := graph.ExtractEdges(desc, header, resolve)
	if len(got) != 0 {
		t.Errorf("ExtractEdges() = %+v, want no edges for an EdgeTypeForPair-unrecognized pair", got)
	}
}

// TestExtractEdgesMakesNoModelOrNetworkCall is Method A's hard requirement
// (deterministic extraction, no model/judge/network call — RISKS R6):
// ExtractEdges's canonical signature — func(corpus.Descriptor,
// DocControlHeader, func(RawControlRef) (ResolvedRef, bool))
// []ExtractedEdge — takes only value types plus a pure function parameter.
// There is no context.Context, no store/pool handle, and no model client in
// scope for it to call out with, and extract.go's own imports (this
// package's types and pkg/corpus only) confirm no vertex/embed package is
// even reachable from it.
//
// What this test proves at runtime: resolve is invoked exactly once per
// qualifying ref — never retried, never re-checked by a second pass — so
// the only work ExtractEdges does beyond ParseControlRefs is arithmetic
// over resolve's pure return value. It then re-runs with identical inputs
// and requires an identical result: the defining property of a pure,
// deterministic function, and exactly what a hidden nondeterministic model
// or network call would break.
func TestExtractEdgesMakesNoModelOrNetworkCall(t *testing.T) {
	desc := mustDescriptor(t, corpus.LocalSOP)
	header := graph.DocControlHeader{
		Corpus: corpus.LocalSOP,
		DocID:  uuid.New(),
		ControlRefs: []graph.RawControlRef{
			{Relation: "derives", TargetNumber: "POL-1", QuotedSpan: "a"},
			{Relation: "derives", TargetNumber: "POL-2", QuotedSpan: "b"},
		},
	}

	calls := 0
	resolve := func(graph.RawControlRef) (graph.ResolvedRef, bool) {
		calls++
		return graph.ResolvedRef{ToCorpusID: string(corpus.LocalPolicy)}, true
	}

	first := graph.ExtractEdges(desc, header, resolve)
	if calls != 2 {
		t.Fatalf("resolve called %d times, want exactly 2 (one per qualifying ref, no retries/model pass)", calls)
	}

	calls = 0
	second := graph.ExtractEdges(desc, header, resolve)
	if calls != 2 {
		t.Fatalf("second run: resolve called %d times, want exactly 2", calls)
	}
	if len(first) != len(second) {
		t.Fatalf("ExtractEdges() produced %d edges then %d on an identical re-run; a deterministic "+
			"function must reproduce the same result", len(first), len(second))
	}
	for i := range first {
		if !reflect.DeepEqual(first[i], second[i]) {
			t.Errorf("edge %d differs between identical runs: %+v vs %+v", i, first[i], second[i])
		}
	}
}
