package embed

import (
	"context"
	"hash/fnv"
	"math"
)

type fakeEmbedder struct{}

// NewFake returns a deterministic embedder for offline/CI use.
func NewFake() Embedder { return &fakeEmbedder{} }

func (f *fakeEmbedder) Model() string { return "gemini-embedding-001" }
func (f *fakeEmbedder) Dims() int     { return 1536 }

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	vecs := make([][]float32, len(texts))
	for i, t := range texts {
		vecs[i] = deterministicVector(t, 1536)
	}
	return vecs, nil
}

func deterministicVector(text string, dims int) []float32 {
	h := fnv.New64a()
	// hash.Write never returns error; gosec error is false positive
	//nolint:gosec,G104
	_, _ = h.Write([]byte(text))
	seed := h.Sum64()

	vec := make([]float32, dims)
	var norm float64
	for i := range vec {
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		vec[i] = float32(seed>>1) / float32(math.MaxInt64)
		norm += float64(vec[i]) * float64(vec[i])
	}
	norm = math.Sqrt(norm)
	for i := range vec {
		vec[i] = float32(float64(vec[i]) / norm)
	}
	return vec
}
