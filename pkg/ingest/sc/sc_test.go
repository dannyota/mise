package sc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"danny.vn/mise/pkg/ingest"
)

func TestDocAnchorParsing(t *testing.T) {
	html := `<ul>
<li><a href="https://www.sc.com.my/api/documentms/download.ashx?id=2f253636-07dd-4355-b89e-010b2ef581c1">
Guidelines on Technology Risk Management (pdf)</a></li>
<li><a href="/api/documentms/download.ashx?id=985D39B2-D548-4E57-AE55-B141159FD20A">
Summary of Amendments&nbsp;(PDF)</a></li>
</ul>`
	matches := docAnchorRe.FindAllStringSubmatch(html, -1)
	if len(matches) != 2 {
		t.Fatalf("anchors = %d, want 2", len(matches))
	}
	if strings.ToLower(matches[0][1]) != "2f253636-07dd-4355-b89e-010b2ef581c1" {
		t.Fatalf("guid0 = %q", matches[0][1])
	}
	if got := cleanTitle(matches[0][2]); got != "Guidelines on Technology Risk Management" {
		t.Fatalf("title0 = %q", got)
	}
	if got := cleanTitle(matches[1][2]); got != "Summary of Amendments" {
		t.Fatalf("title1 = %q (nbsp/(PDF) not stripped)", got)
	}
}

func TestFileFor(t *testing.T) {
	f := fileFor("https://www.sc.com.my", "abc-123", "Guidelines on Cyber Risk")
	if f.URL != "https://www.sc.com.my/api/documentms/download.ashx?id=abc-123" {
		t.Fatalf("url = %q", f.URL)
	}
	if f.Ext != "pdf" || f.Kind != "main" || f.Name != "Guidelines on Cyber Risk.pdf" {
		t.Fatalf("file = %+v", f)
	}
}

// sectionPageHTML stubs an SC section-listing page linking two distinct
// documents — the exact shape that, before the CRITICAL identity fix,
// produced two DiscoveredDocs sharing one DetailURL (the section page) and no
// Number at all, which store.UpsertDocument's doc_number/source_url natural
// key (migration 006's partial unique indexes) collapsed into a single row.
const sectionPageHTML = `<ul>
<li><a href="/api/documentms/download.ashx?id=2f253636-07dd-4355-b89e-010b2ef581c1">
Guidelines on Technology Risk Management (pdf)</a></li>
<li><a href="/api/documentms/download.ashx?id=985d39b2-d548-4e57-ae55-b141159fd20a">
Guidelines on Cyber Risk (pdf)</a></li>
</ul>`

// TestDiscoverAssignsDistinctDocumentIdentity is the regression test for
// CRITICAL B: two documents linked from the SAME section page must come back
// with distinct Number and distinct DetailURL, or a re-index silently
// collapses the whole section's corpus onto one document row.
func TestDiscoverAssignsDistinctDocumentIdentity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(sectionPageHTML))
	}))
	defer srv.Close()

	s := New(srv.Client(), nil)
	s.baseURL = srv.URL

	docs, err := s.Discover(context.Background(), time.Time{}, "")
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	// Every inScopeSections path serves the identical stub page above; the
	// guid-keyed seen-map dedups the repeats across sections, so exactly the
	// 2 distinct documents come back regardless of len(inScopeSections).
	if len(docs) != 2 {
		t.Fatalf("Discover() returned %d docs, want 2: %+v", len(docs), docs)
	}

	d1, d2 := docs[0], docs[1]
	if d1.Number == "" || d2.Number == "" {
		t.Fatalf("Number empty: d1=%q d2=%q (empty Number + a shared source_url is the collapse bug)",
			d1.Number, d2.Number)
	}
	if d1.Number == d2.Number {
		t.Errorf("Number = %q for both documents, want distinct", d1.Number)
	}
	if d1.DetailURL == d2.DetailURL {
		t.Errorf("DetailURL = %q for both documents, want distinct per-document urls, not the shared section page",
			d1.DetailURL)
	}
	for _, d := range docs {
		if want := "SC/" + d.ExternalID; d.Number != want {
			t.Errorf("Number = %q, want %q", d.Number, want)
		}
		if want := srv.URL + "/api/documentms/download.ashx?id=" + d.ExternalID; d.DetailURL != want {
			t.Errorf("DetailURL = %q, want the per-document download url %q", d.DetailURL, want)
		}
	}
}

// TestFetchDetailSetsDistinctNumber proves FetchDetail — not just Discover —
// sets Number: the pipeline indexes FetchDetail's returned DiscoveredDoc, not
// Discover's (pkg/pipeline.processStages), so Number set only in discover.go
// would never reach the stored document.
func TestFetchDetailSetsDistinctNumber(t *testing.T) {
	s := New(nil, nil)
	const guid = "2f253636-07dd-4355-b89e-010b2ef581c1"

	got, err := s.FetchDetail(context.Background(), ingest.DetailRef{
		ExternalID: guid,
		DetailURL:  "https://www.sc.com.my/api/documentms/download.ashx?id=" + guid,
	})
	if err != nil {
		t.Fatalf("FetchDetail() error = %v", err)
	}
	if want := "SC/" + guid; got.Number != want {
		t.Errorf("Number = %q, want %q", got.Number, want)
	}
}
