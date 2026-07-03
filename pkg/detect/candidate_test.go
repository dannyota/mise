package detect_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/detect"
	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/rag/embed"
	"danny.vn/mise/pkg/vertex"
)

// ---------------------------------------------------------------------------
// Unit tests — fakes only, no DB
// ---------------------------------------------------------------------------

// TestCorpusRoutingGroupStdToMYReg verifies that group-std's SatisfiesTarget
// points to my-reg, which is the routing FindCandidates relies on.
func TestCorpusRoutingGroupStdToMYReg(t *testing.T) {
	desc, ok := corpus.Get(corpus.GroupStd)
	if !ok {
		t.Fatal("corpus.Get(GroupStd) not found")
	}
	if desc.GraphRole.SatisfiesTarget != corpus.MYReg {
		t.Fatalf("GroupStd.SatisfiesTarget = %q, want %q",
			desc.GraphRole.SatisfiesTarget, corpus.MYReg)
	}
}

// TestCorpusRoutingLocalPolicyToVNReg verifies that local-policy targets vn-reg.
func TestCorpusRoutingLocalPolicyToVNReg(t *testing.T) {
	desc, ok := corpus.Get(corpus.LocalPolicy)
	if !ok {
		t.Fatal("corpus.Get(LocalPolicy) not found")
	}
	if desc.GraphRole.SatisfiesTarget != corpus.VNReg {
		t.Fatalf("LocalPolicy.SatisfiesTarget = %q, want %q",
			desc.GraphRole.SatisfiesTarget, corpus.VNReg)
	}
}

// TestFindCandidatesRejectsUnregisteredCorpus verifies that an unknown corpus
// ID is rejected before any DB or embedding call.
func TestFindCandidatesRejectsUnregisteredCorpus(t *testing.T) {
	emb := embed.NewFake()
	fe := emb.(embed.FactEmbedder)
	ranker := vertex.NewFakeRanker()

	_, err := detect.FindCandidates(
		t.Context(), nil, fe, ranker,
		graph.NodeRef{}, "text", "nonexistent-corpus", 5,
	)
	if err == nil {
		t.Fatal("FindCandidates(bad corpus) should error")
	}
	if !strings.Contains(err.Error(), "not a registered corpus") {
		t.Errorf("error = %q, want mention of unregistered corpus", err)
	}
}

// TestFindCandidatesRejectsZeroTopK verifies topK validation fires before
// anything else.
func TestFindCandidatesRejectsZeroTopK(t *testing.T) {
	emb := embed.NewFake()
	fe := emb.(embed.FactEmbedder)
	ranker := vertex.NewFakeRanker()

	_, err := detect.FindCandidates(
		t.Context(), nil, fe, ranker,
		graph.NodeRef{}, "text", corpus.MYReg, 0,
	)
	if err == nil {
		t.Fatal("FindCandidates(topK=0) should error")
	}
	if !strings.Contains(err.Error(), "topK must be positive") {
		t.Errorf("error = %q, want topK validation", err)
	}
}

// TestFindCandidatesRejectsNegativeTopK is a boundary check.
func TestFindCandidatesRejectsNegativeTopK(t *testing.T) {
	emb := embed.NewFake()
	fe := emb.(embed.FactEmbedder)
	ranker := vertex.NewFakeRanker()

	_, err := detect.FindCandidates(
		t.Context(), nil, fe, ranker,
		graph.NodeRef{}, "text", corpus.MYReg, -1,
	)
	if err == nil {
		t.Fatal("FindCandidates(topK=-1) should error")
	}
}

// TestFakeEmbedderImplementsFactEmbedder confirms the fake satisfies the
// FactEmbedder interface that FindCandidates requires.
func TestFakeEmbedderImplementsFactEmbedder(t *testing.T) {
	emb := embed.NewFake()
	fe, ok := emb.(embed.FactEmbedder)
	if !ok {
		t.Fatal("fake embedder does not implement FactEmbedder")
	}
	vecs, err := fe.EmbedFact(t.Context(), []string{"test"})
	if err != nil {
		t.Fatalf("EmbedFact() error = %v", err)
	}
	if len(vecs) != 1 {
		t.Fatalf("EmbedFact() returned %d vectors, want 1", len(vecs))
	}
	if len(vecs[0]) != 1536 {
		t.Errorf("vector dims = %d, want 1536", len(vecs[0]))
	}
}

// TestCandidatePairShape verifies the struct can be constructed and its fields
// carry through correctly.
func TestCandidatePairShape(t *testing.T) {
	fromID := uuid.New()
	toID := uuid.New()
	toSec := uuid.New()
	cp := detect.CandidatePair{
		FromRef:      graph.NodeRef{CorpusID: "group-std", DocumentID: fromID},
		FromText:     "control text",
		ToRef:        graph.NodeRef{CorpusID: "my-reg", DocumentID: toID, SectionID: &toSec},
		ToText:       "law text",
		TargetCorpus: corpus.MYReg,
		Score:        0.95,
	}
	if cp.FromRef.CorpusID != "group-std" {
		t.Errorf("FromRef.CorpusID = %q, want group-std", cp.FromRef.CorpusID)
	}
	if cp.ToRef.CorpusID != "my-reg" {
		t.Errorf("ToRef.CorpusID = %q, want my-reg", cp.ToRef.CorpusID)
	}
	if cp.Score != 0.95 {
		t.Errorf("Score = %f, want 0.95", cp.Score)
	}
}

// TestLawCorporaHaveSchemaNames confirms every law corpus has a non-empty
// schema name — buildANNSQL needs this to render valid SQL.
func TestLawCorporaHaveSchemaNames(t *testing.T) {
	for _, id := range []corpus.ID{corpus.VNReg, corpus.MYReg} {
		desc, ok := corpus.Get(id)
		if !ok {
			t.Fatalf("corpus.Get(%s) not found", id)
		}
		if desc.SchemaName == "" {
			t.Errorf("corpus %s has empty SchemaName", id)
		}
	}
}

// TestBuildANNSQLContent verifies the ANN SQL output contains expected
// clauses via the exported helper.
func TestBuildANNSQLContent(t *testing.T) {
	sql := detect.BuildANNSQL("my_reg")
	checks := []string{
		`"my_reg"."section"`,
		"embedding IS NOT NULL",
		"validity_status IN ('in_force','amended')",
		"<=>",
		"LIMIT $2",
	}
	for _, want := range checks {
		if !strings.Contains(sql, want) {
			t.Errorf("BuildANNSQL missing %q in:\n%s", want, sql)
		}
	}
}
