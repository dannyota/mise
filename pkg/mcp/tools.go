package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/store"
)

// defaultSearchTopK is the search tool's result cap when top_k is omitted or
// <=0 (API-CONTRACT §2).
const defaultSearchTopK = 10

// Searcher is the search tool's read-side dependency — satisfied by
// store.Search closed over a pool/embedder (see cmd/serving's wiring).
type Searcher interface {
	Search(ctx context.Context, query string, opts store.SearchOpts) ([]store.Hit, error)
}

// DocGetter is the document tool's read-side dependency — satisfied by a
// per-corpus store.Corpus.GetDocument, resolved by corpusID (see cmd/
// serving's wiring).
type DocGetter interface {
	GetDocument(ctx context.Context, role string, corpusID string, docID uuid.UUID) (store.DocumentDetail, error)
}

// SearchInput is the search tool's input (API-CONTRACT §2).
type SearchInput struct {
	Query       string   `json:"query"`
	Corpora     []string `json:"corpora,omitempty"`
	TopK        int      `json:"top_k,omitempty"`
	AsOfDate    string   `json:"as_of_date,omitempty"`    // YYYY-MM-DD
	InForceOnly *bool    `json:"in_force_only,omitempty"` // default true
}

// SearchOutput is the search tool's output: ranked sections.
type SearchOutput struct {
	Sections []SectionHit `json:"sections"`
}

// SectionHit is one ranked section result — the wire form of store.Hit.
type SectionHit struct {
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

// DocumentInput is the document tool's input.
type DocumentInput struct {
	CorpusID   string `json:"corpus_id"`
	DocumentID string `json:"document_id"`
}

// DocumentOutput is the document tool's output: the full metadata envelope,
// verbatim sections, and the amendment timeline (API-CONTRACT §2).
type DocumentOutput struct {
	Document   DocumentEnvelope `json:"document"`
	Sections   []DocSection     `json:"sections"`
	Amendments []AmendmentOut   `json:"amendments"`
}

// DocumentEnvelope mirrors store.Document (pkg/store/models.go) on the wire:
// snake_case, with uuid.UUID/time.Time rendered as strings.
type DocumentEnvelope struct {
	ID               string  `json:"id"`
	CorpusID         string  `json:"corpus_id"`
	Title            string  `json:"title"`
	DocNumber        string  `json:"doc_number"`
	CitationScheme   string  `json:"citation_scheme"`
	CitationPath     string  `json:"citation_path"`
	Language         string  `json:"language"`
	ValidityStatus   string  `json:"validity_status"`
	IssuingAuthority string  `json:"issuing_authority"`
	SignerName       string  `json:"signer_name"`
	Version          string  `json:"version"`
	SourceURL        string  `json:"source_url"`
	SourceSystem     string  `json:"source_system"`
	ContentType      string  `json:"content_type"`
	AccessTier       string  `json:"access_tier"`
	IssuedDate       *string `json:"issued_date,omitempty"`
	EffectiveDate    *string `json:"effective_date,omitempty"`
	ExpiryDate       *string `json:"expiry_date,omitempty"`
	IngestRunID      string  `json:"ingest_run_id"`
	ObservedAt       string  `json:"observed_at"`
}

// DocSection is one document section: citation, heading, verbatim text, and
// its own validity status (a section may be amended independently of its
// parent document).
type DocSection struct {
	CitationPath   string `json:"citation_path"`
	HeadingPath    string `json:"heading_path"`
	Text           string `json:"text"`
	ValidityStatus string `json:"validity_status"`
}

// AmendmentOut is one dated act on the document's validity. AmendingDocID is
// omitted when the amending document isn't resolved in the store; Kind
// (amended/superseded/repealed) is omitted only for a pre-migration-008 row
// that predates the column.
type AmendmentOut struct {
	AmendingDocID *string `json:"amending_doc_id,omitempty"`
	Kind          string  `json:"kind,omitempty"`
	Clause        string  `json:"clause"`
	EventDate     string  `json:"event_date"`
}

// registerEvidenceTools binds s/g/role into the search and document tool
// handlers and registers them on srv via the SDK's typed mcp.AddTool, which
// derives each tool's JSON input/output schema from the Go structs above.
func registerEvidenceTools(srv *mcp.Server, s Searcher, g DocGetter, role string) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "search",
		Description: "Validity-aware hybrid search across mise's evidence corpora. " +
			"Returns ranked sections with citation, validity status, and source.",
	}, newSearchHandler(s, role))
	mcp.AddTool(srv, &mcp.Tool{
		Name: "document",
		Description: "Full document metadata envelope, verbatim sections, and amendment " +
			"timeline for one corpus_id/document_id.",
	}, newDocumentHandler(g, role))
}

// newSearchHandler returns the search tool's typed handler, closed over s
// and role.
func newSearchHandler(s Searcher, role string) mcp.ToolHandlerFor[SearchInput, SearchOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, SearchOutput, error) {
		opts, err := searchOptsFromInput(in, role)
		if err != nil {
			return nil, SearchOutput{}, err
		}
		hits, err := s.Search(ctx, in.Query, opts)
		if err != nil {
			return nil, SearchOutput{}, fmt.Errorf("mcp search: %w", err)
		}
		return nil, SearchOutput{Sections: mapSectionHits(hits)}, nil
	}
}

// newDocumentHandler returns the document tool's typed handler, closed over
// g and role.
func newDocumentHandler(g DocGetter, role string) mcp.ToolHandlerFor[DocumentInput, DocumentOutput] {
	return func(
		ctx context.Context, _ *mcp.CallToolRequest, in DocumentInput,
	) (*mcp.CallToolResult, DocumentOutput, error) {
		if _, ok := corpus.Get(corpus.ID(in.CorpusID)); !ok {
			return nil, DocumentOutput{}, fmt.Errorf("mcp document: %q is not a registered corpus", in.CorpusID)
		}
		docID, err := uuid.Parse(in.DocumentID)
		if err != nil {
			return nil, DocumentOutput{}, fmt.Errorf("mcp document: invalid document_id %q: %w", in.DocumentID, err)
		}

		detail, err := g.GetDocument(ctx, role, in.CorpusID, docID)
		if err != nil {
			return nil, DocumentOutput{}, fmt.Errorf("mcp document: %w", err)
		}
		return nil, DocumentOutput{
			Document:   mapDocumentEnvelope(detail.Doc),
			Sections:   mapDocSections(detail.Sections),
			Amendments: mapAmendments(detail.Events),
		}, nil
	}
}

// searchOptsFromInput resolves in into a store.SearchOpts, applying the
// search tool's defaults (top_k=10, in_force_only=true, corpora=every
// registered corpus) and validating corpora/as_of_date.
func searchOptsFromInput(in SearchInput, role string) (store.SearchOpts, error) {
	ids, err := resolveCorpora(in.Corpora)
	if err != nil {
		return store.SearchOpts{}, err
	}

	topK := in.TopK
	if topK <= 0 {
		topK = defaultSearchTopK
	}

	inForceOnly := true
	if in.InForceOnly != nil {
		inForceOnly = *in.InForceOnly
	}

	asOf, err := parseAsOfDate(in.AsOfDate)
	if err != nil {
		return store.SearchOpts{}, err
	}

	return store.SearchOpts{
		Corpora:     ids,
		TopK:        topK,
		InForceOnly: inForceOnly,
		AsOf:        asOf,
		Role:        role,
	}, nil
}

// parseAsOfDate parses raw as a YYYY-MM-DD date, returning nil when raw is
// empty (as_of_date omitted).
func parseAsOfDate(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil //nolint:nilnil // absent as_of_date is not an error condition
	}
	t, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return nil, fmt.Errorf("mcp search: invalid as_of_date %q, want YYYY-MM-DD: %w", raw, err)
	}
	return &t, nil
}

// resolveCorpora validates raw corpus IDs against the registry, defaulting
// to every registered corpus when raw is empty.
func resolveCorpora(raw []string) ([]corpus.ID, error) {
	if len(raw) == 0 {
		return allCorpusIDs(), nil
	}
	out := make([]corpus.ID, len(raw))
	for i, r := range raw {
		id := corpus.ID(r)
		if _, ok := corpus.Get(id); !ok {
			return nil, fmt.Errorf("mcp search: %q is not a registered corpus", r)
		}
		out[i] = id
	}
	return out, nil
}

// allCorpusIDs returns every registered corpus ID (the search tool's default
// scope when corpora is omitted).
func allCorpusIDs() []corpus.ID {
	all := corpus.All()
	out := make([]corpus.ID, len(all))
	for i, d := range all {
		out[i] = d.ID
	}
	return out
}

// mapSectionHits maps store.Hit rows to the tool's wire form. Always
// returns a non-nil slice (even for zero hits) so the output marshals to
// `[]`, never `null`.
func mapSectionHits(hits []store.Hit) []SectionHit {
	out := make([]SectionHit, len(hits))
	for i, h := range hits {
		out[i] = SectionHit{
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

// mapDocumentEnvelope maps a store.Document to its wire form.
func mapDocumentEnvelope(d store.Document) DocumentEnvelope {
	return DocumentEnvelope{
		ID:               d.ID.String(),
		CorpusID:         d.CorpusID,
		Title:            d.Title,
		DocNumber:        d.DocNumber,
		CitationScheme:   d.CitationScheme,
		CitationPath:     d.CitationPath,
		Language:         d.Language,
		ValidityStatus:   d.ValidityStatus,
		IssuingAuthority: d.IssuingAuthority,
		SignerName:       d.SignerName,
		Version:          d.Version,
		SourceURL:        d.SourceURL,
		SourceSystem:     d.SourceSystem,
		ContentType:      d.ContentType,
		AccessTier:       d.AccessTier,
		IssuedDate:       formatTimePtr(d.IssuedDate),
		EffectiveDate:    formatTimePtr(d.EffectiveDate),
		ExpiryDate:       formatTimePtr(d.ExpiryDate),
		IngestRunID:      d.IngestRunID.String(),
		ObservedAt:       d.ObservedAt.Format(time.RFC3339),
	}
}

// mapDocSections maps store.Section rows to the tool's reduced wire
// projection (citation_path, heading_path, text, validity_status — no ids,
// embeddings, or access_tier). Always returns a non-nil slice.
func mapDocSections(secs []store.Section) []DocSection {
	out := make([]DocSection, len(secs))
	for i, s := range secs {
		out[i] = DocSection{
			CitationPath:   s.CitationPath,
			HeadingPath:    s.HeadingPath,
			Text:           s.Body,
			ValidityStatus: s.ValidityStatus,
		}
	}
	return out
}

// mapAmendments maps store.AmendmentEvent rows to the tool's wire form.
// Always returns a non-nil slice.
func mapAmendments(evs []store.AmendmentEvent) []AmendmentOut {
	out := make([]AmendmentOut, len(evs))
	for i, e := range evs {
		var amendingID *string
		if e.AmendingDocID != nil {
			s := e.AmendingDocID.String()
			amendingID = &s
		}
		out[i] = AmendmentOut{
			AmendingDocID: amendingID,
			Kind:          e.Kind,
			Clause:        e.Clause,
			EventDate:     e.EventDate.Format(time.RFC3339),
		}
	}
	return out
}

// formatTimePtr formats t as RFC 3339, or returns nil when t is nil — the
// wire form of store.Document's nullable date fields.
func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.RFC3339)
	return &s
}
