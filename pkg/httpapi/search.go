package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/store"
)

// SearchRepoIface is the search endpoint's dependency — satisfied by
// cmd/serving's storeSearcher adapter — narrowed to the single Search method
// the handler needs, consumer-defined per CODE_STYLE_GO.
type SearchRepoIface interface {
	Search(ctx context.Context, query string, opts store.SearchOpts) ([]store.Hit, error)
}

// --- Wire types ---

// SectionHitWire is the wire form of store.Hit, matching the MCP search tool's
// SectionHit shape (pkg/mcp/tools.go) so the REST mirror returns the exact
// same JSON structure.
type SectionHitWire struct {
	CorpusID       string  `json:"corpus_id"`
	DocumentID     string  `json:"document_id"`
	SectionID      string  `json:"section_id"`
	DocNumber      string  `json:"doc_number"`
	Title          string  `json:"title"`
	CitationPath   string  `json:"citation_path"`
	HeadingPath    string  `json:"heading_path"`
	Text           string  `json:"text"`
	ValidityStatus string  `json:"validity_status"`
	Score          float64 `json:"score"`
	SourceURL      string  `json:"source_url"`
	ImageRef       string  `json:"image_ref,omitempty"`
}

// --- Input/Output types ---

// SearchInput is GET /search's input.
type SearchInput struct {
	Q           string `query:"q" doc:"Search query text" example:"IT system safety requirements"`
	Corpora     string `query:"corpora" doc:"Comma-separated corpus IDs to search (empty = all)" example:"vn-reg,my-reg"`
	TopK        int    `query:"top_k" doc:"Maximum results (1-100, default 10)" example:"10"`
	InForceOnly string `query:"in_force_only" doc:"Restrict to in-force/amended: true (default) or false" example:"true"`
	AsOfDate    string `query:"as_of_date" doc:"Point-in-time filter (YYYY-MM-DD)" example:"2026-01-15"`
}

// SearchOutput is GET /search's output.
type SearchOutput struct {
	Body struct {
		Sections []SectionHitWire `json:"sections"`
	}
}

// RegisterSearch mounts the search REST endpoint.
func RegisterSearch(api huma.API, repo SearchRepoIface, role string) {
	huma.Register(api, huma.Operation{
		OperationID: "search",
		Method:      http.MethodGet,
		Path:        "/search",
		Summary:     "Hybrid search across evidence corpora (REST mirror of MCP search tool)",
		Tags:        []string{"Search"},
		Errors:      []int{http.StatusBadRequest},
	}, newSearchHandler(repo, role))
}

func newSearchHandler(
	repo SearchRepoIface, role string,
) func(context.Context, *SearchInput) (*SearchOutput, error) {
	return func(ctx context.Context, in *SearchInput) (*SearchOutput, error) {
		if in.Q == "" {
			return nil, huma.Error400BadRequest("query parameter q is required")
		}

		opts, err := buildSearchOpts(in, role)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}

		var hits []store.Hit
		if repo != nil {
			hits, err = repo.Search(ctx, in.Q, opts)
			if err != nil {
				return nil, fmt.Errorf("httpapi: searching: %w", err)
			}
		}

		out := &SearchOutput{}
		out.Body.Sections = mapHitsToWire(hits)
		return out, nil
	}
}

// buildSearchOpts converts the REST query params into store.SearchOpts.
func buildSearchOpts(in *SearchInput, role string) (store.SearchOpts, error) {
	var corpora []corpus.ID
	if in.Corpora != "" {
		for _, raw := range splitCorpora(in.Corpora) {
			if _, ok := corpus.Get(corpus.ID(raw)); !ok {
				return store.SearchOpts{}, fmt.Errorf("%q is not a registered corpus", raw)
			}
			corpora = append(corpora, corpus.ID(raw))
		}
	}

	inForceOnly := in.InForceOnly != "false"

	var asOf *time.Time
	if in.AsOfDate != "" {
		t, err := time.Parse("2006-01-02", in.AsOfDate)
		if err != nil {
			return store.SearchOpts{}, fmt.Errorf("invalid as_of_date %q: expected YYYY-MM-DD", in.AsOfDate)
		}
		asOf = &t
	}

	return store.SearchOpts{
		Corpora:     corpora,
		TopK:        in.TopK,
		InForceOnly: inForceOnly,
		AsOf:        asOf,
		Role:        role,
	}, nil
}

// splitCorpora splits a comma-separated string into trimmed non-empty parts.
func splitCorpora(s string) []string {
	var out []string
	start := 0
	for i := range len(s) {
		if s[i] == ',' {
			part := trimSpace(s[start:i])
			if part != "" {
				out = append(out, part)
			}
			start = i + 1
		}
	}
	part := trimSpace(s[start:])
	if part != "" {
		out = append(out, part)
	}
	return out
}

// trimSpace trims leading/trailing spaces from s without importing strings.
func trimSpace(s string) string {
	for len(s) > 0 && s[0] == ' ' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == ' ' {
		s = s[:len(s)-1]
	}
	return s
}

// mapHitsToWire maps store.Hit rows to their wire form. Always returns a
// non-nil slice.
func mapHitsToWire(hits []store.Hit) []SectionHitWire {
	out := make([]SectionHitWire, len(hits))
	for i, h := range hits {
		out[i] = SectionHitWire{
			CorpusID:       h.CorpusID,
			DocumentID:     h.DocumentID.String(),
			SectionID:      h.SectionID.String(),
			DocNumber:      h.DocNumber,
			Title:          h.Title,
			CitationPath:   h.CitationPath,
			HeadingPath:    h.HeadingPath,
			Text:           h.Text,
			ValidityStatus: h.ValidityStatus,
			Score:          h.Score,
			SourceURL:      h.SourceURL,
			ImageRef:       h.ImageRef,
		}
	}
	return out
}
