package vertex

import "context"

// GroundResult holds the Check Grounding result.
type GroundResult struct {
	Grounded bool
	Score    float64
}

// Grounder verifies claims against source text using Check Grounding.
type Grounder interface {
	Ground(ctx context.Context, claim, source string) (GroundResult, error)
}
