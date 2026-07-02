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
		return joinSections(result.Sections), nil
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

// headingPathSep is the separator vertex's sectionWalker.headingPath
// (pkg/vertex/parse_docai.go) joins heading segments with, outermost first.
const headingPathSep = " > "

// joinSections joins a PDF/DOCX parse's sections into one linear text stream
// for the downstream legal-structure parsers (pkg/parse/vnlaw, pkg/parse/
// mylaw): deterministic line-by-line state machines that rebuild citation
// paths by matching heading lines like "Điều 7" or "PART III" on their own
// line.
//
// Doc AI's Layout Parser never emits a heading block as a section's own Text
// — a heading-N block's text is recorded ONLY in the HeadingPath of the
// sections that follow it (vertex.sectionWalker.textBlock) — so joining just
// s.Text (the prior behavior) silently drops every heading line the
// downstream parsers key on, and a PDF/DOCX document parses into a
// structureless blob with no citation paths.
//
// joinSections reconstructs the heading lines: for each section, the
// HeadingPath segments newly entered since the previous section's path are
// emitted as their own lines immediately before that section's body text. A
// heading already emitted for a prior section (an unchanged path prefix,
// e.g. sibling list items under one Điều) is never repeated. Distinct
// sections stay separated by a blank line, matching the pre-fix join of
// section bodies.
func joinSections(sections []vertex.Section) string {
	parts := make([]string, 0, len(sections))
	var prev []string
	for _, s := range sections {
		cur := splitHeadingPath(s.HeadingPath)
		lines := newHeadingLines(prev, cur)
		if s.Text != "" {
			lines = append(lines, s.Text)
		}
		if len(lines) > 0 {
			parts = append(parts, strings.Join(lines, "\n"))
		}
		prev = cur
	}
	return strings.Join(parts, "\n\n")
}

// splitHeadingPath splits a Section.HeadingPath on the outermost-first
// headingPathSep separator sectionWalker.headingPath joins with. An empty
// path returns nil.
func splitHeadingPath(path string) []string {
	if path == "" {
		return nil
	}
	return strings.Split(path, headingPathSep)
}

// newHeadingLines returns the cur heading segments from the first index
// where cur diverges from prev — the segments this section enters for the
// first time and must still be emitted as a heading line.
func newHeadingLines(prev, cur []string) []string {
	i := 0
	for i < len(prev) && i < len(cur) && prev[i] == cur[i] {
		i++
	}
	return cur[i:]
}
