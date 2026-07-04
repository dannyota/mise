package corpus_test

import (
	"errors"
	"testing"

	"danny.vn/mise/pkg/corpus"
)

func TestValidateEmbedSpace_MatchingConfig(t *testing.T) {
	err := corpus.ValidateEmbedSpace(corpus.EmbedConfig{
		Model: "gemini-embedding-001", Dims: 1536, TaskType: "RETRIEVAL_DOCUMENT",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidateEmbedSpace_WrongModel(t *testing.T) {
	err := corpus.ValidateEmbedSpace(corpus.EmbedConfig{
		Model: "gemini-embedding-002", Dims: 1536, TaskType: "RETRIEVAL_DOCUMENT",
	})
	if !errors.Is(err, corpus.ErrEmbedSpaceMismatch) {
		t.Fatalf("expected ErrEmbedSpaceMismatch, got %v", err)
	}
}

func TestValidateEmbedSpace_WrongDims(t *testing.T) {
	err := corpus.ValidateEmbedSpace(corpus.EmbedConfig{
		Model: "gemini-embedding-001", Dims: 768, TaskType: "RETRIEVAL_DOCUMENT",
	})
	if !errors.Is(err, corpus.ErrEmbedSpaceMismatch) {
		t.Fatalf("expected ErrEmbedSpaceMismatch, got %v", err)
	}
}

func TestValidateEmbedSpace_ImageVectorBarred(t *testing.T) {
	err := corpus.ValidateEmbedSpace(corpus.EmbedConfig{
		Model: "gemini-embedding-001", Dims: 1536, TaskType: "RETRIEVAL_IMAGE",
	})
	if !errors.Is(err, corpus.ErrEmbedSpaceMismatch) {
		t.Fatalf("expected ErrEmbedSpaceMismatch for image vector, got %v", err)
	}
}

func TestRegister_ValidDescriptor(t *testing.T) {
	d := corpus.Descriptor{
		ID: "test-corpus", Kind: corpus.KindReport,
		SchemaName: "test_corpus", CitationScheme: "test-cite",
		Embed:      corpus.EmbedConfig{Model: "gemini-embedding-001", Dims: 1536, TaskType: "RETRIEVAL_DOCUMENT"},
		AccessTier: corpus.TierPublic, Jurisdiction: "vn",
		GraphRole:      corpus.GraphRole{CanSource: true},
		MetadataConfig: corpus.MetadataConfig{Defaults: map[string]string{"lang": "en"}},
	}
	if err := corpus.Register(d); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	got, ok := corpus.Get("test-corpus")
	if !ok {
		t.Fatal("registered corpus not found")
	}
	if got.Kind != corpus.KindReport {
		t.Errorf("kind = %s, want report", got.Kind)
	}
	corpus.Unregister("test-corpus")
}

func TestRegister_DuplicateID(t *testing.T) {
	err := corpus.Register(corpus.Descriptor{
		ID: corpus.VNReg, Kind: corpus.KindLaw,
		Embed: corpus.EmbedConfig{Model: "gemini-embedding-001", Dims: 1536, TaskType: "RETRIEVAL_DOCUMENT"},
	})
	if !errors.Is(err, corpus.ErrDuplicateCorpus) {
		t.Fatalf("expected ErrDuplicateCorpus, got %v", err)
	}
}

func TestRegister_EmbedMismatch(t *testing.T) {
	err := corpus.Register(corpus.Descriptor{
		ID: "bad-embed", Kind: corpus.KindReport,
		Embed: corpus.EmbedConfig{Model: "wrong-model", Dims: 1536, TaskType: "RETRIEVAL_DOCUMENT"},
	})
	if !errors.Is(err, corpus.ErrEmbedSpaceMismatch) {
		t.Fatalf("expected ErrEmbedSpaceMismatch, got %v", err)
	}
}
