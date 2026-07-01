package embed_test

import (
	"context"
	"testing"

	"danny.vn/mise/pkg/rag/embed"
)

func TestFakeEmbedderDims(t *testing.T) {
	e := embed.NewFake()
	if e.Dims() != 1536 {
		t.Fatalf("dims = %d, want 1536", e.Dims())
	}
	if e.Model() != "gemini-embedding-001" {
		t.Fatalf("model = %q, want gemini-embedding-001", e.Model())
	}
}

func TestFakeEmbedderDeterministic(t *testing.T) {
	e := embed.NewFake()
	ctx := context.Background()
	v1, err := e.Embed(ctx, []string{"hello world"})
	if err != nil {
		t.Fatal(err)
	}
	v2, err := e.Embed(ctx, []string{"hello world"})
	if err != nil {
		t.Fatal(err)
	}
	if len(v1) != 1 || len(v2) != 1 {
		t.Fatalf("expected 1 vector each, got %d and %d", len(v1), len(v2))
	}
	if len(v1[0]) != 1536 {
		t.Fatalf("vector dim = %d, want 1536", len(v1[0]))
	}
	for i := range v1[0] {
		if v1[0][i] != v2[0][i] {
			t.Fatalf("non-deterministic at index %d: %f != %f", i, v1[0][i], v2[0][i])
		}
	}
}

func TestFakeEmbedderBatch(t *testing.T) {
	e := embed.NewFake()
	vecs, err := e.Embed(context.Background(), []string{"a", "b", "c"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 3 {
		t.Fatalf("expected 3 vectors, got %d", len(vecs))
	}
}
