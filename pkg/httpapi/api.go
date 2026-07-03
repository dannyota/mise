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

// GenerateSpec builds the same huma.API NewAPI + Register wire up in
// cmd/serving and renders its OpenAPI 3.1 document as YAML. It opens no
// pool/DB: Register's repo argument is only ever invoked per HTTP request,
// and OpenAPI generation only inspects the Input/Output Go types via
// reflection (huma.Register) — so a nil GraphRepoIface is safe here, and
// role, while threaded through for parity with production's call shape, is
// likewise never inspected at generation time. cmd/openapi-gen/main.go and
// the anti-drift test (openapi_drift_test.go) both call this, so the
// committed api/openapi.yaml and the test's expectation can never be built
// two different ways.
func GenerateSpec(role string) ([]byte, error) {
	router := chi.NewRouter()
	api := NewAPI(router)
	Register(api, nil, nil, nil, role)

	spec, err := api.OpenAPI().YAML()
	if err != nil {
		return nil, fmt.Errorf("httpapi: rendering openapi yaml: %w", err)
	}
	return spec, nil
}
