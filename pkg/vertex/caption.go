package vertex

import (
	"context"
	"time"
)

// CaptionResult holds the output of figure/table captioning.
type CaptionResult struct {
	Text      string
	Model     string
	Prompt    string
	CreatedAt time.Time
}

// Captioner produces text captions for document figures/tables.
type Captioner interface {
	Caption(ctx context.Context, image []byte, contentType string) (CaptionResult, error)
}
