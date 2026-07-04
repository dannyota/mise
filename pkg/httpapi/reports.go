package httpapi

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// ReportsRepoIface is the reports endpoints' dependency — for POST
// /reports/coverage (compute from graph edges + findings) and GET
// /reports/findings.xlsx (Excel export). Neither is implemented yet.
type ReportsRepoIface interface {
	// GenerateCoverage computes the coverage report from the graph.
	GenerateCoverage(ctx context.Context, role string) (CoverageReportWire, error)
}

// --- Wire types ---

// CoverageReportWire is POST /reports/coverage's response body.
type CoverageReportWire struct {
	CoveragePct float64             `json:"coverage_pct"`
	Gaps        []CoverageGapWire   `json:"gaps"`
	Chains      []CoverageChainWire `json:"chains"`
	GeneratedAt string              `json:"generated_at"`
}

// CoverageGapWire is one gap in the coverage report.
type CoverageGapWire struct {
	CorpusID   string `json:"corpus_id"`
	DocumentID string `json:"document_id"`
	Citation   string `json:"citation"`
	GapType    string `json:"gap_type"`
}

// CoverageChainWire is one complete chain in the coverage report.
type CoverageChainWire struct {
	LawRef    string `json:"law_ref"`
	PolicyRef string `json:"policy_ref"`
	SOPRef    string `json:"sop_ref"`
}

// --- Input/Output types ---

// CoverageReportInput is POST /reports/coverage's input.
type CoverageReportInput struct {
	Body struct {
		Corpora []string `json:"corpora,omitempty" doc:"Corpus IDs to include (empty = all)"`
	}
}

// CoverageReportOutput is POST /reports/coverage's output (501 stub).
type CoverageReportOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

// FindingsExportInput is GET /reports/findings.xlsx's input.
type FindingsExportInput struct {
	Kind   string `query:"kind" doc:"Filter by finding kind" example:""`
	Status string `query:"status" doc:"Filter by finding status" example:""`
}

// FindingsExportOutput is GET /reports/findings.xlsx's output (501 stub).
type FindingsExportOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

// RegisterReports mounts the report generation endpoints.
func RegisterReports(api huma.API, _ ReportsRepoIface, _ string) {
	huma.Register(api, huma.Operation{
		OperationID: "generate-coverage-report",
		Method:      http.MethodPost,
		Path:        "/reports/coverage",
		Summary:     "Generate coverage report from graph edges and findings",
		Tags:        []string{"Reports"},
		Errors:      []int{http.StatusNotImplemented},
	}, newCoverageReportHandler())

	huma.Register(api, huma.Operation{
		OperationID: "export-findings-xlsx",
		Method:      http.MethodGet,
		Path:        "/reports/findings.xlsx",
		Summary:     "Export findings register as Excel spreadsheet",
		Tags:        []string{"Reports"},
		Errors:      []int{http.StatusNotImplemented},
	}, newFindingsExportHandler())
}

func newCoverageReportHandler() func(context.Context, *CoverageReportInput) (*CoverageReportOutput, error) {
	return func(_ context.Context, _ *CoverageReportInput) (*CoverageReportOutput, error) {
		// Coverage report computation from graph edges + findings needs the
		// graph traversal + gap detection logic. Return 501 until implemented.
		return nil, huma.Error501NotImplemented("coverage report generation is not yet implemented")
	}
}

func newFindingsExportHandler() func(context.Context, *FindingsExportInput) (*FindingsExportOutput, error) {
	return func(_ context.Context, _ *FindingsExportInput) (*FindingsExportOutput, error) {
		// Excel generation needs an xlsx library (e.g. excelize). Return 501.
		return nil, huma.Error501NotImplemented("findings Excel export is not yet implemented")
	}
}
