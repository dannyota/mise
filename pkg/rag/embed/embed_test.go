package embed_test

import (
	"context"
	"math"
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

// TestFakeEmbedderTokenOverlap asserts the fake embedder carries real token-
// overlap signal: a text sharing tokens with a query must cosine-score
// higher than a text sharing none, so offline retrieval tests (recall@k,
// ranking) are meaningful without a live Vertex call.
func TestFakeEmbedderTokenOverlap(t *testing.T) {
	e := embed.NewFake()
	vecs, err := e.Embed(context.Background(), []string{
		"vốn điều lệ ngân hàng",
		"vốn điều lệ",
		"kiểm toán nội bộ",
	})
	if err != nil {
		t.Fatal(err)
	}
	overlap := cosine(vecs[0], vecs[1])
	distinct := cosine(vecs[0], vecs[2])
	if !(overlap > distinct) {
		t.Fatalf("cos(overlapping) = %f, want > cos(distinct) = %f", overlap, distinct)
	}
}

// TestFakeEmbedderQueryEmbedderSameVectors asserts the fake's QueryEmbedder
// returns the same vectors as Embed, since the fake has no task-type split.
func TestFakeEmbedderQueryEmbedderSameVectors(t *testing.T) {
	e := embed.NewFake()
	qe, ok := e.(embed.QueryEmbedder)
	if !ok {
		t.Fatal("fake embedder does not implement QueryEmbedder")
	}
	ctx := context.Background()
	texts := []string{"vốn điều lệ ngân hàng"}
	docVecs, err := e.Embed(ctx, texts)
	if err != nil {
		t.Fatal(err)
	}
	queryVecs, err := qe.EmbedQueries(ctx, texts)
	if err != nil {
		t.Fatal(err)
	}
	if len(docVecs) != len(queryVecs) {
		t.Fatalf("doc vecs = %d, query vecs = %d", len(docVecs), len(queryVecs))
	}
	for i := range docVecs[0] {
		if docVecs[0][i] != queryVecs[0][i] {
			t.Fatalf("EmbedQueries differs from Embed at index %d: %f != %f", i, docVecs[0][i], queryVecs[0][i])
		}
	}
}

// TestFakeEmbedderFactEmbedderSameVectors asserts the fake's FactEmbedder
// returns the same vectors as Embed, since the fake has no task-type split.
func TestFakeEmbedderFactEmbedderSameVectors(t *testing.T) {
	e := embed.NewFake()
	fe, ok := e.(embed.FactEmbedder)
	if !ok {
		t.Fatal("fake embedder does not implement FactEmbedder")
	}
	ctx := context.Background()
	texts := []string{"vốn điều lệ ngân hàng", "kiểm toán nội bộ"}
	docVecs, err := e.Embed(ctx, texts)
	if err != nil {
		t.Fatal(err)
	}
	factVecs, err := fe.EmbedFact(ctx, texts)
	if err != nil {
		t.Fatal(err)
	}
	if len(docVecs) != len(factVecs) {
		t.Fatalf("doc vecs = %d, fact vecs = %d", len(docVecs), len(factVecs))
	}
	for i := range docVecs {
		if len(factVecs[i]) != 1536 {
			t.Fatalf("EmbedFact vec[%d] dim = %d, want 1536", i, len(factVecs[i]))
		}
		for j := range docVecs[i] {
			if docVecs[i][j] != factVecs[i][j] {
				t.Fatalf("EmbedFact differs from Embed at vec[%d][%d]: %f != %f",
					i, j, docVecs[i][j], factVecs[i][j])
			}
		}
	}
}

// cosine returns the cosine similarity between two equal-length vectors.
func cosine(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
