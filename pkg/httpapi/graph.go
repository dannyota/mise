package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/store"
)

// errMalformedRef reports a {ref} path parameter that doesn't parse as
// corpus_id/document_id[/section_id] — a genuinely malformed request
// (mapped to 400), never folded into the not-found/RLS-denied 404 that
// GraphRepoIface's own methods report (see parseRef).
var errMalformedRef = errors.New("httpapi: malformed ref")

// GraphRepoIface is the graph endpoints' read-side dependency — satisfied
// by *store.GraphRepo (cmd/serving's wiring, newRouter) — narrowed to just
// GetNode/Chain, consumer-defined per CODE_STYLE_GO, so Register can be
// exercised against a fake in graph_test.go with no database. A ref this
// package can parse but role can't see any edges for — because it truly has
// none, or every edge sits above role's tier — reports store.ErrNodeNotFound
// from GetNode; the two causes are deliberately indistinguishable
// (store/graph_read.go), and both map to HTTP 404 here.
type GraphRepoIface interface {
	GetNode(ctx context.Context, role string, ref graph.NodeRef) (store.NodeView, error)
	Chain(ctx context.Context, role string, start graph.NodeRef, maxDepth int) ([]store.Hop, error)
}

// NodeRefWire is the wire form of graph.NodeRef: corpus_id/document_id,
// optionally narrowed to one section_id.
type NodeRefWire struct {
	CorpusID   string  `json:"corpus_id"`
	DocumentID string  `json:"document_id"`
	SectionID  *string `json:"section_id,omitempty"`
}

// EvidenceWire is the wire form of graph.Evidence: the support for why an
// edge exists, plus its full audit trail — run_id/model/prompt_hash and
// created_by/promoted_by (DATA-MODEL.md §4).
type EvidenceWire struct {
	ID             string  `json:"id"`
	EvidenceKind   string  `json:"evidence_kind"`
	Confidence     float64 `json:"confidence"`
	GroundingScore float64 `json:"grounding_score"`
	Rationale      string  `json:"rationale,omitempty"`
	QuotedFromSpan string  `json:"quoted_from_span,omitempty"`
	QuotedToSpan   string  `json:"quoted_to_span,omitempty"`
	Model          string  `json:"model,omitempty"`
	PromptHash     string  `json:"prompt_hash,omitempty"`
	CreatedBy      string  `json:"created_by,omitempty"`
	PromotedBy     string  `json:"promoted_by,omitempty"`
	RunID          *string `json:"run_id,omitempty"`
	PromotedAt     *string `json:"promoted_at,omitempty"`
	CreatedAt      string  `json:"created_at"`
}

// EdgeWire is the wire form of graph.Edge: one typed, directional relation
// from a node to a doc_ref target, plus its backing evidence rows.
type EdgeWire struct {
	ID         string         `json:"id"`
	From       NodeRefWire    `json:"from"`
	ToRefID    string         `json:"to_ref_id"`
	ToCorpusID string         `json:"to_corpus_id"`
	EdgeType   string         `json:"edge_type"`
	Direction  string         `json:"direction"`
	Promoted   bool           `json:"promoted"`
	AccessTier string         `json:"access_tier"`
	CreatedAt  string         `json:"created_at"`
	Evidence   []EvidenceWire `json:"evidence"`
}

// NodeBody is GET /graph/nodes/{ref}'s response body — the wire form of
// store.NodeView: ref echoed back, plus its outgoing edges.
type NodeBody struct {
	Ref   NodeRefWire `json:"ref"`
	Edges []EdgeWire  `json:"edges"`
}

// NodeInput is GET /graph/nodes/{ref}'s input.
type NodeInput struct {
	Ref string `path:"ref" maxLength:"200" doc:"corpus_id/document_id[/section_id], / as %2F" example:"vn-reg%2Fdoc-uuid"`
}

// NodeOutput is GET /graph/nodes/{ref}'s output.
type NodeOutput struct {
	Body NodeBody
}

// HopWire is the wire form of store.Hop: one step of a control-chain walk.
type HopWire struct {
	Ref            NodeRefWire `json:"ref"`
	EdgeType       string      `json:"edge_type"`
	CorpusID       string      `json:"corpus_id"`
	Citation       string      `json:"citation"`
	Text           string      `json:"text,omitempty"`
	Promoted       bool        `json:"promoted"`
	Confidence     float64     `json:"confidence"`
	GroundingScore float64     `json:"grounding_score"`
}

// ChainBody is GET /graph/chain/{ref}'s response body — the wire form of
// []store.Hop: the control chain walked from ref (e.g. SOP -> Policy ->
// Group -> law, DATA-MODEL.md §4), in walk order.
type ChainBody struct {
	Hops []HopWire `json:"hops"`
}

// ChainInput is GET /graph/chain/{ref}'s input. MaxDepth is optional — zero,
// omitted, or negative all mean "use the server's default cap" — and can
// only ever shrink store.Chain's walk, never grow it past that cap.
type ChainInput struct {
	Ref      string `path:"ref" maxLength:"200" doc:"corpus_id/document_id[/section_id], / as %2F" example:"sop%2Fdoc"`
	MaxDepth int    `query:"max_depth" doc:"Max hops; <=0 uses the default cap (shrink only)" example:"8"`
}

// ChainOutput is GET /graph/chain/{ref}'s output.
type ChainOutput struct {
	Body ChainBody
}

// Register wires all REST operations onto api: graph endpoints (GET
// /graph/nodes/{ref}, GET /graph/chain/{ref}) and review/finding endpoints.
// Every read runs under role — the server-resolved RLS role
// (pkg/config.Role()), never derived from request input. Repo arguments may
// be nil when api only exists to generate the OpenAPI spec (GenerateSpec).
func Register(
	api huma.API, graphRepo GraphRepoIface,
	reviewRepo ReviewRepoIface, findingRepo FindingRepoIface,
	role string,
) {
	huma.Register(api, huma.Operation{
		OperationID: "get-graph-node",
		Method:      http.MethodGet,
		Path:        "/graph/nodes/{ref}",
		Summary:     "Get a graph node's outgoing edges",
		Description: "Returns ref's outgoing relation_edge rows, each with its backing " +
			"relation_evidence, tier-filtered to the caller's role. A ref with no visible " +
			"edges — whether it truly has none, or every edge sits above role's tier — " +
			"reports 404: the two cases are deliberately indistinguishable (DATA-MODEL.md §4).",
		Tags:   []string{"Graph"},
		Errors: []int{http.StatusBadRequest, http.StatusNotFound},
	}, newNodeHandler(graphRepo, role))

	huma.Register(api, huma.Operation{
		OperationID: "get-graph-chain",
		Method:      http.MethodGet,
		Path:        "/graph/chain/{ref}",
		Summary:     "Walk the control chain from a graph node",
		Description: "Walks relation_edge's \"up\" direction from ref — e.g. SOP -> Policy -> " +
			"Group -> law (DATA-MODEL.md §4) — tier-filtering every hop exactly like " +
			"GET /graph/nodes/{ref}. A hop role can't see, or that doesn't exist, ends the " +
			"walk cleanly (200 with fewer hops) — never a 404 by itself.",
		Tags:   []string{"Graph"},
		Errors: []int{http.StatusBadRequest, http.StatusNotFound},
	}, newChainHandler(graphRepo, role))

	RegisterReviews(api, reviewRepo, findingRepo, role)
}

// newNodeHandler returns GET /graph/nodes/{ref}'s typed handler, closed over
// repo and role.
func newNodeHandler(repo GraphRepoIface, role string) func(context.Context, *NodeInput) (*NodeOutput, error) {
	return func(ctx context.Context, in *NodeInput) (*NodeOutput, error) {
		ref, err := parseRef(in.Ref)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid ref", err)
		}

		view, err := repo.GetNode(ctx, role, ref)
		switch {
		case errors.Is(err, store.ErrNodeNotFound):
			return nil, huma.Error404NotFound("graph node not found")
		case err != nil:
			return nil, fmt.Errorf("httpapi: getting graph node: %w", err)
		}

		out := &NodeOutput{}
		out.Body = NodeBody{Ref: nodeRefToWire(view.Ref), Edges: mapEdges(view.Edges, view.Evidence)}
		return out, nil
	}
}

// newChainHandler returns GET /graph/chain/{ref}'s typed handler, closed
// over repo and role.
func newChainHandler(repo GraphRepoIface, role string) func(context.Context, *ChainInput) (*ChainOutput, error) {
	return func(ctx context.Context, in *ChainInput) (*ChainOutput, error) {
		ref, err := parseRef(in.Ref)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid ref", err)
		}

		hops, err := repo.Chain(ctx, role, ref, in.MaxDepth)
		switch {
		case errors.Is(err, store.ErrNodeNotFound):
			return nil, huma.Error404NotFound("graph node not found")
		case err != nil:
			return nil, fmt.Errorf("httpapi: walking graph chain: %w", err)
		}

		out := &ChainOutput{}
		out.Body = ChainBody{Hops: mapHops(hops)}
		return out, nil
	}
}

// parseRef parses a {ref} path parameter of the form "corpus_id/document_id"
// or "corpus_id/document_id/section_id" into a graph.NodeRef. corpus_id is
// passed through opaque: an unregistered corpus simply matches zero rows
// downstream (store.ErrNodeNotFound, mapped to 404 exactly like a genuine
// not-found), so parseRef never needs pkg/corpus. document_id/section_id
// must each parse as a UUID — that failure is genuinely malformed input, so
// it is reported as 400, never folded into the not-found/RLS-denied 404.
func parseRef(ref string) (graph.NodeRef, error) {
	parts := strings.Split(ref, "/")
	if len(parts) < 2 || len(parts) > 3 || parts[0] == "" || parts[1] == "" {
		return graph.NodeRef{}, fmt.Errorf("%w: %q, want corpus_id/document_id[/section_id]", errMalformedRef, ref)
	}

	docID, err := uuid.Parse(parts[1])
	if err != nil {
		return graph.NodeRef{}, fmt.Errorf("%w: document_id %q: %w", errMalformedRef, parts[1], err)
	}
	nodeRef := graph.NodeRef{CorpusID: parts[0], DocumentID: docID}

	if len(parts) == 3 {
		if parts[2] == "" {
			return graph.NodeRef{}, fmt.Errorf("%w: empty section_id segment in %q", errMalformedRef, ref)
		}
		secID, err := uuid.Parse(parts[2])
		if err != nil {
			return graph.NodeRef{}, fmt.Errorf("%w: section_id %q: %w", errMalformedRef, parts[2], err)
		}
		nodeRef.SectionID = &secID
	}
	return nodeRef, nil
}

// nodeRefToWire maps a graph.NodeRef to its wire form.
func nodeRefToWire(ref graph.NodeRef) NodeRefWire {
	return NodeRefWire{
		CorpusID:   ref.CorpusID,
		DocumentID: ref.DocumentID.String(),
		SectionID:  uuidPtrToWire(ref.SectionID),
	}
}

// mapEdges maps graph.Edge rows to their wire form, attaching each edge's
// evidence rows from evidence (store.NodeView.Evidence, keyed by edge id).
// Always returns a non-nil slice (even for zero edges) so the output
// marshals to `[]`, never `null` — mirrors pkg/mcp's mapping convention
// (tools.go).
func mapEdges(edges []graph.Edge, evidence map[uuid.UUID][]graph.Evidence) []EdgeWire {
	out := make([]EdgeWire, len(edges))
	for i, e := range edges {
		out[i] = EdgeWire{
			ID:         e.ID.String(),
			From:       nodeRefToWire(e.From),
			ToRefID:    e.ToRefID.String(),
			ToCorpusID: e.ToCorpusID,
			EdgeType:   string(e.EdgeType),
			Direction:  e.Direction,
			Promoted:   e.Promoted,
			AccessTier: string(e.AccessTier),
			CreatedAt:  e.CreatedAt.Format(time.RFC3339),
			Evidence:   mapEvidence(evidence[e.ID]),
		}
	}
	return out
}

// mapEvidence maps graph.Evidence rows to their wire form. Always returns a
// non-nil slice.
func mapEvidence(ev []graph.Evidence) []EvidenceWire {
	out := make([]EvidenceWire, len(ev))
	for i, e := range ev {
		out[i] = EvidenceWire{
			ID:             e.ID.String(),
			EvidenceKind:   string(e.EvidenceKind),
			Confidence:     e.Confidence,
			GroundingScore: e.GroundingScore,
			Rationale:      e.Rationale,
			QuotedFromSpan: e.QuotedFromSpan,
			QuotedToSpan:   e.QuotedToSpan,
			Model:          e.Model,
			PromptHash:     e.PromptHash,
			CreatedBy:      e.CreatedBy,
			PromotedBy:     e.PromotedBy,
			RunID:          uuidPtrToWire(e.RunID),
			PromotedAt:     formatTimePtr(e.PromotedAt),
			CreatedAt:      e.CreatedAt.Format(time.RFC3339),
		}
	}
	return out
}

// mapHops maps store.Hop rows to their wire form. Always returns a non-nil
// slice.
func mapHops(hops []store.Hop) []HopWire {
	out := make([]HopWire, len(hops))
	for i, h := range hops {
		out[i] = HopWire{
			Ref:            nodeRefToWire(h.Ref),
			EdgeType:       h.EdgeType,
			CorpusID:       h.CorpusID,
			Citation:       h.Citation,
			Text:           h.Text,
			Promoted:       h.Promoted,
			Confidence:     h.Confidence,
			GroundingScore: h.GroundingScore,
		}
	}
	return out
}

// uuidPtrToWire formats id as a string pointer, or nil when id is nil.
func uuidPtrToWire(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

// formatTimePtr formats t as RFC 3339, or returns nil when t is nil —
// mirrors pkg/mcp's formatTimePtr (tools.go).
func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.RFC3339)
	return &s
}
