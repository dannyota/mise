package vertex

import "context"

// RankedDoc holds one reranked document reference.
type RankedDoc struct {
	Index int     // original position in the input docs slice
	Score float64 // rerank score
}

// Ranker reranks documents against a query using the Vertex Discovery Engine
// Ranking API.
type Ranker interface {
	Rerank(ctx context.Context, query string, docs []string, topK int) ([]RankedDoc, error)
}
