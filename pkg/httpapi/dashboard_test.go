package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"danny.vn/mise/pkg/store"
)

type fakeDashboardRepo struct {
	stats   store.DashboardStats
	statErr error
	gotRole string
}

func (f *fakeDashboardRepo) GetStats(_ context.Context, role string) (store.DashboardStats, error) {
	f.gotRole = role
	return f.stats, f.statErr
}

func newDashboardTestServer(t *testing.T, repo DashboardRepoIface, role string) *httptest.Server {
	t.Helper()
	router := chi.NewRouter()
	api := NewAPI(router)
	RegisterDashboard(api, repo, role)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

func TestDashboardSummaryReturnsStats(t *testing.T) {
	t.Parallel()
	repo := &fakeDashboardRepo{
		stats: store.DashboardStats{
			CoveragePct:      42.5,
			OpenConflicts:    3,
			StalenessAlerts:  2,
			ReviewQueueDepth: 15,
			Corpora: []store.CorpusStats{
				{CorpusID: "vn-reg", DocumentCount: 120, LastIngest: "2026-07-01T10:00:00Z", Status: "healthy"},
				{CorpusID: "my-reg", DocumentCount: 80, LastIngest: "", Status: "ingesting"},
			},
		},
	}

	srv := newDashboardTestServer(t, repo, "mise_group")
	status, ct, body := getJSON(t, srv, "/dashboards/summary")

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, body)
	}
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if repo.gotRole != "mise_group" {
		t.Errorf("repo received role = %q, want mise_group", repo.gotRole)
	}

	var got struct {
		CoveragePct      float64            `json:"coverage_pct"`
		OpenConflicts    int                `json:"open_conflicts"`
		StalenessAlerts  int                `json:"staleness_alerts"`
		ReviewQueueDepth int                `json:"review_queue_depth"`
		Corpora          []CorpusStatusWire `json:"corpora"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshaling: %v; body: %s", err, body)
	}
	if got.OpenConflicts != 3 {
		t.Errorf("open_conflicts = %d, want 3", got.OpenConflicts)
	}
	if got.StalenessAlerts != 2 {
		t.Errorf("staleness_alerts = %d, want 2", got.StalenessAlerts)
	}
	if got.ReviewQueueDepth != 15 {
		t.Errorf("review_queue_depth = %d, want 15", got.ReviewQueueDepth)
	}
	if got.CoveragePct != 42.5 {
		t.Errorf("coverage_pct = %v, want 42.5", got.CoveragePct)
	}
	if got.Corpora == nil {
		t.Error("corpora = nil, want non-nil slice")
	}
	if len(got.Corpora) == 0 {
		t.Error("corpora is empty, want at least one corpus from registry")
	}

	// Verify per-corpus fields are wired from the store.
	found := false
	for _, c := range got.Corpora {
		if c.CorpusID == "vn-reg" {
			found = true
			if c.DocumentCount != 120 {
				t.Errorf("vn-reg document_count = %d, want 120", c.DocumentCount)
			}
			if c.LastIngest != "2026-07-01T10:00:00Z" {
				t.Errorf("vn-reg last_ingest = %q, want 2026-07-01T10:00:00Z", c.LastIngest)
			}
			if c.Status != "healthy" {
				t.Errorf("vn-reg status = %q, want healthy", c.Status)
			}
		}
	}
	if !found {
		t.Error("vn-reg corpus not found in response corpora")
	}
}

func TestDashboardSummaryCorporaNonNil(t *testing.T) {
	t.Parallel()
	repo := &fakeDashboardRepo{}
	srv := newDashboardTestServer(t, repo, "mise_public")

	_, _, body := getJSON(t, srv, "/dashboards/summary")

	var got struct {
		Corpora []CorpusStatusWire `json:"corpora"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshaling: %v; body: %s", err, body)
	}
	// Must marshal to an array, never null.
	data, _ := json.Marshal(got.Corpora)
	if string(data) == "null" {
		t.Error("corpora marshaled to null, want []")
	}
}

func TestDashboardSummaryNilRepo(t *testing.T) {
	t.Parallel()
	srv := newDashboardTestServer(t, nil, "mise_public")
	status, _, body := getJSON(t, srv, "/dashboards/summary")

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, body)
	}

	var got struct {
		CoveragePct float64            `json:"coverage_pct"`
		Corpora     []CorpusStatusWire `json:"corpora"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshaling: %v; body: %s", err, body)
	}
	if got.CoveragePct != 0 {
		t.Errorf("coverage_pct = %v, want 0 when repo is nil", got.CoveragePct)
	}
	if len(got.Corpora) == 0 {
		t.Error("corpora should still be populated from registry when repo is nil")
	}
	for _, c := range got.Corpora {
		if c.Status != "healthy" {
			t.Errorf("corpus %q status = %q, want healthy (nil repo fallback)", c.CorpusID, c.Status)
		}
	}
}
