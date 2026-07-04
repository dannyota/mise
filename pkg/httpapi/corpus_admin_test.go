package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

type fakeCorpusAdminRepo struct {
	triggerID  string
	triggerErr error
	status     IngestStatusInfo
	statusErr  error
	gotCorpus  string
}

func (f *fakeCorpusAdminRepo) TriggerIngest(_ context.Context, corpusID string) (string, error) {
	f.gotCorpus = corpusID
	return f.triggerID, f.triggerErr
}

func (f *fakeCorpusAdminRepo) GetIngestStatus(_ context.Context, corpusID string) (IngestStatusInfo, error) {
	f.gotCorpus = corpusID
	return f.status, f.statusErr
}

func newCorpusAdminTestServer(t *testing.T, repo CorpusAdminRepoIface, role string) *httptest.Server {
	t.Helper()
	router := chi.NewRouter()
	api := NewAPI(router)
	RegisterCorpusAdmin(api, repo, role)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

func TestCreateCorpusReturns501(t *testing.T) {
	t.Parallel()
	srv := newCorpusAdminTestServer(t, &fakeCorpusAdminRepo{}, "mise_public")

	resp, err := http.Post(srv.URL+"/corpora", "application/json", //nolint:noctx
		jsonReader(map[string]string{"id": "test", "kind": "regulation"}))
	if err != nil {
		t.Fatalf("POST /corpora: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", resp.StatusCode)
	}
}

func TestTriggerIngestReturnsWorkflowID(t *testing.T) {
	t.Parallel()
	repo := &fakeCorpusAdminRepo{triggerID: "wf-run-123"}
	srv := newCorpusAdminTestServer(t, repo, "mise_public")

	resp, err := http.Post(srv.URL+"/corpora/vn-reg/ingest", "application/json", nil) //nolint:noctx
	if err != nil {
		t.Fatalf("POST /corpora/vn-reg/ingest: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got struct {
		WorkflowID string `json:"workflow_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if got.WorkflowID != "wf-run-123" {
		t.Errorf("WorkflowID = %q, want wf-run-123", got.WorkflowID)
	}
	if repo.gotCorpus != "vn-reg" {
		t.Errorf("repo received corpus = %q, want vn-reg", repo.gotCorpus)
	}
}

func TestTriggerIngestBadCorpusReturns404(t *testing.T) {
	t.Parallel()
	srv := newCorpusAdminTestServer(t, &fakeCorpusAdminRepo{}, "mise_public")

	resp, err := http.Post(srv.URL+"/corpora/nonexistent/ingest", "application/json", nil) //nolint:noctx
	if err != nil {
		t.Fatalf("POST /corpora: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestTriggerIngestRepoErrorReturns500(t *testing.T) {
	t.Parallel()
	repo := &fakeCorpusAdminRepo{triggerErr: errors.New("temporal down")}
	srv := newCorpusAdminTestServer(t, repo, "mise_public")

	resp, err := http.Post(srv.URL+"/corpora/vn-reg/ingest", "application/json", nil) //nolint:noctx
	if err != nil {
		t.Fatalf("POST /corpora: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
}

func TestGetCorpusStatusReturnsInfo(t *testing.T) {
	t.Parallel()
	repo := &fakeCorpusAdminRepo{
		status: IngestStatusInfo{
			CorpusID:      "vn-reg",
			Status:        "healthy",
			LastIngest:    "2026-07-01T08:00:00Z",
			DocumentCount: 42,
		},
	}
	srv := newCorpusAdminTestServer(t, repo, "mise_public")

	resp, err := http.Get(srv.URL + "/corpora/vn-reg/status") //nolint:noctx
	if err != nil {
		t.Fatalf("GET /corpora/vn-reg/status: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got CorpusAdminStatusWire
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if got.CorpusID != "vn-reg" {
		t.Errorf("CorpusID = %q, want vn-reg", got.CorpusID)
	}
	if got.Status != "healthy" {
		t.Errorf("Status = %q, want healthy", got.Status)
	}
	if got.DocumentCount != 42 {
		t.Errorf("DocumentCount = %d, want 42", got.DocumentCount)
	}
}

func TestGetCorpusStatusBadCorpusReturns404(t *testing.T) {
	t.Parallel()
	srv := newCorpusAdminTestServer(t, &fakeCorpusAdminRepo{}, "mise_public")

	resp, err := http.Get(srv.URL + "/corpora/nonexistent/status") //nolint:noctx
	if err != nil {
		t.Fatalf("GET /corpora: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}
