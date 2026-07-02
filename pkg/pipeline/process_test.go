package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"danny.vn/mise/pkg/blob"
	"danny.vn/mise/pkg/ingest"
	"danny.vn/mise/pkg/parse/law"
)

// fakeSource is a minimal ingest.Source for unit tests; it does NOT implement
// TreeProvider (treeSource adds that).
type fakeSource struct {
	id       string
	detail   *ingest.DiscoveredDoc
	fileData map[string][]byte // Download payloads keyed by FileRef.URL
}

func (f *fakeSource) ID() string { return f.id }

func (f *fakeSource) Discover(context.Context, time.Time, string) ([]ingest.DiscoveredDoc, error) {
	return nil, nil
}

func (f *fakeSource) FetchDetail(context.Context, ingest.DetailRef) (*ingest.DiscoveredDoc, error) {
	return f.detail, nil
}

func (f *fakeSource) Download(_ context.Context, ref ingest.FileRef, w io.Writer) (int64, string, error) {
	data, ok := f.fileData[ref.URL]
	if !ok {
		return 0, "", errors.New("no such file")
	}
	n, err := w.Write(data)
	sum := sha256.Sum256(data)
	return int64(n), hex.EncodeToString(sum[:]), err
}

// treeSource is a fakeSource that also implements ingest.TreeProvider.
type treeSource struct {
	fakeSource
	payload string
	ok      bool
	err     error
}

func (t *treeSource) FetchTree(context.Context, ingest.DetailRef) (string, bool, error) {
	return t.payload, t.ok, t.err
}

const vnText = "Điều 1. Phạm vi điều chỉnh\nThông tư này quy định về an toàn hệ thống thông tin."

// Contentful and contentless VBPL provision-tree payloads. The contentless one
// decodes cleanly — ParseTree returns (tree, nil) — but flattens to nothing,
// which MUST route structureTree back to the text parser.
const (
	treeWithContent = `[{"id":"1","key":"k1","title":"Điều 1. Phạm vi điều chỉnh","level":"article",` +
		`"content":{"title":"Điều 1. Phạm vi điều chỉnh","content":"Nội dung điều một từ cây điều khoản."},` +
		`"children":[]}]`
	treeWithoutContent = `[{"id":"1","key":"k1","title":"Điều 1. Phạm vi điều chỉnh","level":"article",` +
		`"content":{"title":"Điều 1. Phạm vi điều chỉnh","content":""},"children":[]}]`
)

// bodies flattens a tree to its section bodies, joined for containment checks.
func bodies(t *testing.T, tree []*law.Node) string {
	t.Helper()
	flat := law.Flatten(tree)
	out := make([]string, 0, len(flat))
	for _, s := range flat {
		out = append(out, s.Body)
	}
	return strings.Join(out, "\n")
}

func TestStructureTreeVN(t *testing.T) {
	ctx := t.Context()
	dref := ingest.DetailRef{ExternalID: "1"}

	tests := []struct {
		name     string
		src      ingest.Source
		wantBody string // substring the flattened bodies must contain
	}{
		{
			name:     "tree provider with contentful tree uses the tree",
			src:      &treeSource{payload: treeWithContent, ok: true},
			wantBody: "Nội dung điều một từ cây điều khoản.",
		},
		{
			name: "contentless tree (ParseTree returns tree, nil) falls back to text parse",
			src:  &treeSource{payload: treeWithoutContent, ok: true},
			// The document's actual text — this is the branch that silently
			// drops content when the Flatten-empty check is missing.
			wantBody: "an toàn hệ thống thông tin",
		},
		{
			name:     "tree provider reporting ok=false falls back to text parse",
			src:      &treeSource{payload: "", ok: false},
			wantBody: "an toàn hệ thống thông tin",
		},
		{
			name:     "undecodable tree payload falls back to text parse",
			src:      &treeSource{payload: "not a provision tree", ok: true},
			wantBody: "an toàn hệ thống thông tin",
		},
		{
			name:     "source without TreeProvider parses text directly",
			src:      &fakeSource{},
			wantBody: "an toàn hệ thống thông tin",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree, err := structureTree(ctx, "vn", tt.src, dref, vnText)
			if err != nil {
				t.Fatalf("structureTree() error = %v", err)
			}
			if got := bodies(t, tree); !strings.Contains(got, tt.wantBody) {
				t.Errorf("flattened bodies = %q, want substring %q", got, tt.wantBody)
			}
		})
	}
}

func TestStructureTreeVNPropagatesFetchTreeError(t *testing.T) {
	src := &treeSource{err: errors.New("tree endpoint down")}
	if _, err := structureTree(t.Context(), "vn", src, ingest.DetailRef{}, vnText); err == nil {
		t.Fatal("structureTree() error = nil, want the FetchTree failure (transient, retryable)")
	}
}

func TestStructureTreeMY(t *testing.T) {
	text := "This Act may be cited as the Test Act 2026."
	tree, err := structureTree(t.Context(), "my", &fakeSource{}, ingest.DetailRef{}, text)
	if err != nil {
		t.Fatalf("structureTree() error = %v", err)
	}
	if got := bodies(t, tree); !strings.Contains(got, "Test Act 2026") {
		t.Errorf("flattened bodies = %q, want the document text", got)
	}
}

func TestStructureTreeUnknownJurisdictionReturnsNil(t *testing.T) {
	tree, err := structureTree(t.Context(), "xx", &fakeSource{}, ingest.DetailRef{}, "text")
	if err != nil {
		t.Fatalf("structureTree() error = %v", err)
	}
	if tree != nil {
		t.Errorf("structureTree() = %v, want nil (Normalize falls back to one whole-document section)", tree)
	}
}

func TestPickMainFile(t *testing.T) {
	appendix := ingest.FileRef{URL: "u1", Kind: "appendix"}
	main := ingest.FileRef{URL: "u2", Kind: "main"}
	scan := ingest.FileRef{URL: "u3", Kind: "original_scan"}

	if got, ok := pickMainFile([]ingest.FileRef{appendix, main, scan}); !ok || got != main {
		t.Errorf(`pickMainFile() = %+v, %v — want the "main" file`, got, ok)
	}
	if got, ok := pickMainFile([]ingest.FileRef{appendix, scan}); !ok || got != appendix {
		t.Errorf("pickMainFile() = %+v, %v — want the first file as fallback", got, ok)
	}
	if _, ok := pickMainFile(nil); ok {
		t.Error("pickMainFile(nil) reported ok for no files")
	}
}

func TestContentTypeFor(t *testing.T) {
	tests := []struct {
		name string
		ref  ingest.FileRef
		want string
	}{
		{"scraped mime wins", ingest.FileRef{MIMEType: "application/pdf", Ext: "docx"}, "application/pdf"},
		{"pdf ext", ingest.FileRef{Ext: "pdf"}, "application/pdf"},
		{
			"docx ext",
			ingest.FileRef{Ext: "docx"},
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		},
		{"doc ext", ingest.FileRef{Ext: "doc"}, "application/msword"},
		{"html ext", ingest.FileRef{Ext: "html"}, "text/html"},
		{"unknown ext", ingest.FileRef{Ext: "zip"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contentTypeFor(tt.ref); got != tt.want {
				t.Errorf("contentTypeFor(%+v) = %q, want %q", tt.ref, got, tt.want)
			}
		})
	}
}

func TestFetchMainContentPrefersInlineHTML(t *testing.T) {
	a := NewActivities(Deps{Blob: blob.NewFS(t.TempDir())})
	doc := &ingest.DiscoveredDoc{
		HTML:  "<p>body</p>",
		Files: []ingest.FileRef{{URL: "u", Kind: "main", Ext: "pdf"}},
	}
	got, err := a.fetchMainContent(t.Context(), &fakeSource{}, doc)
	if err != nil {
		t.Fatalf("fetchMainContent() error = %v", err)
	}
	if got.contentType != "text/html" || string(got.data) != "<p>body</p>" || got.blobKey != "" {
		t.Errorf("fetchMainContent() = %+v, want the inline HTML body and no blob key", got)
	}
}

func TestFetchMainContentDownloadsAndPreservesMainFile(t *testing.T) {
	fs := blob.NewFS(t.TempDir())
	a := NewActivities(Deps{Blob: fs})
	payload := []byte("%PDF-1.7 fake")
	src := &fakeSource{fileData: map[string][]byte{"u": payload}}
	doc := &ingest.DiscoveredDoc{
		Files: []ingest.FileRef{{URL: "u", Kind: "main", Ext: "pdf", MIMEType: "application/pdf"}},
	}

	got, err := a.fetchMainContent(t.Context(), src, doc)
	if err != nil {
		t.Fatalf("fetchMainContent() error = %v", err)
	}
	sum := sha256.Sum256(payload)
	wantKey := blob.Key(hex.EncodeToString(sum[:]), ".pdf")
	if got.blobKey != wantKey || got.contentType != "application/pdf" || string(got.data) != string(payload) {
		t.Errorf("fetchMainContent() = %+v, want key %q and the downloaded bytes", got, wantKey)
	}
	exists, err := fs.Exists(t.Context(), wantKey)
	if err != nil || !exists {
		t.Errorf("blob Exists(%q) = %v, %v — raw bytes must be preserved", wantKey, exists, err)
	}
}

func TestFetchMainContentNoContentIsPermanentFailure(t *testing.T) {
	a := NewActivities(Deps{Blob: blob.NewFS(t.TempDir())})
	_, err := a.fetchMainContent(t.Context(), &fakeSource{}, &ingest.DiscoveredDoc{})
	if !errors.Is(err, errNoMainContent) {
		t.Fatalf("fetchMainContent() error = %v, want errNoMainContent", err)
	}
}
