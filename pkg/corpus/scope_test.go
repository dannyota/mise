package corpus_test

import (
	"testing"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/graph"
)

func TestNewScope_DescriptorOnly(t *testing.T) {
	myPolicy := corpus.Descriptor{
		ID: "local-policy-my", Kind: corpus.KindPolicy,
		SchemaName: "local_policy_my", CitationScheme: "policy-section",
		Embed:        corpus.EmbedConfig{Model: "gemini-embedding-001", Dims: 1536, TaskType: "RETRIEVAL_DOCUMENT"},
		AccessTier:   corpus.TierLocalConfidential,
		Tier:         corpus.TierLocal,
		Jurisdiction: "my",
		GraphRole: corpus.GraphRole{
			CanSource: true, CanTarget: true,
			DefaultEdges:    []string{"satisfies", "implements"},
			SatisfiesTarget: corpus.MYReg,
		},
	}
	if err := corpus.Register(myPolicy); err != nil {
		t.Fatalf("Register local-policy-my: %v", err)
	}
	defer corpus.Unregister("local-policy-my")

	got, ok := corpus.Get("local-policy-my")
	if !ok {
		t.Fatal("local-policy-my not found")
	}
	if got.Jurisdiction != "my" {
		t.Errorf("jurisdiction = %q, want my", got.Jurisdiction)
	}

	edgeType, ok := graph.EdgeTypeForPair("local-policy-my", corpus.MYReg)
	if !ok {
		t.Fatal("EdgeTypeForPair should find satisfies for local-policy-my -> my-reg")
	}
	if edgeType != "satisfies" {
		t.Errorf("edge type = %q, want satisfies", edgeType)
	}
}
