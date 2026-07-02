package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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

// TestReadyzUnreachablePoolReturns503 verifies /readyz reports 503 when it
// can't reach AlloyDB — unlike healthzHandler, readiness genuinely depends
// on the DB. The pool targets a closed local port (127.0.0.1:1, reserved,
// nothing listens), so the ping fails fast without needing a real database.
func TestReadyzUnreachablePoolReturns503(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://mise:mise@127.0.0.1:1/mise?sslmode=disable")
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v, want nil (pool construction doesn't connect eagerly)", err)
	}
	defer pool.Close()

	r := chi.NewRouter()
	r.Get("/readyz", readyzHandler(pool))

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}
