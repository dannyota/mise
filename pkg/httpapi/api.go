// Package httpapi is mise's REST surface: huma v2 operations mounted at
// /api/v1 (cmd/serving/main.go's newRouter), generating OpenAPI 3.1 from the
// exact same Go Input/Output types the handlers use
// (cmd/openapi-gen/main.go) — the provider side of API-CONTRACT.md §5's
// "one schema, generated from Go, never hand-mirrored" decision. RFC 9457
// application/problem+json errors and JSON Schema 2020-12 come from huma
// itself (huma.DefaultConfig). See docs/engineering/CODE_STYLE_GO.md for
// this package's Input/Output struct convention.
package httpapi

import (
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// name and version identify this API in its generated OpenAPI Info object —
// mirrors pkg/mcp's name/version (server.go).
const (
	name    = "mise"
	version = "0.1.0"
)

// NewAPI wraps r with huma's default config (OpenAPI 3.1, JSON Schema
// 2020-12, RFC 9457 problem+json errors — huma.DefaultConfig). It is the one
// place cmd/serving (mounted under /api/v1) and GenerateSpec (the anti-drift
// gate's generator) both build the huma.API from, so the two can never
// diverge on config alone.
func NewAPI(r chi.Router) huma.API {
	return humachi.New(r, huma.DefaultConfig(name, version))
}

// Deps carries every repository the REST surface reads through, so
// RegisterAll is the single place the endpoint set is enumerated. All fields
// may be nil when the api only exists to generate the OpenAPI spec
// (GenerateSpec): repos are only ever invoked per HTTP request, and spec
// generation only inspects the Input/Output Go types via reflection.
type Deps struct {
	Graph         GraphRepoIface
	Reviews       ReviewRepoIface
	Findings      FindingRepoIface
	Dashboard     DashboardRepoIface
	GraphCanvas   GraphCanvasRepoIface
	Timeline      TimelineRepoIface
	Notifications NotificationRepoIface
	Search        SearchRepoIface
	Documents     DocumentRepoIface
	CorpusAdmin   CorpusAdminRepoIface
	Reports       ReportsRepoIface
}

// RegisterAll mounts the complete /api/v1 operation set onto api. It is the
// ONLY registration entry point — cmd/serving's newRouter and GenerateSpec
// both call it, so the served router and the generated contract can never
// enumerate two different endpoint sets. (Exactly that drift shipped once:
// M10's endpoints were added to spec generation but not to serving, so the
// contract advertised routes that 404'd at runtime.) Every read runs under
// role — the server-resolved RLS role (pkg/config.Role()), never derived
// from request input.
func RegisterAll(api huma.API, d Deps, role string) {
	Register(api, d.Graph, d.Reviews, d.Findings, role)
	RegisterRegistry(api)
	RegisterDashboard(api, d.Dashboard, role)
	RegisterGraphCanvas(api, d.GraphCanvas, role)
	RegisterTimeline(api, d.Timeline, role)
	RegisterNotifications(api, d.Notifications, role)
	RegisterSearch(api, d.Search, role)
	RegisterDocument(api, d.Documents, role)
	RegisterCorpusAdmin(api, d.CorpusAdmin, role)
	RegisterReports(api, d.Reports, role)
	RegisterTranslate(api)
}

// GenerateSpec builds the same huma.API cmd/serving wires up (NewAPI +
// RegisterAll, nil deps) and renders its OpenAPI 3.1 document as YAML.
// cmd/openapi-gen/main.go and the anti-drift test (openapi_drift_test.go)
// both call this, so the committed api/openapi.yaml and the test's
// expectation can never be built two different ways.
func GenerateSpec(role string) ([]byte, error) {
	router := chi.NewRouter()
	api := NewAPI(router)
	RegisterAll(api, Deps{}, role)

	spec, err := api.OpenAPI().YAML()
	if err != nil {
		return nil, fmt.Errorf("httpapi: rendering openapi yaml: %w", err)
	}
	return spec, nil
}
