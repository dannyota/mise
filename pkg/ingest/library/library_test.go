package library

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/ingest"
)

func TestNewRejectsUnregisteredCorpus(t *testing.T) {
	if _, err := New(t.TempDir(), corpus.ID("no-such-corpus"), nil); err == nil {
		t.Fatal("expected error for unregistered corpus id, got nil")
	}
}

func TestDiscoverReturnsNewestFirst(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "older.pdf", "old content", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	writeFile(t, root, "newer.pdf", "new content", time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC))

	docs, err := newSource(t, root).Discover(context.Background(), time.Time{}, "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("docs = %d, want 2", len(docs))
	}
	if docs[0].PublishedAt.Before(docs[1].PublishedAt) {
		t.Fatalf("not newest-first: %v then %v", docs[0].PublishedAt, docs[1].PublishedAt)
	}
}

func TestDiscoverWatermarkFilters(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "old.pdf", "old", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	writeFile(t, root, "new.pdf", "new", time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC))

	since := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	docs, err := newSource(t, root).Discover(context.Background(), since, "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("docs = %d, want 1 (only the file after watermark)", len(docs))
	}
	if docs[0].Title != "new" {
		t.Fatalf("title = %q, want new", docs[0].Title)
	}
}

func TestDiscoverWatermarkIsStrictlyAfter(t *testing.T) {
	root := t.TempDir()
	boundary := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	writeFile(t, root, "at-boundary.pdf", "same mtime as since", boundary)

	docs, err := newSource(t, root).Discover(context.Background(), boundary, "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(docs) != 0 {
		t.Fatalf("docs = %d, want 0 (mtime == since must be excluded)", len(docs))
	}
}

func TestDiscoverSkipsSidecarsAndHiddenFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "good.pdf", "content", time.Now())
	writeFile(t, root, "good.pdf.meta.json", `{"title":"sidecar"}`, time.Now())
	writeFile(t, root, ".hidden.pdf", "hidden", time.Now())

	// Hidden directory.
	hiddenDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(hiddenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, hiddenDir, "internal.pdf", "gitfile", time.Now())

	docs, err := newSource(t, root).Discover(context.Background(), time.Time{}, "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("docs = %d, want 1 (only good.pdf)", len(docs))
	}
	if docs[0].Title != "good" {
		t.Fatalf("title = %q", docs[0].Title)
	}
}

func TestDiscoverSkipsUnsupportedExtensions(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "legacy.doc", "old word", time.Now())
	writeFile(t, root, "kept.docx", "new word", time.Now())

	docs, err := newSource(t, root).Discover(context.Background(), time.Time{}, "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(docs) != 1 || docs[0].Title != "kept" {
		t.Fatalf("docs = %+v, want only kept.docx", docs)
	}
}

func TestDiscoverKeywordFilter(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "Information Security Standard.pdf", "a", time.Now())
	writeFile(t, root, "AML Policy.pdf", "b", time.Now())

	docs, err := newSource(t, root).Discover(context.Background(), time.Time{}, "security")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("docs = %d, want 1", len(docs))
	}
	if docs[0].Title != "Information Security Standard" {
		t.Fatalf("title = %q", docs[0].Title)
	}
}

func TestDiscoverKeywordMatchesSidecarNumber(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "doc.pdf", "content", time.Now())
	writeFile(t, root, "doc.pdf.meta.json", `{"number":"GRP-STD-014"}`, time.Now())

	docs, err := newSource(t, root).Discover(context.Background(), time.Time{}, "GRP-STD-014")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("docs = %d, want 1", len(docs))
	}
}

func TestDiscoverAbsentRootReturnsEmpty(t *testing.T) {
	src, err := New("/nonexistent/path/xyz", corpus.GroupStd, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	docs, err := src.Discover(context.Background(), time.Time{}, "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(docs) != 0 {
		t.Fatalf("docs = %d, want 0", len(docs))
	}
}

func TestDiscoverSkipsBrokenSymlink(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "good.pdf", "content", time.Now())
	if err := os.Symlink(filepath.Join(root, "gone.pdf"), filepath.Join(root, "dangling.pdf")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	docs, err := newSource(t, root).Discover(context.Background(), time.Time{}, "")
	if err != nil {
		t.Fatalf("Discover must survive a dangling symlink: %v", err)
	}
	if len(docs) != 1 || docs[0].Title != "good" {
		t.Fatalf("docs = %+v, want only good.pdf", docs)
	}
}

func TestDiscoverSetsContentFingerprint(t *testing.T) {
	root := t.TempDir()
	content := "fingerprint me"
	writeFile(t, root, "doc.pdf", content, time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC))

	src := newSource(t, root)
	docs, err := src.Discover(context.Background(), time.Time{}, "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("docs = %d, want 1", len(docs))
	}
	if want := sha256Hex(content); docs[0].ContentFingerprint != want {
		t.Fatalf("fingerprint = %q, want %q", docs[0].ContentFingerprint, want)
	}

	// An in-place content edit (same path, same title) must change the
	// fingerprint — the pipeline's only change-detection signal for it.
	writeFile(t, root, "doc.pdf", content+" v2", time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC))
	docs, err = src.Discover(context.Background(), time.Time{}, "")
	if err != nil {
		t.Fatalf("Discover after edit: %v", err)
	}
	if want := sha256Hex(content + " v2"); docs[0].ContentFingerprint != want {
		t.Fatalf("fingerprint after edit = %q, want %q", docs[0].ContentFingerprint, want)
	}
}

func TestFetchDetailSidecarMerge(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "policy.pdf", "content", time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC))
	writeFile(t, root, "policy.pdf.meta.json", `{
		"number": "GRP-STD-014",
		"title": "Group Information Security Standard",
		"doc_type": "standard",
		"language": "en",
		"signer_name": "Group CRO",
		"signer_role": "Chief Risk Officer",
		"owner_department": "Information Security",
		"owner_role": "Head of InfoSec",
		"version": "3.2",
		"issued_date": "2026-01-15",
		"effective_date": "2026-02-01",
		"relations": [{"type": "implements", "target_number": "BNM/RMiT"}]
	}`, time.Now())

	ref := ingest.DetailRef{
		ExternalID: "policy.pdf",
		DetailURL:  filepath.Join(root, "policy.pdf"),
	}
	doc, err := newSource(t, root).FetchDetail(context.Background(), ref)
	if err != nil {
		t.Fatalf("FetchDetail: %v", err)
	}
	checks := []struct {
		name string
		got  string
		want string
	}{
		{"Number", doc.Number, "GRP-STD-014"},
		{"Title", doc.Title, "Group Information Security Standard"},
		{"DocType", string(doc.DocType), "standard"},
		{"Language", doc.Language, "en"},
		{"Signer", doc.Signer, "Group CRO"},
		{"SignerRole", doc.SignerRole, "Chief Risk Officer"},
		{"OwnerDepartment", doc.OwnerDepartment, "Information Security"},
		{"OwnerRole", doc.OwnerRole, "Head of InfoSec"},
		{"Version", doc.Version, "3.2"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("doc.%s = %q, want %q", c.name, c.got, c.want)
		}
	}
	if doc.IssuedAt != time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("issued_at = %v", doc.IssuedAt)
	}
	if doc.EffectiveAt != time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("effective_at = %v", doc.EffectiveAt)
	}
	if len(doc.Relations) != 1 {
		t.Fatalf("relations = %d", len(doc.Relations))
	}
	if doc.Relations[0].Type != "implements" || doc.Relations[0].TargetNumber != "BNM/RMiT" {
		t.Fatalf("relation = %+v", doc.Relations[0])
	}
	if len(doc.Files) != 1 || doc.Files[0].Ext != "pdf" {
		t.Fatalf("files = %+v", doc.Files)
	}
	if want := sha256Hex("content"); doc.ContentFingerprint != want {
		t.Fatalf("fingerprint = %q, want %q", doc.ContentFingerprint, want)
	}
}

func TestFetchDetailFallbackTitle(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "My Document.docx", "content", time.Now())

	src, err := New(root, corpus.LocalPolicy, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ref := ingest.DetailRef{
		ExternalID: "My Document.docx",
		DetailURL:  filepath.Join(root, "My Document.docx"),
	}
	doc, err := src.FetchDetail(context.Background(), ref)
	if err != nil {
		t.Fatalf("FetchDetail: %v", err)
	}
	if doc.Title != "My Document" {
		t.Fatalf("title = %q, want fallback from filename", doc.Title)
	}
}

func TestFetchDetailMissingSidecarOK(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "bare.pdf", "content", time.Now())

	ref := ingest.DetailRef{
		ExternalID: "bare.pdf",
		DetailURL:  filepath.Join(root, "bare.pdf"),
	}
	doc, err := newSource(t, root).FetchDetail(context.Background(), ref)
	if err != nil {
		t.Fatalf("FetchDetail: %v", err)
	}
	if doc.Title != "bare" {
		t.Fatalf("title = %q", doc.Title)
	}
}

func TestFetchDetailStrictDecodeRejectsUnknownKeys(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "doc.pdf", "content", time.Now())
	writeFile(t, root, "doc.pdf.meta.json", `{"number":"X","unknown_field":"oops"}`, time.Now())

	ref := ingest.DetailRef{
		ExternalID: "doc.pdf",
		DetailURL:  filepath.Join(root, "doc.pdf"),
	}
	if _, err := newSource(t, root).FetchDetail(context.Background(), ref); err == nil {
		t.Fatal("expected error for unknown sidecar key, got nil")
	}
}

func TestFetchDetailExternalIDOnly(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "sub/nested.pdf", "content", time.Now())

	ref := ingest.DetailRef{ExternalID: "sub/nested.pdf"}
	doc, err := newSource(t, root).FetchDetail(context.Background(), ref)
	if err != nil {
		t.Fatalf("FetchDetail: %v", err)
	}
	if doc.Title != "nested" {
		t.Fatalf("title = %q", doc.Title)
	}
}

func TestDownloadBytesAndSHA(t *testing.T) {
	root := t.TempDir()
	content := []byte("hello world pdf bytes")
	writeFile(t, root, "test.pdf", string(content), time.Now())

	ref := ingest.FileRef{URL: filepath.Join(root, "test.pdf"), Name: "test.pdf", Ext: "pdf"}

	var buf bytes.Buffer
	n, sha, err := newSource(t, root).Download(context.Background(), ref, &buf)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if n != int64(len(content)) {
		t.Fatalf("n = %d, want %d", n, len(content))
	}
	if buf.String() != string(content) {
		t.Fatalf("content mismatch")
	}
	if want := sha256Hex(string(content)); sha != want {
		t.Fatalf("sha = %s, want %s", sha, want)
	}
}

func TestDownloadEmptyPathErrors(t *testing.T) {
	src := newSource(t, t.TempDir())
	if _, _, err := src.Download(context.Background(), ingest.FileRef{}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestSourceID(t *testing.T) {
	if got := newSource(t, t.TempDir()).ID(); got != "library" {
		t.Fatalf("ID = %q, want library", got)
	}
}

// newSource builds a group-std library source over root, failing the test on
// construction errors.
func newSource(t *testing.T, root string) *Source {
	t.Helper()
	src, err := New(root, corpus.GroupStd, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return src
}

// writeFile creates a file with the given content and mtime. Parent dirs are
// created as needed.
func writeFile(t *testing.T, dir, name, content string, mtime time.Time) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

// sha256Hex is the lowercase-hex SHA-256 of s — the digest format the Source
// contract promises for Download and ContentFingerprint.
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
