package vertex

import "context"

type fakeRanker struct{}

// NewFakeRanker returns a deterministic ranker for offline/CI use. Documents
// are returned in input order with descending scores (1.0, 0.9, …, clamped
// to 0.0).
func NewFakeRanker() Ranker { return &fakeRanker{} }

func (f *fakeRanker) Rerank(_ context.Context, _ string, docs []string, topK int) ([]RankedDoc, error) {
	n := len(docs)
	if topK > 0 && topK < n {
		n = topK
	}
	out := make([]RankedDoc, n)
	for i := range n {
		score := 1.0 - 0.1*float64(i)
		if score < 0 {
			score = 0
		}
		out[i] = RankedDoc{Index: i, Score: score}
	}
	return out, nil
}
