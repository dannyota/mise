package embed

import (
	"context"
	"hash/fnv"
	"math"
	"strings"
	"unicode"
)

type fakeEmbedder struct{}

// NewFake returns a deterministic embedder for offline/CI use.
func NewFake() Embedder { return &fakeEmbedder{} }

func (f *fakeEmbedder) Model() string { return "gemini-embedding-001" }
func (f *fakeEmbedder) Dims() int     { return 1536 }

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	vecs := make([][]float32, len(texts))
	for i, t := range texts {
		vecs[i] = tokenBagVector(t, 1536)
	}
	return vecs, nil
}

// EmbedQueries implements QueryEmbedder. The fake has no document/query
// split, so it returns exactly what Embed would.
func (f *fakeEmbedder) EmbedQueries(ctx context.Context, texts []string) ([][]float32, error) {
	return f.Embed(ctx, texts)
}

// tokenBagVector embeds text as a bag of tokens so texts sharing vocabulary
// score higher on cosine similarity — the offline stand-in for real semantic
// embedding. Text is lowercased and split into unicode-aware letter/digit
// runs; each token is hashed to a unit vector (deterministicVector), the
// per-token vectors are summed, and the sum is L2-normalized. Text with no
// tokens (empty, or only punctuation/whitespace) falls back to hashing the
// whole string, matching the original whole-string behavior.
func tokenBagVector(text string, dims int) []float32 {
	tokens := tokenize(text)
	if len(tokens) == 0 {
		return deterministicVector(text, dims)
	}

	sum := make([]float64, dims)
	for _, tok := range tokens {
		for i, x := range deterministicVector(tok, dims) {
			sum[i] += float64(x)
		}
	}

	vec := make([]float32, dims)
	var norm float64
	for _, x := range sum {
		norm += x * x
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		return vec
	}
	for i, x := range sum {
		vec[i] = float32(x / norm)
	}
	return vec
}

// tokenize lowercases text and splits it into unicode-aware runs of letters
// and digits, dropping everything else (whitespace, punctuation, symbols).
func tokenize(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// deterministicVector hashes text to a reproducible unit vector: an FNV-64a
// digest seeds an xorshift generator whose outputs become the vector
// components, which are then L2-normalized.
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
