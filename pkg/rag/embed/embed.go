// Package embed defines the Embedder interface and adapters.
package embed

import "context"

// Embedder turns text into embedding vectors.
type Embedder interface {
	Model() string
	Dims() int
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// QueryEmbedder is an optional Embedder capability for adapters that embed
// search queries differently from documents (Vertex's gemini-embedding-001
// splits RETRIEVAL_DOCUMENT vs RETRIEVAL_QUERY task types). Callers that need
// query-side embedding should type-assert for it and fall back to Embed when
// an Embedder does not implement it:
//
//	vecs, err := embedder.Embed(ctx, queries)
//	if qe, ok := embedder.(QueryEmbedder); ok {
//		vecs, err = qe.EmbedQueries(ctx, queries)
//	}
type QueryEmbedder interface {
	EmbedQueries(ctx context.Context, texts []string) ([][]float32, error)
}

// FactEmbedder is an optional Embedder capability for adapters that embed
// candidate pairs for fact-verification scoring (Vertex's
// gemini-embedding-001 FACT_VERIFICATION task type). Callers should
// type-assert and fall back to Embed when an Embedder does not implement it:
//
//	vecs, err := embedder.Embed(ctx, texts)
//	if fe, ok := embedder.(FactEmbedder); ok {
//		vecs, err = fe.EmbedFact(ctx, texts)
//	}
type FactEmbedder interface {
	EmbedFact(ctx context.Context, texts []string) ([][]float32, error)
}
