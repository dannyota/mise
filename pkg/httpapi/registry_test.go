package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"danny.vn/mise/pkg/httpapi"
)

func testRegistryAPI() huma.API {
	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("test", "0.0.1"))
	httpapi.RegisterRegistry(api)
	return api
}

func TestListRegistry(t *testing.T) {
	api := testRegistryAPI()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/registry", nil)
	api.Adapter().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var body struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) < 5 {
		t.Errorf("expected >= 5 corpora, got %d", len(body.Items))
	}
}

func TestGetRegistry_Found(t *testing.T) {
	api := testRegistryAPI()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/registry/vn-reg", nil)
	api.Adapter().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var body httpapi.CorpusDescriptorWire
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ID != "vn-reg" {
		t.Errorf("id = %q, want vn-reg", body.ID)
	}
}

func TestGetRegistry_NotFound(t *testing.T) {
	api := testRegistryAPI()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/registry/nonexistent", nil)
	api.Adapter().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}
