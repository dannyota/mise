package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func newStubsTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	router := chi.NewRouter()
	api := NewAPI(router)
	RegisterReports(api, nil, "mise_public")
	RegisterTranslate(api)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

func TestCoverageReportReturns501(t *testing.T) {
	t.Parallel()
	srv := newStubsTestServer(t)

	resp, err := http.Post(srv.URL+"/reports/coverage", "application/json", //nolint:noctx
		jsonReader(map[string]any{"corpora": []string{"vn-reg"}}))
	if err != nil {
		t.Fatalf("POST /reports/coverage: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", resp.StatusCode)
	}
}

func TestFindingsExportReturns501(t *testing.T) {
	t.Parallel()
	srv := newStubsTestServer(t)

	resp, err := http.Get(srv.URL + "/reports/findings.xlsx") //nolint:noctx
	if err != nil {
		t.Fatalf("GET /reports/findings.xlsx: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", resp.StatusCode)
	}
}

func TestTranslateReturns501(t *testing.T) {
	t.Parallel()
	srv := newStubsTestServer(t)

	resp, err := http.Post(srv.URL+"/translate", "application/json", //nolint:noctx
		jsonReader(map[string]string{
			"text":        "hello",
			"source_lang": "en",
			"target_lang": "vi",
		}))
	if err != nil {
		t.Fatalf("POST /translate: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", resp.StatusCode)
	}
}

// jsonReader marshals v into a reader for http.Post.
func jsonReader(v any) *bytes.Reader {
	b, _ := json.Marshal(v)
	return bytes.NewReader(b)
}
