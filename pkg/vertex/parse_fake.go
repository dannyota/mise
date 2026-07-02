package vertex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

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

type fixtureParser struct {
	dir string
}

// NewFixtureParser returns a Parser that replays canned extractions from dir
// for offline/CI use. Content is keyed by its SHA-256 (lowercase hex): Parse
// reads dir/<sha256(content)>.txt and returns the file's contents as a single
// section. A missing fixture is an error, so tests fail loudly instead of
// silently parsing nothing.
func NewFixtureParser(dir string) Parser { return &fixtureParser{dir: dir} }

func (f *fixtureParser) Parse(_ context.Context, content []byte, _ string) (ParseResult, error) {
	root, err := os.OpenRoot(f.dir)
	if err != nil {
		return ParseResult{}, fmt.Errorf("opening fixture dir: %w", err)
	}
	defer func() { _ = root.Close() }()

	sum := sha256.Sum256(content)
	name := hex.EncodeToString(sum[:]) + ".txt"
	text, err := root.ReadFile(name)
	if err != nil {
		return ParseResult{}, fmt.Errorf("reading parse fixture %s: %w", name, err)
	}
	return ParseResult{Sections: []Section{{Text: string(text)}}}, nil
}
