package mcp

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/store"
)

// --- shared tier helper ---

type tierSearcher struct {
	hits []store.Hit
}

func (s *tierSearcher) Search(
	_ context.Context, _ string, opts store.SearchOpts,
) ([]store.Hit, error) {
	var out []store.Hit
	for _, h := range s.hits {
		if canSeeTier(h.AccessTier, opts.Role) {
			out = append(out, h)
		}
	}
	if out == nil {
		out = []store.Hit{}
	}
	return out, nil
}

func canSeeTier(tier, role string) bool {
	switch tier {
	case "local-confidential":
		return role == "mise_local"
	case "group-confidential":
		return role == "mise_local" || role == "mise_group"
	default:
		return true
	}
}

func localConfidentialHit() store.Hit {
	return store.Hit{
		CorpusID:       "local-sop",
		DocumentID:     uuid.New(),
		SectionID:      uuid.New(),
		Text:           "confidential SOP text",
		CitationPath:   "SOP-001 §3",
		SourceURL:      "https://internal.example.com/sop-001",
		ValidityStatus: "in_force",
		Score:          0.95,
		AccessTier:     "local-confidential",
	}
}

// --- search tool tier deny ---

func TestSearchDenyPublicSeesNoLocalConfidential(t *testing.T) {
	t.Parallel()
	stub := &tierSearcher{hits: []store.Hit{localConfidentialHit()}}
	h := newSearchHandler(stub, "mise_public")

	_, out, err := h(context.Background(), nil, SearchInput{Query: "SOP"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if len(out.Sections) != 0 {
		t.Errorf("mise_public sees %d sections, want 0", len(out.Sections))
	}
}

func TestSearchDenyGroupSeesNoLocalConfidential(t *testing.T) {
	t.Parallel()
	stub := &tierSearcher{hits: []store.Hit{localConfidentialHit()}}
	h := newSearchHandler(stub, "mise_group")

	_, out, err := h(context.Background(), nil, SearchInput{Query: "SOP"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if len(out.Sections) != 0 {
		t.Errorf("mise_group sees %d sections, want 0", len(out.Sections))
	}
}

func TestSearchAllowLocalSeesLocalConfidential(t *testing.T) {
	t.Parallel()
	stub := &tierSearcher{hits: []store.Hit{localConfidentialHit()}}
	h := newSearchHandler(stub, "mise_local")

	_, out, err := h(context.Background(), nil, SearchInput{Query: "SOP"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if len(out.Sections) != 1 {
		t.Fatalf("mise_local sees %d sections, want 1", len(out.Sections))
	}
	s := out.Sections[0]
	if s.CitationPath == "" {
		t.Error("search hit missing citation_path (provenance)")
	}
	if s.SourceURL == "" {
		t.Error("search hit missing source_url (provenance)")
	}
}

// --- document tool tier deny ---

type tierDocGetter struct {
	detail    store.DocumentDetail
	accessErr error
}

func (g *tierDocGetter) GetDocument(
	_ context.Context, role, _ string, _ uuid.UUID,
) (store.DocumentDetail, error) {
	if !canSeeTier(g.detail.Doc.AccessTier, role) {
		return store.DocumentDetail{}, g.accessErr
	}
	return g.detail, nil
}

func localConfidentialDocDetail() store.DocumentDetail {
	return store.DocumentDetail{
		Doc: store.Document{
			ID:         uuid.New(),
			CorpusID:   "local-sop",
			Title:      "Confidential SOP",
			AccessTier: "local-confidential",
			SourceURL:  "https://internal.example.com/sop-001",
		},
	}
}

func TestDocumentDenyPublicIndistinguishableFromNotFound(t *testing.T) {
	t.Parallel()
	stub := &tierDocGetter{
		detail:    localConfidentialDocDetail(),
		accessErr: store.ErrDocumentNotFound,
	}
	h := newDocumentHandler(stub, "mise_public")

	_, _, err := h(context.Background(), nil, DocumentInput{
		CorpusID:   "local-sop",
		DocumentID: uuid.New().String(),
	})
	if err == nil {
		t.Fatal("expected error for denied document, got nil")
	}
}

func TestDocumentAllowLocalReturnsDetail(t *testing.T) {
	t.Parallel()
	detail := localConfidentialDocDetail()
	stub := &tierDocGetter{detail: detail}
	h := newDocumentHandler(stub, "mise_local")

	_, out, err := h(context.Background(), nil, DocumentInput{
		CorpusID:   detail.Doc.CorpusID,
		DocumentID: detail.Doc.ID.String(),
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if out.Document.Title != "Confidential SOP" {
		t.Errorf("title = %q, want Confidential SOP", out.Document.Title)
	}
	if out.Document.SourceURL == "" {
		t.Error("document missing source_url (provenance)")
	}
}

// --- graph tool tier deny ---

type tierGraphRepo struct {
	view store.NodeView
	hops []store.Hop
}

func (g *tierGraphRepo) GetNode(
	_ context.Context, role string, _ graph.NodeRef,
) (store.NodeView, error) {
	filtered := store.NodeView{Ref: g.view.Ref}
	for _, e := range g.view.Edges {
		if canSeeTier(string(e.AccessTier), role) {
			filtered.Edges = append(filtered.Edges, e)
		}
	}
	filtered.Evidence = g.view.Evidence
	return filtered, nil
}

func (g *tierGraphRepo) Chain(
	_ context.Context, _ string, _ graph.NodeRef, _ int,
) ([]store.Hop, error) {
	return g.hops, nil
}

func TestGraphDenyPublicSeesNoLocalEdges(t *testing.T) {
	t.Parallel()
	sopRef := graph.NodeRef{
		CorpusID: "local-sop", DocumentID: uuid.New(),
	}
	edgeID := uuid.New()
	stub := &tierGraphRepo{
		view: store.NodeView{
			Ref: sopRef,
			Edges: []graph.Edge{{
				ID:         edgeID,
				From:       sopRef,
				ToRefID:    uuid.New(),
				ToCorpusID: "local-policy",
				EdgeType:   "derives",
				Direction:  "up",
				AccessTier: "local-confidential",
			}},
			Evidence: map[uuid.UUID][]graph.Evidence{},
		},
		hops: nil,
	}
	h := newGraphHandler(stub, "mise_public")

	nodeRef := sopRef.CorpusID + "/" + sopRef.DocumentID.String()
	_, out, err := h(context.Background(), nil, GraphInput{NodeRef: nodeRef})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if len(out.Edges) != 0 {
		t.Errorf("mise_public sees %d edges, want 0", len(out.Edges))
	}
	if len(out.Chain) != 0 {
		t.Errorf("mise_public sees %d chain hops, want 0", len(out.Chain))
	}
}

func TestGraphAllowLocalSeesLocalEdges(t *testing.T) {
	t.Parallel()
	sopRef := graph.NodeRef{
		CorpusID: "local-sop", DocumentID: uuid.New(),
	}
	edgeID := uuid.New()
	stub := &tierGraphRepo{
		view: store.NodeView{
			Ref: sopRef,
			Edges: []graph.Edge{{
				ID:         edgeID,
				From:       sopRef,
				ToRefID:    uuid.New(),
				ToCorpusID: "local-policy",
				EdgeType:   "derives",
				Direction:  "up",
				AccessTier: "local-confidential",
			}},
			Evidence: map[uuid.UUID][]graph.Evidence{
				edgeID: {{Confidence: 0.9, GroundingScore: 0.85}},
			},
		},
		hops: []store.Hop{{
			Ref:            graph.NodeRef{CorpusID: "local-policy", DocumentID: uuid.New()},
			Confidence:     0.9,
			GroundingScore: 0.85,
		}},
	}
	h := newGraphHandler(stub, "mise_local")

	nodeRef := sopRef.CorpusID + "/" + sopRef.DocumentID.String()
	_, out, err := h(context.Background(), nil, GraphInput{NodeRef: nodeRef})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if len(out.Edges) != 1 {
		t.Fatalf("mise_local sees %d edges, want 1", len(out.Edges))
	}
	if len(out.Chain) != 1 {
		t.Fatalf("mise_local sees %d chain hops, want 1", len(out.Chain))
	}
}
