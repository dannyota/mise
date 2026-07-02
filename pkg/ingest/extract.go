package ingest

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"danny.vn/mise/pkg/parse/htmltext"
	"danny.vn/mise/pkg/vertex"
)

// MIME types the Extractor dispatches on. HTML and plain text are handled
// locally; PDF and DOCX go through the vertex.Parser seam (Doc AI Layout
// Parser in the real deployment, a fake/fixture offline).
const (
	mimeHTML  = "text/html"
	mimePlain = "text/plain"
	mimePDF   = "application/pdf"
	mimeDOCX  = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
)

// ErrUnsupportedContentType marks content types Extractor.Text cannot turn
// into text; callers classify with errors.Is.
var ErrUnsupportedContentType = errors.New("unsupported content type")

// Extractor turns a fetched artifact's raw bytes into plain text for the
// downstream legal-structure parsers, dispatching on content type.
type Extractor struct {
	parser vertex.Parser
}

// NewExtractor returns an Extractor that sends PDF/DOCX content through p.
func NewExtractor(p vertex.Parser) *Extractor {
	return &Extractor{parser: p}
}

// Text extracts plain text from content. text/html is extracted locally with
// a stable one-line-per-block discipline (htmltext); text/plain passes
// through; application/pdf and DOCX go through the parser seam, joining
// section texts with a blank line. contentType may carry media-type
// parameters ("text/html; charset=utf-8"); unknown types return
// ErrUnsupportedContentType.
func (e *Extractor) Text(ctx context.Context, content []byte, contentType string) (string, error) {
	switch mt := mediaType(contentType); mt {
	case mimeHTML:
		return htmltext.Text(content), nil
	case mimePlain:
		return string(content), nil
	case mimePDF, mimeDOCX:
		// The parser gets the bare media type: Doc AI rejects parameters
		// ("application/pdf; name=a.pdf").
		result, err := e.parser.Parse(ctx, content, mt)
		if err != nil {
			return "", fmt.Errorf("parsing %s content: %w", mt, err)
		}
		texts := make([]string, len(result.Sections))
		for i, s := range result.Sections {
			texts[i] = s.Text
		}
		return strings.Join(texts, "\n\n"), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedContentType, contentType)
	}
}

// mediaType strips media-type parameters and normalizes case:
// "text/HTML; charset=utf-8" → "text/html".
func mediaType(contentType string) string {
	mt, _, _ := strings.Cut(contentType, ";")
	return strings.ToLower(strings.TrimSpace(mt))
}
