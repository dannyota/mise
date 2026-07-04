package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"danny.vn/mise/pkg/store"
)

type fakeDocumentRepo struct {
	detail  store.DocumentDetail
	err     error
	gotRole string
	gotCorp string
	gotID   uuid.UUID
}

func (f *fakeDocumentRepo) GetDocument(
	_ context.Context, role, corpusID string, docID uuid.UUID,
) (store.DocumentDetail, error) {
	f.gotRole, f.gotCorp, f.gotID = role, corpusID, docID
	return f.detail, f.err
}

func newDocumentTestServer(t *testing.T, repo DocumentRepoIface, role string) *httptest.Server {
	t.Helper()
	router := chi.NewRouter()
	api := NewAPI(router)
	RegisterDocument(api, repo, role)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

func TestGetDocumentReturnsDetail(t *testing.T) {
	t.Parallel()
	docID := uuid.New()
	secID := uuid.New()
	effDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	repo := &fakeDocumentRepo{
		detail: store.DocumentDetail{
			Doc: store.Document{
				ID:             docID,
				CorpusID:       "vn-reg",
				Title:          "Circular 09/2024/TT-NHNN",
				ValidityStatus: "in_force",
				Language:       "vi",
				ObservedAt:     effDate,
			},
			Sections: []store.Section{{
				ID:             secID,
				DocumentID:     docID,
				CorpusID:       "vn-reg",
				CitationPath:   "Điều 5",
				Body:           "Text content",
				ValidityStatus: "in_force",
				Position:       0,
			}},
			Events: []store.AmendmentEvent{{
				TargetDocID: docID,
				Kind:        "amended",
				Clause:      "Khoản 2",
				EventDate:   effDate,
			}},
		},
	}

	srv := newDocumentTestServer(t, repo, "mise_group")
	resp, err := http.Get(srv.URL + "/documents/vn-reg/" + docID.String()) //nolint:noctx
	if err != nil {
		t.Fatalf("GET /documents: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var got DocumentDetailWire
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if got.Document.ID != docID.String() {
		t.Errorf("Document.ID = %q, want %s", got.Document.ID, docID)
	}
	if got.Document.Title != "Circular 09/2024/TT-NHNN" {
		t.Errorf("Document.Title = %q", got.Document.Title)
	}
	if len(got.Sections) != 1 {
		t.Fatalf("Sections = %d, want 1", len(got.Sections))
	}
	if got.Sections[0].ID != secID.String() {
		t.Errorf("Sections[0].ID = %q, want %s", got.Sections[0].ID, secID)
	}
	if len(got.Amendments) != 1 {
		t.Fatalf("Amendments = %d, want 1", len(got.Amendments))
	}
	if got.Amendments[0].Kind != "amended" {
		t.Errorf("Amendments[0].Kind = %q, want amended", got.Amendments[0].Kind)
	}

	// Verify the repo received the right args.
	if repo.gotRole != "mise_group" {
		t.Errorf("repo received role = %q, want mise_group", repo.gotRole)
	}
	if repo.gotCorp != "vn-reg" {
		t.Errorf("repo received corpus = %q, want vn-reg", repo.gotCorp)
	}
	if repo.gotID != docID {
		t.Errorf("repo received id = %v, want %v", repo.gotID, docID)
	}
}

func TestGetDocumentNotFoundReturns404(t *testing.T) {
	t.Parallel()
	repo := &fakeDocumentRepo{err: store.ErrDocumentNotFound}
	srv := newDocumentTestServer(t, repo, "mise_public")

	resp, err := http.Get(srv.URL + "/documents/vn-reg/" + uuid.NewString()) //nolint:noctx
	if err != nil {
		t.Fatalf("GET /documents: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestGetDocumentBadCorpusReturns404(t *testing.T) {
	t.Parallel()
	srv := newDocumentTestServer(t, &fakeDocumentRepo{}, "mise_public")

	resp, err := http.Get(srv.URL + "/documents/nonexistent/" + uuid.NewString()) //nolint:noctx
	if err != nil {
		t.Fatalf("GET /documents: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestGetDocumentBadUUIDReturns400(t *testing.T) {
	t.Parallel()
	srv := newDocumentTestServer(t, &fakeDocumentRepo{}, "mise_public")

	resp, err := http.Get(srv.URL + "/documents/vn-reg/not-a-uuid") //nolint:noctx
	if err != nil {
		t.Fatalf("GET /documents: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
