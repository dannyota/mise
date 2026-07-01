package vertex

import "context"

type fakeParser struct{}

// NewFakeParser returns a deterministic parser for offline/CI use.
func NewFakeParser() Parser { return &fakeParser{} }

func (f *fakeParser) Parse(_ context.Context, content []byte, _ string) (ParseResult, error) {
	return ParseResult{
		Sections: []Section{
			{HeadingPath: "Section 1", Text: string(content)},
		},
	}, nil
}
