package vertex

import "context"

type fakeJudge struct{}

// NewFakeJudge returns a deterministic judge for offline/CI use.
func NewFakeJudge() Judge { return &fakeJudge{} }

func (f *fakeJudge) Judge(_ context.Context, _, _ string) (JudgeResult, error) {
	return JudgeResult{
		EdgeType:   "satisfies",
		Confidence: 0.85,
		Rationale:  "fake judge — offline mode",
	}, nil
}
