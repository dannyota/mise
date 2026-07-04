package sharepoint

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/ingest"
)

// fixtureServer returns an httptest server that simulates the SharePoint
// REST API endpoints. It serves files from the provided slice and supports
// pagination via odata.nextLink if page2Files is non-nil.
func fixtureServer(
	t *testing.T,
	files []spFile,
	subFolders []spFolder,
	page2Files []spFile,
) *httptest.Server {
	t.Helper()
	page1Served := false
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(
				"Content-Type",
				"application/json;odata=nometadata",
			)
			urlPath := r.URL.Path

			// File download ($value).
			if strings.HasSuffix(urlPath, "/$value") {
				name := extractFileNameFromValuePath(urlPath)
				_, _ = w.Write([]byte(name))
				return
			}

			// Single file fetch (FetchDetail).
			if strings.Contains(urlPath, "GetFileByServerRelativeUrl") &&
				!strings.Contains(urlPath, "/Files") {
				for _, f := range files {
					b, _ := json.Marshal(f)
					_, _ = w.Write(b)
					return
				}
				http.NotFound(w, r)
				return
			}

			// Folders endpoint.
			if strings.HasSuffix(urlPath, "/Folders") {
				resp := spFoldersResponse{Value: subFolders}
				b, _ := json.Marshal(resp)
				_, _ = w.Write(b)
				return
			}

			// Page 2 of files (pagination test).
			if r.URL.Path == "/page2" {
				resp := spFilesResponse{Value: page2Files}
				b, _ := json.Marshal(resp)
				_, _ = w.Write(b)
				return
			}

			// Files endpoint (page 1).
			if strings.Contains(urlPath, "/Files") {
				var nextLink string
				if page2Files != nil && !page1Served {
					page1Served = true
					nextLink = "http://" + r.Host + "/page2"
				}
				resp := spFilesResponse{
					Value:    files,
					NextLink: nextLink,
				}
				b, _ := json.Marshal(resp)
				_, _ = w.Write(b)
				return
			}

			http.NotFound(w, r)
		},
	))
	t.Cleanup(srv.Close)
	return srv
}

func extractFileNameFromValuePath(p string) string {
	start := strings.Index(p, "('")
	end := strings.Index(p, "')")
	if start >= 0 && end > start {
		rel := p[start+2 : end]
		rel = strings.ReplaceAll(rel, "''", "'")
		parts := strings.Split(rel, "/")
		return parts[len(parts)-1]
	}
	return "unknown"
}

func testFiles() []spFile {
	return []spFile{
		{
			Name:              "Information Security Standard.pdf",
			ServerRelativeURL: "/sites/controls/Docs/Info.pdf",
			TimeLastModified: time.Date(
				2026, 6, 15, 10, 0, 0, 0, time.UTC,
			),
			UniqueID: "aaa-111",
			ETag:     "\"{AABB1122-3344-5566-7788-99AABBCCDDEE},1\"",
			ListItemAllFields: makeFields(map[string]string{
				"Title":           "Group Information Security Standard",
				"DocumentNumber":  "GRP-STD-014",
				"SignerRole":      "Chief Risk Officer",
				"OwnerDepartment": "Information Security",
				"OwnerRole":       "Head of InfoSec",
				"Version0":        "3.2",
				"Language":        "en",
				"IssuedDate":      "2026-01-15",
				"EffectiveDate":   "2026-02-01",
			}),
		},
		{
			Name:              "AML Policy.docx",
			ServerRelativeURL: "/sites/controls/Docs/AML Policy.docx",
			TimeLastModified: time.Date(
				2026, 5, 1, 8, 0, 0, 0, time.UTC,
			),
			UniqueID: "bbb-222",
			ETag:     "\"{CCDD1122-3344-5566-7788-99AABBCCDDEE},2\"",
			ListItemAllFields: makeFields(map[string]string{
				"Title":          "Anti-Money Laundering Policy",
				"DocumentNumber": "LOC-POL-007",
			}),
		},
	}
}

func makeFields(m map[string]string) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(m))
	for k, v := range m {
		b, _ := json.Marshal(v)
		out[k] = b
	}
	return out
}

func newTestSource(t *testing.T, srv *httptest.Server) *Source {
	t.Helper()
	src, err := New(
		srv.URL, "/sites/controls/Docs",
		corpus.GroupStd, FakeAuth{}, srv.Client(), nil,
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return src
}

func TestNewRejectsUnregisteredCorpus(t *testing.T) {
	_, err := New(
		"http://x", "/lib", corpus.ID("bogus"),
		FakeAuth{}, nil, nil,
	)
	if err == nil {
		t.Fatal("expected error for unregistered corpus id, got nil")
	}
}

func TestFetchDetailFieldMapping(t *testing.T) {
	files := testFiles()
	srv := fixtureServer(t, files, nil, nil)
	src := newTestSource(t, srv)

	ref := ingest.DetailRef{
		ExternalID: "aaa-111",
		DetailURL:  "/sites/controls/Docs/Info.pdf",
	}
	doc, err := src.FetchDetail(context.Background(), ref)
	if err != nil {
		t.Fatalf("FetchDetail: %v", err)
	}

	checks := []struct {
		name string
		got  string
		want string
	}{
		{"Title", doc.Title, "Group Information Security Standard"},
		{"Number", doc.Number, "GRP-STD-014"},
		{"SignerRole", doc.SignerRole, "Chief Risk Officer"},
		{"OwnerDepartment", doc.OwnerDepartment, "Information Security"},
		{"OwnerRole", doc.OwnerRole, "Head of InfoSec"},
		{"Version", doc.Version, "3.2"},
		{"Language", doc.Language, "en"},
		{"SourceID", doc.SourceID, SourceID},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("doc.%s = %q, want %q", c.name, c.got, c.want)
		}
	}
	if doc.IssuedAt != time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("IssuedAt = %v", doc.IssuedAt)
	}
	if doc.EffectiveAt != time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("EffectiveAt = %v", doc.EffectiveAt)
	}
	if len(doc.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(doc.Files))
	}
	if doc.Files[0].Kind != "main" || doc.Files[0].Ext != "pdf" {
		t.Fatalf("file = %+v", doc.Files[0])
	}
}

func TestDownloadSHACorrectness(t *testing.T) {
	srv := fixtureServer(t, testFiles(), nil, nil)
	src := newTestSource(t, srv)

	ref := ingest.FileRef{
		URL: srv.URL +
			"/_api/web/GetFileByServerRelativeUrl(" +
			"'/sites/controls/Docs/Info.pdf')/$value",
		Name: "Info.pdf",
		Ext:  "pdf",
	}

	var buf bytes.Buffer
	n, sha, err := src.Download(context.Background(), ref, &buf)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	content := buf.Bytes()
	if n != int64(len(content)) {
		t.Fatalf("n = %d, len = %d", n, len(content))
	}
	h := sha256.Sum256(content)
	want := hex.EncodeToString(h[:])
	if sha != want {
		t.Fatalf("sha = %q, want %q", sha, want)
	}
}

func TestDownloadEmptyURLErrors(t *testing.T) {
	srv := fixtureServer(t, testFiles(), nil, nil)
	src := newTestSource(t, srv)
	_, _, err := src.Download(
		context.Background(), ingest.FileRef{}, &bytes.Buffer{},
	)
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestSourceID(t *testing.T) {
	srv := fixtureServer(t, testFiles(), nil, nil)
	src := newTestSource(t, srv)
	if got := src.ID(); got != SourceID {
		t.Fatalf("ID = %q, want %q", got, SourceID)
	}
}

func TestOdataEscapePreservesSlashesDoubleApostrophes(t *testing.T) {
	input := "/sites/controls/Bob's Docs/Sub Folder"
	want := "/sites/controls/Bob''s Docs/Sub Folder"
	got := odataEscape(input)
	if got != want {
		t.Fatalf("odataEscape(%q) = %q, want %q", input, got, want)
	}

	src := &Source{siteURL: "https://tenant.sharepoint.com"}
	url := src.fileAPIURL(input)
	expected := "https://tenant.sharepoint.com" +
		"/_api/web/GetFileByServerRelativeUrl(" +
		"'/sites/controls/Bob''s Docs/Sub Folder')"
	if url != expected {
		t.Fatalf("fileAPIURL = %q, want %q", url, expected)
	}
}

func TestStaticAuthFailsWhenEmpty(t *testing.T) {
	a := &StaticAuth{}
	req, _ := http.NewRequest(http.MethodGet, "http://x", nil)
	if err := a.Apply(req); err == nil {
		t.Fatal("expected error when both Cookie and Bearer are empty")
	}
}
