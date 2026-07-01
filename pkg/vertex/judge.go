package vertex

import "context"

// JudgeResult holds an edge classification from Gemini.
type JudgeResult struct {
	EdgeType   string
	Confidence float64
	FromSpan   string
	ToSpan     string
	Rationale  string
}

// Judge classifies potential relation edges using Gemini.
type Judge interface {
	Judge(ctx context.Context, fromText, toText string) (JudgeResult, error)
}
