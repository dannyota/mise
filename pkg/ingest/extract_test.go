package ingest_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/mise/pkg/ingest"
	"danny.vn/mise/pkg/vertex"
)

// multiSectionParser is a consumer-side stub returning several sections, to
// prove Extractor joins them rather than keeping only the first.
type multiSectionParser struct{}

func (multiSectionParser) Parse(_ context.Context, _ []byte, _ string) (vertex.ParseResult, error) {
	return vertex.ParseResult{Sections: []vertex.Section{
		{HeadingPath: "Chương I", Text: "Điều 1. Phạm vi điều chỉnh"},
		{HeadingPath: "Chương I", Text: "Điều 2. Đối tượng áp dụng"},
	}}, nil
}

// writeFixture stores text under sha256(content).txt in dir, the key scheme
// vertex.NewFixtureParser reads.
func writeFixture(t *testing.T, dir string, content []byte, text string) {
	t.Helper()
	sum := sha256.Sum256(content)
	name := hex.EncodeToString(sum[:]) + ".txt"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestExtractorText(t *testing.T) {
	dir := t.TempDir()
	pdfContent := []byte("%PDF-1.4 fake pdf bytes")
	docxContent := []byte("PK fake docx bytes")
	writeFixture(t, dir, pdfContent, "trang một")
	writeFixture(t, dir, docxContent, "nội dung docx")
	e := ingest.NewExtractor(vertex.NewFixtureParser(dir))

	tests := []struct {
		name        string
		content     []byte
		contentType string
		want        string
	}{
		{
			name:        "html extracts locally line per block",
			content:     []byte("<p>Điều 1</p><script>x()</script><p>Điều 2</p>"),
			contentType: "text/html",
			want:        "Điều 1\nĐiều 2",
		},
		{
			name:        "html content type may carry parameters",
			content:     []byte("<p>a</p><p>b</p>"),
			contentType: "text/html; charset=utf-8",
			want:        "a\nb",
		},
		{
			name:        "plain text passes through",
			content:     []byte("Điều 1. Phạm vi\nĐiều 2. Đối tượng"),
			contentType: "text/plain",
			want:        "Điều 1. Phạm vi\nĐiều 2. Đối tượng",
		},
		{
			name:        "plain text content type may carry parameters",
			content:     []byte("nguyên văn"),
			contentType: "text/plain; charset=UTF-8",
			want:        "nguyên văn",
		},
		{
			name:        "pdf goes through the parser seam",
			content:     pdfContent,
			contentType: "application/pdf",
			want:        "trang một",
		},
		{
			name:        "docx goes through the parser seam",
			content:     docxContent,
			contentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			want:        "nội dung docx",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := e.Text(context.Background(), tt.content, tt.contentType)
			if err != nil {
				t.Fatalf("Text() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Text() = %q, want %q", got, tt.want)
			}
		})
	}
}

// recordingParser captures the content type it was invoked with.
type recordingParser struct {
	contentType string
}

func (r *recordingParser) Parse(_ context.Context, _ []byte, contentType string) (vertex.ParseResult, error) {
	r.contentType = contentType
	return vertex.ParseResult{Sections: []vertex.Section{{Text: "x"}}}, nil
}

func TestExtractorPassesBareMediaTypeToParser(t *testing.T) {
	// Doc AI rejects media-type parameters, so the parser must receive the
	// bare type even when the source served "application/pdf; name=a.pdf".
	rec := &recordingParser{}
	e := ingest.NewExtractor(rec)
	if _, err := e.Text(context.Background(), []byte("%PDF"), "application/pdf; name=a.pdf"); err != nil {
		t.Fatalf("Text() error = %v", err)
	}
	if rec.contentType != "application/pdf" {
		t.Errorf("parser received content type %q, want %q", rec.contentType, "application/pdf")
	}
}

func TestExtractorJoinsSectionsWithBlankLine(t *testing.T) {
	e := ingest.NewExtractor(multiSectionParser{})
	got, err := e.Text(context.Background(), []byte("%PDF"), "application/pdf")
	if err != nil {
		t.Fatalf("Text() error = %v", err)
	}
	want := "Điều 1. Phạm vi điều chỉnh\n\nĐiều 2. Đối tượng áp dụng"
	if got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
}

func TestExtractorUnknownContentTypeErrors(t *testing.T) {
	e := ingest.NewExtractor(vertex.NewFakeParser())
	for _, ct := range []string{"application/msword", "image/png", ""} {
		t.Run("ct="+ct, func(t *testing.T) {
			_, err := e.Text(context.Background(), []byte("x"), ct)
			if !errors.Is(err, ingest.ErrUnsupportedContentType) {
				t.Errorf("Text() error = %v, want ErrUnsupportedContentType", err)
			}
		})
	}
}

func TestExtractorParserErrorPropagates(t *testing.T) {
	// Empty fixture dir: the fixture parser errors on every content.
	e := ingest.NewExtractor(vertex.NewFixtureParser(t.TempDir()))
	if _, err := e.Text(context.Background(), []byte("%PDF"), "application/pdf"); err == nil {
		t.Error("Text() error = nil, want parser error to propagate")
	}
}
