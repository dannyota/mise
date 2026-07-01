package vertex

import "context"

type fakeGrounder struct{}

// NewFakeGrounder returns a deterministic grounder for offline/CI use.
func NewFakeGrounder() Grounder { return &fakeGrounder{} }

func (f *fakeGrounder) Ground(_ context.Context, _, _ string) (GroundResult, error) {
	return GroundResult{Grounded: true, Score: 0.95}, nil
}
