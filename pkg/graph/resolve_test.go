package graph_test

import (
	"testing"

	"github.com/google/uuid"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/graph"
)

// --- EdgeTypeForPair -------------------------------------------------------

// allControlChainCorpora is every registered corpus.ID, including ones that
// never source a control-chain edge (vn-reg, my-reg). Ranging it against
// itself below is what makes TestEdgeTypeForPair an exhaustive "all pairs
// incl. unmapped" table, not just a check of the four positive cases.
var allControlChainCorpora = []corpus.ID{
	corpus.VNReg, corpus.MYReg, corpus.GroupStd, corpus.LocalPolicy, corpus.LocalSOP,
}

// edgeTypeForPairWant is the brief's fixed control-chain pairs — the only
// four (from, to) combinations EdgeTypeForPair should recognize. Every
// other pair in the 5x5 cross product below must come back ("", false).
var edgeTypeForPairWant = map[[2]corpus.ID]string{
	{corpus.LocalSOP, corpus.LocalPolicy}: "derives",
	{corpus.LocalPolicy, corpus.GroupStd}: "implements",
	{corpus.GroupStd, corpus.MYReg}:       "satisfies",
	{corpus.LocalPolicy, corpus.VNReg}:    "satisfies",
}

// TestEdgeTypeForPair exhaustively checks every (from, to) pair across all
// five registered corpora: the four fixed control-chain pairs return their
// edge type, and the remaining twenty-one — including every from==to
// self-pair — return ("", false) rather than a guess.
func TestEdgeTypeForPair(t *testing.T) {
	for _, from := range allControlChainCorpora {
		for _, to := range allControlChainCorpora {
			wantType, wantOK := edgeTypeForPairWant[[2]corpus.ID{from, to}]
			t.Run(string(from)+"->"+string(to), func(t *testing.T) {
				gotType, gotOK := graph.EdgeTypeForPair(from, to)
				if gotOK != wantOK || gotType != wantType {
					t.Errorf("EdgeTypeForPair(%s, %s) = (%q, %v), want (%q, %v)",
						from, to, gotType, gotOK, wantType, wantOK)
				}
			})
		}
	}
}

// --- ResolveRef --------------------------------------------------------

// notFoundLookup always reports the target as not yet ingested, forcing
// ResolveRef down the stub path.
func notFoundLookup(corpus.ID, string) (uuid.UUID, bool) { return uuid.Nil, false }

// TestResolveRefResolvesToExistingCorpusNode is the "resolved corpus node"
// case: lookup finds the target document, so ResolveRef reports a real
// NodeRef, not a stub.
func TestResolveRefResolvesToExistingCorpusNode(t *testing.T) {
	tests := []struct {
		name       string
		from       corpus.ID
		ref        graph.RawControlRef
		wantTarget corpus.ID
	}{
		{
			name:       "implements from local-policy resolves against group-std",
			from:       corpus.LocalPolicy,
			ref:        graph.RawControlRef{Relation: "implements", TargetNumber: "STD-9", QuotedSpan: "implements STD-9"},
			wantTarget: corpus.GroupStd,
		},
		{
			name:       "derives from local-sop resolves against local-policy",
			from:       corpus.LocalSOP,
			ref:        graph.RawControlRef{Relation: "derives", TargetNumber: "POL-7", QuotedSpan: "derives from POL-7"},
			wantTarget: corpus.LocalPolicy,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docID := uuid.New()
			var gotCorpus corpus.ID
			var gotNumber string
			lookup := func(target corpus.ID, number string) (uuid.UUID, bool) {
				gotCorpus, gotNumber = target, number
				return docID, true
			}

			got, ok := graph.ResolveRef(tt.from, tt.ref, lookup)
			if !ok {
				t.Fatal("ResolveRef() ok = false, want true")
			}
			if gotCorpus != tt.wantTarget || gotNumber != tt.ref.TargetNumber {
				t.Errorf("lookup called with (%s, %q), want (%s, %q)",
					gotCorpus, gotNumber, tt.wantTarget, tt.ref.TargetNumber)
			}
			want := graph.ResolvedRef{
				Target:     graph.NodeRef{CorpusID: string(tt.wantTarget), DocumentID: docID},
				ToCorpusID: string(tt.wantTarget),
				IsStub:     false,
			}
			if got != want {
				t.Errorf("ResolveRef() = %+v, want %+v", got, want)
			}
		})
	}
}

// TestResolveRefStubWhenLookupMisses is the "stub path" case: the target
// corpus is still derivable, but lookup can't find the document yet (either
// it isn't ingested, or the ref only named a title, no number) — ResolveRef
// must still report ok=true with IsStub=true, never guessing a document id.
func TestResolveRefStubWhenLookupMisses(t *testing.T) {
	tests := []struct {
		name       string
		from       corpus.ID
		ref        graph.RawControlRef
		wantTarget corpus.ID
	}{
		{
			name:       "implements from local-policy with no matching group-std doc",
			from:       corpus.LocalPolicy,
			ref:        graph.RawControlRef{Relation: "implements", TargetNumber: "STD-404", QuotedSpan: "implements STD-404"},
			wantTarget: corpus.GroupStd,
		},
		{
			name: "derives from local-sop with a title-only ref (no target number to match)",
			from: corpus.LocalSOP,
			ref: graph.RawControlRef{
				Relation: "derives", TargetTitle: "Missing Policy", QuotedSpan: "derives from Missing Policy",
			},
			wantTarget: corpus.LocalPolicy,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := graph.ResolveRef(tt.from, tt.ref, notFoundLookup)
			if !ok {
				t.Fatal("ResolveRef() ok = false, want true (a stub is still a resolution)")
			}
			want := graph.ResolvedRef{ToCorpusID: string(tt.wantTarget), IsStub: true}
			if got != want {
				t.Errorf("ResolveRef() = %+v, want stub %+v", got, want)
			}
		})
	}
}

// TestResolveRefAmbiguousIsDropped covers every way a ref can fail to yield
// a derivable target corpus: ResolveRef must return ok=false, a zero-value
// ResolvedRef, and — critically — never call lookup, since there is no
// target corpus to look anything up against.
func TestResolveRefAmbiguousIsDropped(t *testing.T) {
	tests := []struct {
		name string
		from corpus.ID
		ref  graph.RawControlRef
	}{
		{
			name: "derives from local-policy has no target in the control chain",
			from: corpus.LocalPolicy,
			ref:  graph.RawControlRef{Relation: "derives", TargetNumber: "X-1", QuotedSpan: "derives from X-1"},
		},
		{
			name: "implements from local-sop has no target in the control chain",
			from: corpus.LocalSOP,
			ref:  graph.RawControlRef{Relation: "implements", TargetNumber: "X-2", QuotedSpan: "implements X-2"},
		},
		{
			name: "unrecognized relation verb",
			from: corpus.LocalPolicy,
			ref:  graph.RawControlRef{Relation: "satisfies", TargetNumber: "X-3", QuotedSpan: "satisfies X-3"},
		},
		{
			name: "source corpus outside the control chain",
			from: corpus.VNReg,
			ref:  graph.RawControlRef{Relation: "implements", TargetNumber: "X-4", QuotedSpan: "implements X-4"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookupCalled := false
			lookup := func(corpus.ID, string) (uuid.UUID, bool) {
				lookupCalled = true
				return uuid.Nil, false
			}

			got, ok := graph.ResolveRef(tt.from, tt.ref, lookup)
			if ok {
				t.Error("ResolveRef() ok = true, want false (ambiguous ref must be dropped)")
			}
			if got != (graph.ResolvedRef{}) {
				t.Errorf("ResolveRef() = %+v, want the zero value when ok = false", got)
			}
			if lookupCalled {
				t.Error("lookup was called for an ambiguous ref; must not guess a target corpus to look up")
			}
		})
	}
}
