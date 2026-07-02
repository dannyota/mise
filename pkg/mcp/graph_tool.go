package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/store"
)

// Direction values the graph tool's direction input accepts (API-CONTRACT
// §2). directionUp is the tool's default when direction is omitted, and the
// only value graph.relation_edge.direction ever holds today (mirrors
// store's own directionUp const, graph_chain.go); directionDown is accepted
// for forward compatibility with the documented contract and simply
// filters GetNode's edges down to none until a "down" edge is ever written.
const (
	directionUp   = "up"
	directionDown = "down"
)

// GraphRepoIface is the graph tool's read-side dependency — satisfied by
// *store.GraphRepo (see cmd/serving's wiring). Named with an Iface suffix
// (unlike Searcher/DocGetter) since GraphRepo already names the concrete
// store type this interface is a read-only mirror of.
type GraphRepoIface interface {
	GetNode(ctx context.Context, role string, ref graph.NodeRef) (store.NodeView, error)
	Chain(ctx context.Context, role string, start graph.NodeRef, maxDepth int) ([]store.Hop, error)
}

// GraphInput is the graph tool's input (API-CONTRACT §2). NodeRef is the
// wire form "<corpus_id>/<document_id>[/<section_id>]"; Direction defaults
// to "up" when omitted; Depth defaults to store.MaxChainDepth when <= 0
// (mirroring Chain's own clampDepth default).
type GraphInput struct {
	NodeRef   string   `json:"node_ref"`
	Direction string   `json:"direction,omitempty"`
	EdgeTypes []string `json:"edge_types,omitempty"`
	Depth     int      `json:"depth,omitempty"`
}

// GraphOutput is the graph tool's output: the queried node, its tier-scoped
// outgoing edges (filtered by direction/edge_types), and the control chain
// walked from it (API-CONTRACT §2).
type GraphOutput struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
	Chain []GraphHop  `json:"chain"`
}

// GraphNode is one graph node reference — a corpus/document, optionally
// narrowed to a section (DATA-MODEL §4). Mirrors graph.NodeRef on the wire.
type GraphNode struct {
	CorpusID   string  `json:"corpus_id"`
	DocumentID string  `json:"document_id"`
	SectionID  *string `json:"section_id,omitempty"`
}

// GraphEdge is one graph.relation_edge row, wire form: the resolved "from"
// side, the unresolved doc_ref "to" side (ToRefID/ToCorpusID — GetNode
// doesn't resolve a doc_ref's own target document/section, see
// store.NodeView's doc comment), and the edge's best evidence (highest
// Confidence) contributing Confidence/GroundingScore — mirrors store.Hop's
// own "best evidence" convention (graph_chain.go).
type GraphEdge struct {
	ID             string  `json:"id"`
	FromCorpusID   string  `json:"from_corpus_id"`
	FromDocumentID string  `json:"from_document_id"`
	FromSectionID  *string `json:"from_section_id,omitempty"`
	ToRefID        string  `json:"to_ref_id"`
	ToCorpusID     string  `json:"to_corpus_id"`
	EdgeType       string  `json:"edge_type"`
	Direction      string  `json:"direction"`
	Promoted       bool    `json:"promoted"`
	AccessTier     string  `json:"access_tier"`
	Confidence     float64 `json:"confidence"`
	GroundingScore float64 `json:"grounding_score"`
	CreatedAt      string  `json:"created_at"`
}

// GraphHop is one control-chain hop — the wire form of store.Hop. Text is
// usually empty; see store.Hop's own doc comment (graph_chain.go) for why
// Chain deliberately leaves it blank.
type GraphHop struct {
	CorpusID       string  `json:"corpus_id"`
	DocumentID     string  `json:"document_id"`
	SectionID      *string `json:"section_id,omitempty"`
	EdgeType       string  `json:"edge_type"`
	Citation       string  `json:"citation"`
	Text           string  `json:"text"`
	Promoted       bool    `json:"promoted"`
	Confidence     float64 `json:"confidence"`
	GroundingScore float64 `json:"grounding_score"`
}

// registerGraphTool binds repo/role into the graph tool's handler and
// registers it on srv via the SDK's typed mcp.AddTool, which derives the
// tool's JSON input/output schema from the structs above.
func registerGraphTool(srv *mcp.Server, repo GraphRepoIface, role string) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "graph",
		Description: "Compliance graph read: a node's tier-scoped edges (confidence/" +
			"grounding_score/promoted) plus the control chain (SOP -> Policy -> Group -> " +
			"law) walked from it.",
	}, newGraphHandler(repo, role))
}

// newGraphHandler returns the graph tool's typed handler, closed over repo
// and role.
func newGraphHandler(repo GraphRepoIface, role string) mcp.ToolHandlerFor[GraphInput, GraphOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in GraphInput) (*mcp.CallToolResult, GraphOutput, error) {
		ref, direction, depth, err := graphOptsFromInput(in)
		if err != nil {
			return nil, GraphOutput{}, err
		}

		view, err := repo.GetNode(ctx, role, ref)
		if err != nil {
			return nil, GraphOutput{}, fmt.Errorf("mcp graph: %w", err)
		}

		hops, err := repo.Chain(ctx, role, ref, depth)
		if err != nil {
			return nil, GraphOutput{}, fmt.Errorf("mcp graph: %w", err)
		}

		return nil, GraphOutput{
			Nodes: []GraphNode{mapGraphNode(view.Ref)},
			Edges: mapGraphEdges(view.Edges, view.Evidence, direction, in.EdgeTypes),
			Chain: mapGraphHops(hops),
		}, nil
	}
}

// graphOptsFromInput validates in.NodeRef (parseNodeRef) and in.Direction,
// and applies the graph tool's defaults: direction "up", depth
// store.MaxChainDepth — mirrors searchOptsFromInput's validate-before-dep
// shape (tools.go).
func graphOptsFromInput(in GraphInput) (ref graph.NodeRef, direction string, depth int, err error) {
	ref, err = parseNodeRef(in.NodeRef)
	if err != nil {
		return graph.NodeRef{}, "", 0, err
	}

	direction = in.Direction
	if direction == "" {
		direction = directionUp
	}
	if direction != directionUp && direction != directionDown {
		return graph.NodeRef{}, "", 0, fmt.Errorf(
			"mcp graph: invalid direction %q, want %q or %q", direction, directionUp, directionDown)
	}

	depth = in.Depth
	if depth <= 0 {
		depth = store.MaxChainDepth
	}
	return ref, direction, depth, nil
}

// parseNodeRef parses raw — the wire node_ref "<corpus_id>/<document_id>
// [/<section_id>]" — into a graph.NodeRef, validating corpus_id against the
// registry and document_id/section_id as UUIDs. A malformed shape/UUID and
// a well-formed but unregistered corpus_id are deliberately distinct error
// cases (both tool errors, but callers/tests can tell them apart).
func parseNodeRef(raw string) (graph.NodeRef, error) {
	parts := strings.Split(raw, "/")
	if len(parts) < 2 || len(parts) > 3 {
		return graph.NodeRef{}, fmt.Errorf(
			`mcp graph: invalid node_ref %q, want "<corpus_id>/<document_id>[/<section_id>]"`, raw)
	}

	corpusID := parts[0]
	if _, ok := corpus.Get(corpus.ID(corpusID)); !ok {
		return graph.NodeRef{}, fmt.Errorf("mcp graph: %q is not a registered corpus", corpusID)
	}

	docID, err := uuid.Parse(parts[1])
	if err != nil {
		return graph.NodeRef{}, fmt.Errorf("mcp graph: invalid node_ref %q: bad document_id: %w", raw, err)
	}

	ref := graph.NodeRef{CorpusID: corpusID, DocumentID: docID}
	if len(parts) == 3 {
		secID, err := uuid.Parse(parts[2])
		if err != nil {
			return graph.NodeRef{}, fmt.Errorf("mcp graph: invalid node_ref %q: bad section_id: %w", raw, err)
		}
		ref.SectionID = &secID
	}
	return ref, nil
}

// mapGraphNode maps a graph.NodeRef to its wire form.
func mapGraphNode(ref graph.NodeRef) GraphNode {
	return GraphNode{
		CorpusID:   ref.CorpusID,
		DocumentID: ref.DocumentID.String(),
		SectionID:  sectionIDPtr(ref.SectionID),
	}
}

// mapGraphEdges maps edges to their wire form, keeping only those matching
// direction and (when non-empty) edgeTypes, and attaching each edge's best
// evidence (bestGraphEvidence) as Confidence/GroundingScore. This filtering
// applies only here — Chain's walk (mapGraphHops) has no equivalent
// parameter at the store layer (graph_chain.go's Chain signature takes no
// edge-type filter), so the chain is always the unfiltered up-direction
// walk. Always returns a non-nil slice.
func mapGraphEdges(
	edges []graph.Edge, evidence map[uuid.UUID][]graph.Evidence, direction string, edgeTypes []string,
) []GraphEdge {
	want := toSet(edgeTypes)
	out := make([]GraphEdge, 0, len(edges))
	for _, e := range edges {
		if e.Direction != direction {
			continue
		}
		if want != nil && !want[string(e.EdgeType)] {
			continue
		}
		best := bestGraphEvidence(evidence[e.ID])
		out = append(out, GraphEdge{
			ID:             e.ID.String(),
			FromCorpusID:   e.From.CorpusID,
			FromDocumentID: e.From.DocumentID.String(),
			FromSectionID:  sectionIDPtr(e.From.SectionID),
			ToRefID:        e.ToRefID.String(),
			ToCorpusID:     e.ToCorpusID,
			EdgeType:       string(e.EdgeType),
			Direction:      e.Direction,
			Promoted:       e.Promoted,
			AccessTier:     string(e.AccessTier),
			Confidence:     best.Confidence,
			GroundingScore: best.GroundingScore,
			CreatedAt:      e.CreatedAt.Format(time.RFC3339),
		})
	}
	return out
}

// mapGraphHops maps Chain's ordered hops to their wire form. Always returns
// a non-nil slice.
func mapGraphHops(hops []store.Hop) []GraphHop {
	out := make([]GraphHop, len(hops))
	for i, h := range hops {
		out[i] = GraphHop{
			CorpusID:       h.Ref.CorpusID,
			DocumentID:     h.Ref.DocumentID.String(),
			SectionID:      sectionIDPtr(h.Ref.SectionID),
			EdgeType:       h.EdgeType,
			Citation:       h.Citation,
			Text:           h.Text,
			Promoted:       h.Promoted,
			Confidence:     h.Confidence,
			GroundingScore: h.GroundingScore,
		}
	}
	return out
}

// bestGraphEvidence returns ev's highest-Confidence row, mirroring
// store.bestEvidence (graph_chain.go, unexported there) — GraphEdge's
// Confidence/GroundingScore come from "the edge's best evidence." Seeding
// best from ev[0] (not a zero graph.Evidence) matters when a real row's own
// Confidence is exactly 0 — see store.bestEvidence's own doc comment.
func bestGraphEvidence(ev []graph.Evidence) graph.Evidence {
	if len(ev) == 0 {
		return graph.Evidence{}
	}
	best := ev[0]
	for _, e := range ev[1:] {
		if e.Confidence > best.Confidence {
			best = e
		}
	}
	return best
}

// sectionIDPtr formats id as a string pointer, or nil when id is nil — the
// wire form of a NodeRef's optional SectionID (mirrors formatTimePtr,
// tools.go).
func sectionIDPtr(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

// toSet returns vals as a membership set, or nil when vals is empty — nil
// means "no filter" to mapGraphEdges, distinct from a set that happens to
// match nothing.
func toSet(vals []string) map[string]bool {
	if len(vals) == 0 {
		return nil
	}
	set := make(map[string]bool, len(vals))
	for _, v := range vals {
		set[v] = true
	}
	return set
}
