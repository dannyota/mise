package corpus_test

import (
	"testing"

	"danny.vn/mise/pkg/corpus"
)

func TestGAGuard_DescriptorOnlyRegistration(t *testing.T) {
	fixture := corpus.Descriptor{
		ID: "ga-fixture", Kind: corpus.KindReport,
		SchemaName: "ga_fixture", CitationScheme: "ga-cite",
		Embed:      corpus.EmbedConfig{Model: "gemini-embedding-001", Dims: 1536, TaskType: "RETRIEVAL_DOCUMENT"},
		AccessTier: corpus.TierPublic, Jurisdiction: "vn",
		GraphRole: corpus.GraphRole{
			CanSource:       true,
			CanTarget:       true,
			DefaultEdges:    []string{"implements"},
			SatisfiesTarget: corpus.VNReg,
		},
		MetadataConfig: corpus.MetadataConfig{
			Defaults: map[string]string{"lang": "en"},
		},
	}
	if err := corpus.Register(fixture); err != nil {
		t.Fatalf("Register fixture corpus: %v", err)
	}
	defer corpus.Unregister("ga-fixture")

	got, ok := corpus.Get("ga-fixture")
	if !ok {
		t.Fatal("fixture corpus not found after registration")
	}
	if got.Kind != corpus.KindReport {
		t.Errorf("kind = %s, want report", got.Kind)
	}
	if !got.GraphRole.CanSource {
		t.Error("fixture should be able to source edges")
	}
	if got.GraphRole.SatisfiesTarget != corpus.VNReg {
		t.Errorf("satisfies target = %s, want vn-reg", got.GraphRole.SatisfiesTarget)
	}
}
