package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestHealthz verifies the /healthz route reports liveness without requiring
// a running server or any dependency (DB, MCP) to be up.
func TestHealthz(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/healthz", healthzHandler)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got, want := w.Body.String(), "ok"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}
