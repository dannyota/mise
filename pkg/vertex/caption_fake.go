package vertex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"
)

type fakeCaptioner struct{}

// NewFakeCaptioner returns a deterministic captioner for offline/CI use.
func NewFakeCaptioner() Captioner { return &fakeCaptioner{} }

func (f *fakeCaptioner) Caption(_ context.Context, image []byte, _ string) (CaptionResult, error) {
	return CaptionResult{
		Text:      fmt.Sprintf("fake caption for %d-byte image", len(image)),
		Model:     "fake-vision",
		Prompt:    "Describe this figure.",
		CreatedAt: time.Now().UTC(),
	}, nil
}

type fixtureCaptioner struct {
	dir string
}

// NewFixtureCaptioner returns a Captioner that replays canned captions
// from dir keyed by sha256 of image bytes.
func NewFixtureCaptioner(dir string) Captioner { return &fixtureCaptioner{dir: dir} }

func (f *fixtureCaptioner) Caption(_ context.Context, image []byte, _ string) (CaptionResult, error) {
	root, err := os.OpenRoot(f.dir)
	if err != nil {
		return CaptionResult{}, fmt.Errorf("opening caption fixture dir: %w", err)
	}
	defer func() { _ = root.Close() }()

	sum := sha256.Sum256(image)
	name := hex.EncodeToString(sum[:]) + ".txt"
	text, err := root.ReadFile(name)
	if err != nil {
		return CaptionResult{}, fmt.Errorf("reading caption fixture %s: %w", name, err)
	}
	return CaptionResult{
		Text:      string(text),
		Model:     "fixture-vision",
		Prompt:    "Describe this figure.",
		CreatedAt: time.Now().UTC(),
	}, nil
}
