package httpapi

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/store"
)

// DashboardRepoIface is the dashboard endpoint's dependency — satisfied by
// *store.DashboardStore — narrowed to the exact methods the handler needs.
type DashboardRepoIface interface {
	GetStats(ctx context.Context, role string) (store.DashboardStats, error)
}

// CorpusStatusWire is the wire form of a corpus's operational status in the
// dashboard summary.
type CorpusStatusWire struct {
	CorpusID      string `json:"corpus_id"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	LastIngest    string `json:"last_ingest"`
	DocumentCount int    `json:"document_count"`
}

// DashboardSummaryOutput is GET /dashboards/summary's output.
type DashboardSummaryOutput struct {
	Body struct {
		CoveragePct      float64            `json:"coverage_pct"`
		OpenConflicts    int                `json:"open_conflicts"`
		StalenessAlerts  int                `json:"staleness_alerts"`
		ReviewQueueDepth int                `json:"review_queue_depth"`
		Corpora          []CorpusStatusWire `json:"corpora"`
	}
}

// RegisterDashboard mounts the dashboard summary endpoint.
func RegisterDashboard(api huma.API, repo DashboardRepoIface, role string) {
	huma.Register(api, huma.Operation{
		OperationID: "get-dashboard-summary",
		Method:      http.MethodGet,
		Path:        "/dashboards/summary",
		Summary:     "Get the dashboard summary (counts + corpus health)",
		Tags:        []string{"Dashboard"},
	}, newDashboardSummaryHandler(repo, role))
}

func newDashboardSummaryHandler(
	repo DashboardRepoIface, role string,
) func(context.Context, *struct{}) (*DashboardSummaryOutput, error) {
	return func(ctx context.Context, _ *struct{}) (*DashboardSummaryOutput, error) {
		var stats store.DashboardStats
		if repo != nil {
			var err error
			stats, err = repo.GetStats(ctx, role)
			if err != nil {
				return nil, err
			}
		}

		out := &DashboardSummaryOutput{}
		out.Body.CoveragePct = stats.CoveragePct
		out.Body.OpenConflicts = stats.OpenConflicts
		out.Body.StalenessAlerts = stats.StalenessAlerts
		out.Body.ReviewQueueDepth = stats.ReviewQueueDepth
		out.Body.Corpora = buildCorporaStatus(stats.Corpora)
		return out, nil
	}
}

// buildCorporaStatus merges the static corpus registry with live per-corpus
// stats from the store. When storeCorpora is nil (repo was nil), it falls back
// to healthy defaults with zero counts.
func buildCorporaStatus(storeCorpora []store.CorpusStats) []CorpusStatusWire {
	all := corpus.All()

	// Index store stats by corpus ID for O(1) lookup.
	byID := make(map[string]store.CorpusStats, len(storeCorpora))
	for _, cs := range storeCorpora {
		byID[cs.CorpusID] = cs
	}

	out := make([]CorpusStatusWire, len(all))
	for i, d := range all {
		cs, ok := byID[string(d.ID)]
		if ok {
			out[i] = CorpusStatusWire{
				CorpusID:      string(d.ID),
				Name:          string(d.ID),
				Status:        cs.Status,
				LastIngest:    cs.LastIngest,
				DocumentCount: cs.DocumentCount,
			}
		} else {
			out[i] = CorpusStatusWire{
				CorpusID:      string(d.ID),
				Name:          string(d.ID),
				Status:        "healthy",
				LastIngest:    "",
				DocumentCount: 0,
			}
		}
	}
	return out
}
