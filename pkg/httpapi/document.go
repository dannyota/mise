package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/store"
)

// DocumentRepoIface is the document endpoint's dependency — satisfied by
// cmd/serving's storeDocGetter adapter — narrowed to the single GetDocument
// method the handler needs, consumer-defined per CODE_STYLE_GO.
type DocumentRepoIface interface {
	GetDocument(ctx context.Context, role, corpusID string, docID uuid.UUID) (store.DocumentDetail, error)
}

// --- Wire types ---

// DocumentDetailWire is the wire form of store.DocumentDetail. Matches the MCP
// document tool's DocumentOutput shape (pkg/mcp/tools.go) so REST and MCP
// return equivalent JSON.
type DocumentDetailWire struct {
	Document   DocumentEnvelopeWire `json:"document"`
	Sections   []DocSectionWire     `json:"sections"`
	Amendments []AmendmentWire      `json:"amendments"`
}

// DocumentEnvelopeWire is the wire form of store.Document.
type DocumentEnvelopeWire struct {
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

// DocSectionWire is the wire form of one document section.
type DocSectionWire struct {
	ID             string `json:"id"`
	CitationPath   string `json:"citation_path"`
	HeadingPath    string `json:"heading_path"`
	Text           string `json:"text"`
	ValidityStatus string `json:"validity_status"`
	Position       int    `json:"position"`
	ImageRef       string `json:"image_ref,omitempty"`
}

// AmendmentWire is the wire form of store.AmendmentEvent.
type AmendmentWire struct {
	AmendingDocID *string `json:"amending_doc_id,omitempty"`
	Kind          string  `json:"kind,omitempty"`
	Clause        string  `json:"clause"`
	EventDate     string  `json:"event_date"`
}

// --- Input/Output types ---

// DocumentGetInput is GET /documents/{corpus}/{id}'s input.
type DocumentGetInput struct {
	Corpus string `path:"corpus" doc:"Corpus ID" example:"vn-reg"`
	ID     string `path:"id" doc:"Document UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
}

// DocumentGetOutput is GET /documents/{corpus}/{id}'s output.
type DocumentGetOutput struct {
	Body DocumentDetailWire
}

// RegisterDocument mounts the document detail REST endpoint.
func RegisterDocument(api huma.API, repo DocumentRepoIface, role string) {
	huma.Register(api, huma.Operation{
		OperationID: "get-document",
		Method:      http.MethodGet,
		Path:        "/documents/{corpus}/{id}",
		Summary:     "Get full document envelope with sections and amendment timeline",
		Tags:        []string{"Documents"},
		Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
	}, newGetDocumentHandler(repo, role))
}

func newGetDocumentHandler(
	repo DocumentRepoIface, role string,
) func(context.Context, *DocumentGetInput) (*DocumentGetOutput, error) {
	return func(ctx context.Context, in *DocumentGetInput) (*DocumentGetOutput, error) {
		if _, ok := corpus.Get(corpus.ID(in.Corpus)); !ok {
			return nil, huma.Error404NotFound(fmt.Sprintf("corpus %q not found", in.Corpus))
		}
		docID, err := uuid.Parse(in.ID)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid document UUID", err)
		}

		detail, err := repo.GetDocument(ctx, role, in.Corpus, docID)
		if err != nil {
			if errors.Is(err, store.ErrDocumentNotFound) {
				return nil, huma.Error404NotFound("document not found")
			}
			return nil, fmt.Errorf("httpapi: getting document: %w", err)
		}

		out := &DocumentGetOutput{}
		out.Body = mapDocumentDetail(detail)
		return out, nil
	}
}

// mapDocumentDetail maps store.DocumentDetail to its wire form.
func mapDocumentDetail(d store.DocumentDetail) DocumentDetailWire {
	return DocumentDetailWire{
		Document:   mapDocEnvelope(d.Doc),
		Sections:   mapDocSectionsWire(d.Sections),
		Amendments: mapAmendmentsWire(d.Events),
	}
}

func mapDocEnvelope(d store.Document) DocumentEnvelopeWire {
	return DocumentEnvelopeWire{
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
		IssuedDate:       timePtrToWire(d.IssuedDate),
		EffectiveDate:    timePtrToWire(d.EffectiveDate),
		ExpiryDate:       timePtrToWire(d.ExpiryDate),
		IngestRunID:      d.IngestRunID.String(),
		ObservedAt:       d.ObservedAt.Format(time.RFC3339),
	}
}

func mapDocSectionsWire(sections []store.Section) []DocSectionWire {
	out := make([]DocSectionWire, len(sections))
	for i, s := range sections {
		out[i] = DocSectionWire{
			ID:             s.ID.String(),
			CitationPath:   s.CitationPath,
			HeadingPath:    s.HeadingPath,
			Text:           s.Body,
			ValidityStatus: s.ValidityStatus,
			Position:       s.Position,
			ImageRef:       s.ImageRef,
		}
	}
	return out
}

func mapAmendmentsWire(events []store.AmendmentEvent) []AmendmentWire {
	out := make([]AmendmentWire, len(events))
	for i, e := range events {
		var amendingID *string
		if e.AmendingDocID != nil {
			s := e.AmendingDocID.String()
			amendingID = &s
		}
		out[i] = AmendmentWire{
			AmendingDocID: amendingID,
			Kind:          e.Kind,
			Clause:        e.Clause,
			EventDate:     e.EventDate.Format(time.RFC3339),
		}
	}
	return out
}

// timePtrToWire renders a *time.Time as an RFC3339 string pointer, or nil.
func timePtrToWire(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.RFC3339)
	return &s
}
