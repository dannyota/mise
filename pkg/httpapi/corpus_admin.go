package httpapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"danny.vn/mise/pkg/corpus"
)

// CorpusAdminRepoIface is the corpus admin endpoints' dependency — satisfied
// by cmd/serving's wiring (a store-backed adapter for status, and a Temporal
// client adapter for ingest triggers). consumer-defined per CODE_STYLE_GO.
type CorpusAdminRepoIface interface {
	// TriggerIngest starts the IngestCorpusWorkflow for the given corpus and
	// returns the Temporal workflow run ID.
	TriggerIngest(ctx context.Context, corpusID string) (string, error)
	// GetIngestStatus returns the latest ingest run status for a corpus.
	GetIngestStatus(ctx context.Context, corpusID string) (IngestStatusInfo, error)
}

// IngestStatusInfo is the in-memory form GetIngestStatus returns.
type IngestStatusInfo struct {
	CorpusID      string
	Status        string // "healthy" | "ingesting" | "error"
	LastIngest    string // RFC3339 or ""
	WorkflowID    string
	DocumentCount int
	ErrorMessage  string
}

// --- Wire types ---

// CorpusAdminStatusWire is GET /corpora/{id}/status's wire form.
type CorpusAdminStatusWire struct {
	CorpusID      string  `json:"corpus_id"`
	Status        string  `json:"status"`
	LastIngest    string  `json:"last_ingest"`
	WorkflowID    *string `json:"workflow_id"`
	DocumentCount int     `json:"document_count"`
	ErrorMessage  *string `json:"error_message"`
}

// --- Input/Output types ---

// CorpusCreateInput is POST /corpora's input.
type CorpusCreateInput struct {
	Body struct {
		ID   string `json:"id" doc:"Corpus identifier" example:"vn-reg"`
		Kind string `json:"kind" doc:"Corpus kind" example:"regulation"`
	}
}

// CorpusCreateOutput is POST /corpora's output (501 stub).
type CorpusCreateOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

// CorpusIngestInput is POST /corpora/{id}/ingest's input.
type CorpusIngestInput struct {
	ID string `path:"id" doc:"Corpus ID" example:"vn-reg"`
}

// CorpusIngestOutput is POST /corpora/{id}/ingest's output.
type CorpusIngestOutput struct {
	Body struct {
		WorkflowID string `json:"workflow_id"`
	}
}

// CorpusStatusInput is GET /corpora/{id}/status's input.
type CorpusStatusInput struct {
	ID string `path:"id" doc:"Corpus ID" example:"vn-reg"`
}

// CorpusStatusOutput is GET /corpora/{id}/status's output.
type CorpusStatusOutput struct {
	Body CorpusAdminStatusWire
}

// RegisterCorpusAdmin mounts the corpus administration endpoints.
func RegisterCorpusAdmin(api huma.API, repo CorpusAdminRepoIface, role string) {
	huma.Register(api, huma.Operation{
		OperationID: "create-corpus",
		Method:      http.MethodPost,
		Path:        "/corpora",
		Summary:     "Register a new corpus (not yet implemented)",
		Tags:        []string{"Corpus Admin"},
		Errors:      []int{http.StatusNotImplemented},
	}, newCreateCorpusHandler())

	huma.Register(api, huma.Operation{
		OperationID: "trigger-corpus-ingest",
		Method:      http.MethodPost,
		Path:        "/corpora/{id}/ingest",
		Summary:     "Trigger the IngestCorpusWorkflow for a corpus",
		Tags:        []string{"Corpus Admin"},
		Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
	}, newTriggerIngestHandler(repo, role))

	huma.Register(api, huma.Operation{
		OperationID: "get-corpus-status",
		Method:      http.MethodGet,
		Path:        "/corpora/{id}/status",
		Summary:     "Get latest ingest run status for a corpus",
		Tags:        []string{"Corpus Admin"},
		Errors:      []int{http.StatusNotFound},
	}, newGetCorpusStatusHandler(repo, role))
}

func newCreateCorpusHandler() func(context.Context, *CorpusCreateInput) (*CorpusCreateOutput, error) {
	return func(_ context.Context, _ *CorpusCreateInput) (*CorpusCreateOutput, error) {
		// Runtime corpus creation needs design work (schema provisioning,
		// migration orchestration, registry persistence). Return 501 until then.
		return nil, huma.Error501NotImplemented("runtime corpus creation is not yet implemented")
	}
}

func newTriggerIngestHandler(
	repo CorpusAdminRepoIface, _ string,
) func(context.Context, *CorpusIngestInput) (*CorpusIngestOutput, error) {
	return func(ctx context.Context, in *CorpusIngestInput) (*CorpusIngestOutput, error) {
		if _, ok := corpus.Get(corpus.ID(in.ID)); !ok {
			return nil, huma.Error404NotFound(fmt.Sprintf("corpus %q not found", in.ID))
		}

		if repo == nil {
			return nil, huma.Error501NotImplemented("ingest trigger not wired (no Temporal client)")
		}

		wfID, err := repo.TriggerIngest(ctx, in.ID)
		if err != nil {
			return nil, fmt.Errorf("httpapi: triggering ingest for %s: %w", in.ID, err)
		}

		out := &CorpusIngestOutput{}
		out.Body.WorkflowID = wfID
		return out, nil
	}
}

func newGetCorpusStatusHandler(
	repo CorpusAdminRepoIface, _ string,
) func(context.Context, *CorpusStatusInput) (*CorpusStatusOutput, error) {
	return func(ctx context.Context, in *CorpusStatusInput) (*CorpusStatusOutput, error) {
		if _, ok := corpus.Get(corpus.ID(in.ID)); !ok {
			return nil, huma.Error404NotFound(fmt.Sprintf("corpus %q not found", in.ID))
		}

		if repo == nil {
			// Without a repo wired, return a minimal status from the registry.
			out := &CorpusStatusOutput{}
			out.Body = CorpusAdminStatusWire{
				CorpusID: in.ID,
				Status:   "healthy",
			}
			return out, nil
		}

		info, err := repo.GetIngestStatus(ctx, in.ID)
		if err != nil {
			return nil, fmt.Errorf("httpapi: getting corpus status for %s: %w", in.ID, err)
		}

		out := &CorpusStatusOutput{}
		out.Body = ingestStatusToWire(info)
		return out, nil
	}
}

func ingestStatusToWire(info IngestStatusInfo) CorpusAdminStatusWire {
	w := CorpusAdminStatusWire{
		CorpusID:      info.CorpusID,
		Status:        info.Status,
		LastIngest:    info.LastIngest,
		DocumentCount: info.DocumentCount,
	}
	if info.WorkflowID != "" {
		w.WorkflowID = &info.WorkflowID
	}
	if info.ErrorMessage != "" {
		w.ErrorMessage = &info.ErrorMessage
	}
	return w
}
