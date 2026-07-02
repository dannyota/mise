package vbpl

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"danny.vn/mise/pkg/ingest"
)

// detailData is the GET /doc/{id} `data` object. Only the fields mise consumes
// are modeled; the rest is preserved verbatim via the bronze `detail_json` raw
// payload (see FetchDetail), so skipped fields can be mined later without a
// re-crawl. Objects vbpl serves as null in some documents (effStatus, docType,
// organization, documentContent) decode to the zero value, so reads are nil-safe.
type detailData struct {
	ID                     string   `json:"id"`
	DocNum                 string   `json:"docNum"`
	Title                  string   `json:"title"`
	IssueDate              string   `json:"issueDate"`
	EffFrom                string   `json:"effFrom"`
	EffTo                  string   `json:"effTo"`
	AgencyName             string   `json:"agencyName"`
	Organization           codeName `json:"organization"` // stable issuer code/name ({code,name} OR null)
	EffStatus              codeName `json:"effStatus"`    // {code,name} OR null — parse defensively
	DocType                codeName `json:"docType"`      // {code,name} OR null
	HasContent             bool     `json:"hasContent"`
	IsConsolidatedDocument bool     `json:"isConsolidatedDocument"`
	// DocumentContent.content is the born-digital body HTML carried inline; we
	// keep it in DiscoveredDoc.HTML rather than downloading the *_content.html.
	DocumentContent struct {
		Content string `json:"content"`
	} `json:"documentContent"`
	References []vbplReference `json:"references"`
}

type provisionTreeResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
}

// FetchDetail fetches a document's metadata and file list from the vbpl JSON API
// using the opaque id returned by doc/all. The id can be a legacy numeric ItemID
// or a UUID, so it is never parsed as a number. DetailURL is only a fallback for
// ad-hoc callers that have not come through Discover.
func (s *Source) FetchDetail(ctx context.Context, ref ingest.DetailRef) (*ingest.DiscoveredDoc, error) {
	id, err := docID(ref)
	if err != nil {
		return nil, err
	}

	// Decode twice: capture the verbatim `data` object as raw JSON (preserved to
	// bronze so fields we don't yet map — signer, organization, referenceProvisions,
	// flags — can be mined later without re-crawling), then into the typed struct.
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := s.getJSON(ctx, apiBase+"/doc/"+id, &envelope); err != nil {
		return nil, fmt.Errorf("fetch detail %s: %w", id, err)
	}
	var d detailData
	if err := json.Unmarshal(envelope.Data, &d); err != nil {
		return nil, fmt.Errorf("decode detail %s: %w", id, err)
	}

	var files filesResponse
	filesURL := apiBase + "/doc/minio/buckets/vbpl/folders/" + id + "/files?parts=1,2,3,4,5"
	if err := s.getJSON(ctx, filesURL, &files); err != nil {
		return nil, fmt.Errorf("fetch files %s: %w", id, err)
	}

	var diagram diagramResponse
	if err := s.getJSON(ctx, apiBase+"/doc/"+id+"/diagram", &diagram); err != nil {
		return nil, fmt.Errorf("fetch diagram %s: %w", id, err)
	}
	if !diagram.Success {
		return nil, fmt.Errorf("fetch diagram %s: unsuccessful response", id)
	}

	doc := &ingest.DiscoveredDoc{
		SourceID:    SourceID,
		ExternalID:  id,
		DocGUID:     d.ID,
		Number:      d.DocNum,
		Title:       d.Title,
		DocType:     ingest.DocType(d.DocType.Name),
		DocTypeCode: d.DocType.Code,
		Issuer:      d.AgencyName,
		IssuerCode:  strings.TrimSpace(d.Organization.Code), // stable issuer identity (vs. fuzzy agencyName)
		Status:      d.EffStatus.Code,                       // CHL / HHL / HHL1P … ("" when effStatus is null)
		IssuedAt:    parseDate(d.IssueDate),
		EffectiveAt: parseDate(d.EffFrom),
		ExpireAt:    parseDate(d.EffTo),
		DetailURL:   detailURL(id),
		HTML:        d.DocumentContent.Content,
		Files:       preferredFiles(files.Data),
		Relations: mergeVBPLRelations(
			s.vbplRelations(d.References),
			s.vbplDiagramRelations(diagram.Data.DocumentNamesByType),
		),
		HasContent:     d.HasContent || strings.TrimSpace(d.DocumentContent.Content) != "",
		IsConsolidated: d.IsConsolidatedDocument,
		RawMeta:        detailRawMeta(envelope.Data),
	}
	return doc, nil
}

// detailRawMeta returns the verbatim detail `data` object minus the bulky inline
// HTML body (documentContent — kept in DiscoveredDoc.HTML / the content_html
// payload, so duplicating ~130KB per doc here would only bloat storage). Returns
// nil on malformed input so a fetch is never failed over preservation. Persisting
// this keeps signer, organization, referenceProvisions, publicDate, the document
// taxonomy, and other unmapped fields minable later without re-crawling.
func detailRawMeta(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	delete(m, "documentContent")
	out, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return out
}

// FetchTree fetches the official VBPL Điều/Khoản provision tree. VBPL returns an
// empty array for some recently published or placeholder documents; that is not a
// hard failure, because Normalize can fall back to the official HTML/DOCX text.
func (s *Source) FetchTree(ctx context.Context, ref ingest.DetailRef) (string, bool, error) {
	id, err := docID(ref)
	if err != nil {
		return "", false, err
	}

	var tree provisionTreeResponse
	if err := s.getJSON(ctx, apiBase+"/doc/provision/tree/"+id, &tree); err != nil {
		return "", false, fmt.Errorf("fetch provision tree %s: %w", id, err)
	}
	if !tree.Success {
		return "", false, fmt.Errorf("fetch provision tree %s: unsuccessful response", id)
	}

	content := strings.TrimSpace(string(tree.Data))
	switch content {
	case "", "null", "[]":
		return "", false, nil
	}
	var nodes []json.RawMessage
	if err := json.Unmarshal(tree.Data, &nodes); err != nil {
		return "", false, fmt.Errorf("decode provision tree %s: %w", id, err)
	}
	if len(nodes) == 0 {
		return "", false, nil
	}
	return content, true, nil
}

// docID returns the source API id to use for vbpl detail calls. The Discover
// response already gives this id as text; it may be numeric or UUID, and the
// gateway accepts either verbatim. DetailURL parsing is only a compatibility
// fallback for one-off calls.
func docID(ref ingest.DetailRef) (string, error) {
	if id := strings.TrimSpace(ref.ExternalID); id != "" {
		return id, nil
	}
	return parseDocID(ref.DetailURL)
}

// parseDocID extracts the document id — the last path segment — from a vbpl
// detail URL (…/van-ban/chi-tiet/{id}). The id is either a legacy numeric ItemID
// (…/chi-tiet/144532) or the newer UUID form (…/chi-tiet/835e3190-54dd-…); the
// gateway's doc/{id} endpoint accepts either verbatim, so the segment is used
// as-is — extracting a substring (e.g. a trailing numeric run) would silently
// resolve a UUID to the wrong document. Query and fragment are stripped first.
func parseDocID(detailURL string) (string, error) {
	s := strings.TrimSpace(detailURL)
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimRight(s, "/")
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		s = s[i+1:]
	}
	if s == "" {
		return "", fmt.Errorf("parse doc id from %q: no id segment", detailURL)
	}
	return s, nil
}
