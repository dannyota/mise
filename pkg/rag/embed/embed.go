// Package embed defines the Embedder interface and adapters.
package embed

import "context"

// Embedder turns text into embedding vectors.
type Embedder interface {
	Model() string
	Dims() int
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}
