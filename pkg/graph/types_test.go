package graph_test

import (
	"testing"

	"github.com/google/uuid"

	"danny.vn/mise/pkg/graph"
)

// TestTierConstantsMatchDBCorpusTierOutput pins the Tier const values against
// migrations/009_graph_tables.sql's graph.corpus_tier CASE output strings —
// a drift here would silently desync the Go side from the DB-derived value
// it reads back into an Edge.AccessTier.
func TestTierConstantsMatchDBCorpusTierOutput(t *testing.T) {
	tests := []struct {
		tier graph.Tier
		want string
	}{
		{graph.TierPublic, "public"},
		{graph.TierGroupConfidential, "group-confidential"},
		{graph.TierLocalConfidential, "local-confidential"},
	}
	for _, tt := range tests {
		if string(tt.tier) != tt.want {
			t.Errorf("Tier %v = %q, want %q", tt.tier, string(tt.tier), tt.want)
		}
	}
}

// TestTierRankMatchesDBTierRank pins TierRank's return values against
// migrations/009_graph_tables.sql's graph.tier_rank CASE output exactly.
func TestTierRankMatchesDBTierRank(t *testing.T) {
	tests := []struct {
		tier graph.Tier
		rank int
	}{
		{graph.TierPublic, 0},
		{graph.TierGroupConfidential, 1},
		{graph.TierLocalConfidential, 2},
	}
	for _, tt := range tests {
		if got := graph.TierRank(tt.tier); got != tt.rank {
			t.Errorf("TierRank(%s) = %d, want %d", tt.tier, got, tt.rank)
		}
	}
}

// TestTierRankIsStrictlyIncreasing is the ordering property
// graph.stricter_tier relies on: public < group-confidential <
// local-confidential, so "higher rank wins" always picks the stricter tier.
func TestTierRankIsStrictlyIncreasing(t *testing.T) {
	if graph.TierRank(graph.TierPublic) >= graph.TierRank(graph.TierGroupConfidential) {
		t.Error("TierRank(public) must be < TierRank(group-confidential)")
	}
	if graph.TierRank(graph.TierGroupConfidential) >= graph.TierRank(graph.TierLocalConfidential) {
		t.Error("TierRank(group-confidential) must be < TierRank(local-confidential)")
	}
}

// TestTierRankUnknownFailsClosedToStrictest mirrors graph.tier_rank's ELSE
// branch: an unrecognized tier ranks as strict as TierLocalConfidential, the
// same fail-closed default graph.corpus_tier uses for an unmapped corpus_id.
func TestTierRankUnknownFailsClosedToStrictest(t *testing.T) {
	if got, want := graph.TierRank(graph.Tier("unknown")), graph.TierRank(graph.TierLocalConfidential); got != want {
		t.Errorf("TierRank(unknown) = %d, want %d (fail-closed to strictest)", got, want)
	}
}

// TestEdgeTypeConstantsMatchCheckConstraint pins the EdgeType const values
// against relation_edge.edge_type's CHECK constraint list
// (migrations/009_graph_tables.sql).
func TestEdgeTypeConstantsMatchCheckConstraint(t *testing.T) {
	tests := []struct {
		edgeType graph.EdgeType
		want     string
	}{
		{graph.EdgeSatisfies, "satisfies"},
		{graph.EdgeImplements, "implements"},
		{graph.EdgeDerives, "derives"},
		{graph.EdgeCovers, "covers"},
	}
	for _, tt := range tests {
		if string(tt.edgeType) != tt.want {
			t.Errorf("EdgeType %v = %q, want %q", tt.edgeType, string(tt.edgeType), tt.want)
		}
	}
}

// TestEvidenceKindConstantsMatchCheckConstraint pins the EvidenceKind const
// values against relation_evidence.evidence_kind's CHECK constraint list
// (migrations/009_graph_tables.sql).
func TestEvidenceKindConstantsMatchCheckConstraint(t *testing.T) {
	tests := []struct {
		kind graph.EvidenceKind
		want string
	}{
		{graph.EvidenceExtracted, "extracted"},
		{graph.EvidenceModelClassification, "model_classification"},
		{graph.EvidenceHumanAttested, "human_attested"},
	}
	for _, tt := range tests {
		if string(tt.kind) != tt.want {
			t.Errorf("EvidenceKind %v = %q, want %q", tt.kind, string(tt.kind), tt.want)
		}
	}
}

// TestDocRefResolveUnresolved is the doc_ref_unresolved_idx case: a DocRef
// with no DocumentID yet must report ok=false, not a zero-value NodeRef
// mistaken for a real one.
func TestDocRefResolveUnresolved(t *testing.T) {
	unresolved := graph.DocRef{CorpusID: "vn-reg", RefKey: "vn-reg:Điều 12"}
	if ref, ok := unresolved.Resolve(); ok {
		t.Errorf("Resolve() on an unresolved DocRef = (%+v, true), want ok = false", ref)
	}
}

// TestDocRefResolveResolved is TestDocRefResolveUnresolved's counterpart:
// once DocumentID (and optionally SectionID) is set, Resolve must surface
// exactly that location as a NodeRef.
func TestDocRefResolveResolved(t *testing.T) {
	resolved := graph.DocRef{
		CorpusID: "vn-reg", RefKey: "vn-reg:Điều 12",
		DocumentID: new(uuid.New()), SectionID: new(uuid.New()),
	}

	ref, ok := resolved.Resolve()
	if !ok {
		t.Fatal("Resolve() on a resolved DocRef ok = false, want true")
	}
	if ref.CorpusID != resolved.CorpusID || ref.DocumentID != *resolved.DocumentID {
		t.Errorf("Resolve() = %+v, want CorpusID=%q DocumentID=%v", ref, resolved.CorpusID, *resolved.DocumentID)
	}
	if ref.SectionID == nil || *ref.SectionID != *resolved.SectionID {
		t.Errorf("Resolve().SectionID = %v, want %v", ref.SectionID, *resolved.SectionID)
	}
}
