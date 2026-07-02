package vertex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

	// docAIMaxAttempts bounds the :process call: one initial try plus three
	// retries on throttling (429) and server errors (5xx).
	docAIMaxAttempts = 4

	// docAIMaxErrBody caps how much of an error response body is quoted.
	docAIMaxErrBody = 512
)

// docAIParser calls the Doc AI Layout Parser REST API to turn PDF/DOCX bytes
// into layout-aware sections.
type docAIParser struct {
	endpoint string        // full …/processors/<id>:process URL
	client   *http.Client  // OAuth2-wrapped HTTP client
	backoff  time.Duration // base retry delay, doubled per retry
}

// NewDocAIParser returns a Parser backed by a Doc AI Layout Parser processor
// (REST :process endpoint). Credentials come from Application Default
// Credentials; location is a Doc AI region ("us", "eu"). The returned parser
// retries throttled (429) and failed (5xx) calls with exponential backoff.
func NewDocAIParser(project, location, processorID string) (Parser, error) {
	if project == "" || location == "" || processorID == "" {
		return nil, errors.New("doc ai parser: project, location, and processor id are all required")
	}
	ts, err := google.DefaultTokenSource(context.Background(), cloudPlatformScope)
	if err != nil {
		return nil, fmt.Errorf("creating doc ai token source: %w", err)
	}
	return &docAIParser{
		endpoint: fmt.Sprintf(
			"https://%s-documentai.googleapis.com/v1/projects/%s/locations/%s/processors/%s:process",
			location, project, location, processorID,
		),
		client:  oauth2.NewClient(context.Background(), ts),
		backoff: time.Second,
	}, nil
}

// processRequest is the :process request body for an inline document.
type processRequest struct {
	RawDocument rawDocument `json:"rawDocument"`
}

// rawDocument carries the document bytes; encoding/json marshals []byte as
// base64, which is exactly the wire format the API expects.
type rawDocument struct {
	Content  []byte `json:"content"`
	MIMEType string `json:"mimeType"`
}

// processResponse is the subset of the :process response the parser reads.
type processResponse struct {
	Document layoutDocument `json:"document"`
}

type layoutDocument struct {
	DocumentLayout documentLayout `json:"documentLayout"`
}

type documentLayout struct {
	Blocks []layoutBlock `json:"blocks"`
}

// layoutBlock is one node of the Layout Parser block tree; exactly one of the
// union fields is set.
type layoutBlock struct {
	TextBlock  *layoutTextBlock  `json:"textBlock"`
	ListBlock  *layoutListBlock  `json:"listBlock"`
	TableBlock *layoutTableBlock `json:"tableBlock"`
}

type layoutTextBlock struct {
	Text   string        `json:"text"`
	Type   string        `json:"type"` // paragraph, title, heading-1 … heading-6, …
	Blocks []layoutBlock `json:"blocks"`
}

type layoutListBlock struct {
	ListEntries []layoutListEntry `json:"listEntries"`
}

type layoutListEntry struct {
	Blocks []layoutBlock `json:"blocks"`
}

type layoutTableBlock struct {
	HeaderRows []layoutTableRow `json:"headerRows"`
	BodyRows   []layoutTableRow `json:"bodyRows"`
}

type layoutTableRow struct {
	Cells []layoutTableCell `json:"cells"`
}

type layoutTableCell struct {
	Blocks []layoutBlock `json:"blocks"`
}

// Parse implements Parser via the Doc AI Layout Parser :process endpoint.
func (p *docAIParser) Parse(ctx context.Context, content []byte, contentType string) (ParseResult, error) {
	payload, err := json.Marshal(processRequest{
		RawDocument: rawDocument{Content: content, MIMEType: contentType},
	})
	if err != nil {
		return ParseResult{}, fmt.Errorf("encoding doc ai request: %w", err)
	}

	body, err := p.post(ctx, payload)
	if err != nil {
		return ParseResult{}, err
	}

	var resp processResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return ParseResult{}, fmt.Errorf("decoding doc ai response: %w", err)
	}
	return ParseResult{Sections: layoutSections(resp.Document.DocumentLayout.Blocks)}, nil
}

// post sends payload to the :process endpoint, retrying 429/5xx (and
// transport errors) with exponential backoff up to docAIMaxAttempts.
func (p *docAIParser) post(ctx context.Context, payload []byte) ([]byte, error) {
	var lastErr error
	for attempt := range docAIMaxAttempts {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("waiting to retry doc ai call: %w", ctx.Err())
			case <-time.After(p.backoff << (attempt - 1)):
			}
		}
		body, retryable, err := p.postOnce(ctx, payload)
		if err == nil {
			return body, nil
		}
		if !retryable {
			return nil, err
		}
		lastErr = err
	}
	return nil, fmt.Errorf("doc ai call failed after %d attempts: %w", docAIMaxAttempts, lastErr)
}

// postOnce performs a single :process call. retryable reports whether the
// failure is transient (throttling, server error, transport error).
func (p *docAIParser) postOnce(ctx context.Context, payload []byte) (body []byte, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, false, fmt.Errorf("building doc ai request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("calling doc ai: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, fmt.Errorf("reading doc ai response: %w", err)
	}
	if resp.StatusCode == http.StatusOK {
		return body, false, nil
	}
	retryable = resp.StatusCode == http.StatusTooManyRequests ||
		resp.StatusCode >= http.StatusInternalServerError
	return nil, retryable, fmt.Errorf("doc ai returned %s: %s", resp.Status, truncate(body, docAIMaxErrBody))
}

// layoutSections flattens a Layout Parser block tree into sections. Text
// blocks typed heading-1…heading-6 maintain the running heading path (a
// deeper heading is cleared when a shallower one starts); every other
// non-empty text block — including list entries and table cells — becomes a
// Section under the current path. No block type is filtered out: page
// furniture is left to the downstream legal parsers to discard.
func layoutSections(blocks []layoutBlock) []Section {
	w := &sectionWalker{}
	w.walk(blocks)
	return w.sections
}

// sectionWalker accumulates sections while tracking the active heading text
// per level (headings[i] belongs to heading-(i+1)).
type sectionWalker struct {
	sections []Section
	headings [6]string
}

func (w *sectionWalker) walk(blocks []layoutBlock) {
	for _, b := range blocks {
		switch {
		case b.TextBlock != nil:
			w.textBlock(b.TextBlock)
		case b.ListBlock != nil:
			for _, entry := range b.ListBlock.ListEntries {
				w.walk(entry.Blocks)
			}
		case b.TableBlock != nil:
			w.tableBlock(b.TableBlock)
		}
	}
}

func (w *sectionWalker) textBlock(tb *layoutTextBlock) {
	text := strings.TrimSpace(tb.Text)
	if level, ok := headingLevel(tb.Type); ok {
		if text != "" {
			w.headings[level-1] = text
			clear(w.headings[level:])
		}
	} else if text != "" {
		w.sections = append(w.sections, Section{HeadingPath: w.headingPath(), Text: text})
	}
	w.walk(tb.Blocks)
}

func (w *sectionWalker) tableBlock(tb *layoutTableBlock) {
	for _, rows := range [][]layoutTableRow{tb.HeaderRows, tb.BodyRows} {
		for _, row := range rows {
			for _, cell := range row.Cells {
				w.walk(cell.Blocks)
			}
		}
	}
}

// headingPath joins the active headings, outermost first.
func (w *sectionWalker) headingPath() string {
	parts := make([]string, 0, len(w.headings))
	for _, h := range w.headings {
		if h != "" {
			parts = append(parts, h)
		}
	}
	return strings.Join(parts, " > ")
}

// headingLevel parses "heading-N" block types; ok is false for content types.
func headingLevel(blockType string) (int, bool) {
	s, ok := strings.CutPrefix(blockType, "heading-")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 6 {
		return 0, false
	}
	return n, true
}

// truncate clips b to at most n bytes for error messages.
func truncate(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return b[:n]
}
