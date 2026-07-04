package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"danny.vn/mise/pkg/store"
)

type fakeSearchRepo struct {
	hits    []store.Hit
	err     error
	gotQ    string
	gotOpts store.SearchOpts
}

func (f *fakeSearchRepo) Search(_ context.Context, query string, opts store.SearchOpts) ([]store.Hit, error) {
	f.gotQ = query
	f.gotOpts = opts
	return f.hits, f.err
}

func newSearchTestServer(t *testing.T, repo SearchRepoIface, role string) *httptest.Server {
	t.Helper()
	router := chi.NewRouter()
	api := NewAPI(router)
	RegisterSearch(api, repo, role)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

func TestSearchReturnsHits(t *testing.T) {
	t.Parallel()
	docID := uuid.New()
	secID := uuid.New()

	repo := &fakeSearchRepo{
		hits: []store.Hit{{
			CorpusID:       "vn-reg",
			DocumentID:     docID,
			SectionID:      secID,
			DocNumber:      "01/2024/TT-NHNN",
			Title:          "IT Safety",
			CitationPath:   "Điều 5, Khoản 1",
			HeadingPath:    "Chapter II > Article 5",
			Text:           "Banks must implement...",
			ValidityStatus: "in_force",
			Score:          0.85,
			SourceURL:      "https://vbpl.vn/example",
		}},
	}

	srv := newSearchTestServer(t, repo, "mise_public")
	resp, err := http.Get(srv.URL + "/search?q=IT+safety&top_k=5") //nolint:noctx
	if err != nil {
		t.Fatalf("GET /search: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var got struct {
		Sections []SectionHitWire `json:"sections"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(got.Sections) != 1 {
		t.Fatalf("Sections = %d, want 1", len(got.Sections))
	}
	if got.Sections[0].CorpusID != "vn-reg" {
		t.Errorf("CorpusID = %q, want vn-reg", got.Sections[0].CorpusID)
	}
	if got.Sections[0].DocumentID != docID.String() {
		t.Errorf("DocumentID = %q, want %s", got.Sections[0].DocumentID, docID)
	}
	if got.Sections[0].Score != 0.85 {
		t.Errorf("Score = %f, want 0.85", got.Sections[0].Score)
	}
	if repo.gotQ != "IT safety" {
		t.Errorf("repo received q = %q, want IT safety", repo.gotQ)
	}
	if repo.gotOpts.TopK != 5 {
		t.Errorf("repo received top_k = %d, want 5", repo.gotOpts.TopK)
	}
}

func TestSearchRequiresQuery(t *testing.T) {
	t.Parallel()
	srv := newSearchTestServer(t, &fakeSearchRepo{}, "mise_public")
	resp, err := http.Get(srv.URL + "/search") //nolint:noctx
	if err != nil {
		t.Fatalf("GET /search: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestSearchRejectsInvalidCorpus(t *testing.T) {
	t.Parallel()
	srv := newSearchTestServer(t, &fakeSearchRepo{}, "mise_public")
	resp, err := http.Get(srv.URL + "/search?q=test&corpora=nonexistent") //nolint:noctx
	if err != nil {
		t.Fatalf("GET /search: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestSearchEmptyReturnsNonNilSlice(t *testing.T) {
	t.Parallel()
	repo := &fakeSearchRepo{hits: nil}
	srv := newSearchTestServer(t, repo, "mise_public")
	resp, err := http.Get(srv.URL + "/search?q=nothing") //nolint:noctx
	if err != nil {
		t.Fatalf("GET /search: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got struct {
		Sections []SectionHitWire `json:"sections"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if got.Sections == nil {
		t.Error("Sections = nil, want non-nil empty slice")
	}
}
