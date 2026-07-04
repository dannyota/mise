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
			OpenConflicts:    3,
			StalenessAlerts:  2,
			ReviewQueueDepth: 15,
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
	if got.CoveragePct != 0.0 {
		t.Errorf("coverage_pct = %v, want 0.0", got.CoveragePct)
	}
	if got.Corpora == nil {
		t.Error("corpora = nil, want non-nil slice")
	}
	if len(got.Corpora) == 0 {
		t.Error("corpora is empty, want at least one corpus from registry")
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
