package sharepoint

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/ingest"
)

func TestDiscoverReturnsNewestFirst(t *testing.T) {
	srv := fixtureServer(t, testFiles(), nil, nil)
	src := newTestSource(t, srv)

	docs, err := src.Discover(context.Background(), time.Time{}, "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("docs = %d, want 2", len(docs))
	}
	if docs[0].PublishedAt.Before(docs[1].PublishedAt) {
		t.Fatalf(
			"not newest-first: %v then %v",
			docs[0].PublishedAt, docs[1].PublishedAt,
		)
	}
}

func TestDiscoverWatermarkStrictlyAfter(t *testing.T) {
	srv := fixtureServer(t, testFiles(), nil, nil)
	src := newTestSource(t, srv)

	since := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	docs, err := src.Discover(context.Background(), since, "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("docs = %d, want 1 (mtime == since excluded)", len(docs))
	}
	if docs[0].ExternalID != "aaa-111" {
		t.Fatalf("expected newer doc, got %q", docs[0].ExternalID)
	}
}

func TestDiscoverKeywordFilter(t *testing.T) {
	srv := fixtureServer(t, testFiles(), nil, nil)
	src := newTestSource(t, srv)

	docs, err := src.Discover(
		context.Background(), time.Time{}, "security",
	)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("docs = %d, want 1", len(docs))
	}
	if docs[0].Title != "Group Information Security Standard" {
		t.Fatalf("title = %q", docs[0].Title)
	}
}

func TestDiscoverKeywordMatchesNumber(t *testing.T) {
	srv := fixtureServer(t, testFiles(), nil, nil)
	src := newTestSource(t, srv)

	docs, err := src.Discover(
		context.Background(), time.Time{}, "LOC-POL",
	)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("docs = %d, want 1", len(docs))
	}
	if docs[0].Number != "LOC-POL-007" {
		t.Fatalf("number = %q", docs[0].Number)
	}
}

func TestDiscoverContentFingerprint(t *testing.T) {
	files := testFiles()
	srv := fixtureServer(t, files, nil, nil)
	src := newTestSource(t, srv)

	docs, err := src.Discover(context.Background(), time.Time{}, "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(docs) < 1 {
		t.Fatal("no docs")
	}

	want := normalizeETag(files[0].ETag)
	fp := findDoc(docs, "aaa-111").ContentFingerprint
	if fp != want {
		t.Fatalf("fingerprint = %q, want %q", fp, want)
	}
}

func TestDiscoverContentFingerprintChangesOnETagChange(t *testing.T) {
	files := testFiles()
	srv1 := fixtureServer(t, files, nil, nil)
	src1 := newTestSource(t, srv1)
	docs1, _ := src1.Discover(context.Background(), time.Time{}, "")

	files2 := testFiles()
	files2[0].ETag = "\"{NEW-ETAG-VALUE},3\""
	srv2 := fixtureServer(t, files2, nil, nil)
	src2 := newTestSource(t, srv2)
	docs2, _ := src2.Discover(context.Background(), time.Time{}, "")

	fp1 := findDoc(docs1, "aaa-111").ContentFingerprint
	fp2 := findDoc(docs2, "aaa-111").ContentFingerprint
	if fp1 == fp2 {
		t.Fatalf("fingerprint unchanged after ETag change: %q", fp1)
	}
}

func TestDiscoverAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	))
	t.Cleanup(srv.Close)

	src := newTestSource(t, srv)
	_, err := src.Discover(context.Background(), time.Time{}, "")
	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}
	if !strings.Contains(err.Error(), "session or credentials are invalid") {
		t.Fatalf("error = %q, want fail-closed message", err.Error())
	}
}

func TestDiscoverAuthFailureBrokenBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	))
	t.Cleanup(srv.Close)

	src := newTestSource(t, srv)
	_, err := src.Discover(context.Background(), time.Time{}, "")
	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}
	if !strings.Contains(err.Error(), "session or credentials are invalid") {
		t.Fatalf("error = %q, want fail-closed message", err.Error())
	}
}

func TestDiscoverRetryOn429(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			attempts++
			if attempts == 1 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.Header().Set(
				"Content-Type",
				"application/json;odata=nometadata",
			)
			urlPath := r.URL.Path
			if strings.HasSuffix(urlPath, "/Folders") {
				_, _ = w.Write([]byte(`{"value":[]}`))
				return
			}
			resp := spFilesResponse{Value: testFiles()}
			b, _ := json.Marshal(resp)
			_, _ = w.Write(b)
		},
	))
	t.Cleanup(srv.Close)

	src := newTestSource(t, srv)
	docs, err := src.Discover(context.Background(), time.Time{}, "")
	if err != nil {
		t.Fatalf("Discover after retry: %v", err)
	}
	if len(docs) == 0 {
		t.Fatal("expected docs after retry, got none")
	}
}

func TestDiscoverPagination(t *testing.T) {
	page1 := []spFile{
		{
			Name:              "doc1.pdf",
			ServerRelativeURL: "/lib/doc1.pdf",
			TimeLastModified:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
			UniqueID:          "p1-1",
			ETag:              "\"e1\"",
		},
		{
			Name:              "doc2.pdf",
			ServerRelativeURL: "/lib/doc2.pdf",
			TimeLastModified:  time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC),
			UniqueID:          "p1-2",
			ETag:              "\"e2\"",
		},
	}
	page2 := []spFile{
		{
			Name:              "doc3.pdf",
			ServerRelativeURL: "/lib/doc3.pdf",
			TimeLastModified:  time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC),
			UniqueID:          "p2-1",
			ETag:              "\"e3\"",
		},
	}

	srv := fixtureServer(t, page1, nil, page2)
	src, err := New(
		srv.URL, "/lib",
		corpus.GroupStd, FakeAuth{}, srv.Client(), nil,
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	docs, err := src.Discover(context.Background(), time.Time{}, "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(docs) != 3 {
		t.Fatalf(
			"docs = %d, want 3 (2 from page1 + 1 from page2)",
			len(docs),
		)
	}
}

func TestDiscoverCycleDetection(t *testing.T) {
	selfRef := []spFolder{
		{Name: "loop", ServerRelativeURL: "/sites/controls/Docs"},
	}
	srv := fixtureServer(t, testFiles(), selfRef, nil)
	src := newTestSource(t, srv)

	docs, err := src.Discover(context.Background(), time.Time{}, "")
	if err != nil {
		t.Fatalf("Discover with cycle: %v", err)
	}
	if len(docs) < 1 {
		t.Fatal("expected at least 1 doc")
	}
}

func TestDiscoverIssuedAtFallbackToMtime(t *testing.T) {
	mtime := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	files := []spFile{
		{
			Name:              "no-issued.pdf",
			ServerRelativeURL: "/lib/no-issued.pdf",
			TimeLastModified:  mtime,
			UniqueID:          "no-issued",
			ETag:              "\"x\"",
		},
	}
	srv := fixtureServer(t, files, nil, nil)
	src, err := New(
		srv.URL, "/lib",
		corpus.GroupStd, FakeAuth{}, srv.Client(), nil,
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	docs, err := src.Discover(context.Background(), time.Time{}, "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("docs = %d, want 1", len(docs))
	}
	if docs[0].IssuedAt.IsZero() {
		t.Fatal("IssuedAt is zero, want fallback to TimeLastModified")
	}
	if !docs[0].IssuedAt.Equal(mtime) {
		t.Fatalf("IssuedAt = %v, want %v", docs[0].IssuedAt, mtime)
	}
}

func findDoc(
	docs []ingest.DiscoveredDoc, externalID string,
) *ingest.DiscoveredDoc {
	for i := range docs {
		if docs[i].ExternalID == externalID {
			return &docs[i]
		}
	}
	return &ingest.DiscoveredDoc{}
}
