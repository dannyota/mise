package httpapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"danny.vn/mise/pkg/store"
)

// GraphCanvasRepoIface is the whole-graph canvas endpoint's dependency —
// satisfied by *store.CanvasStore — narrowed to the exact methods the handler
// needs, consumer-defined per CODE_STYLE_GO.
type GraphCanvasRepoIface interface {
	GetCanvas(ctx context.Context, role string, limit int) (store.CanvasView, error)
}

// RestGraphNodeWire is the wire form of a canvas node.
type RestGraphNodeWire struct {
	ID         string `json:"id"`
	CorpusID   string `json:"corpus_id"`
	DocumentID string `json:"document_id"`
	Label      string `json:"label"`
	Tier       string `json:"tier"`
	NodeType   string `json:"node_type"`
}

// RestGraphEdgeWire is the wire form of a canvas edge.
type RestGraphEdgeWire struct {
	ID             string  `json:"id"`
	Source         string  `json:"source"`
	Target         string  `json:"target"`
	EdgeType       string  `json:"edge_type"`
	Confidence     float64 `json:"confidence"`
	GroundingScore float64 `json:"grounding_score"`
	Promoted       bool    `json:"promoted"`
}

// GraphCanvasInput is GET /graph's input.
type GraphCanvasInput struct {
	Limit int `query:"limit" doc:"Max edges (1-500, default 500)" example:"500"`
}

// GraphCanvasOutput is GET /graph's output.
type GraphCanvasOutput struct {
	Body struct {
		Nodes     []RestGraphNodeWire `json:"nodes"`
		Edges     []RestGraphEdgeWire `json:"edges"`
		Truncated bool                `json:"truncated,omitempty"`
	}
}

// RegisterGraphCanvas mounts the whole-graph canvas endpoint.
func RegisterGraphCanvas(api huma.API, repo GraphCanvasRepoIface, role string) {
	huma.Register(api, huma.Operation{
		OperationID: "get-graph-canvas",
		Method:      http.MethodGet,
		Path:        "/graph",
		Summary:     "Get the whole-graph canvas (nodes + edges)",
		Tags:        []string{"Graph"},
	}, newGraphCanvasHandler(repo, role))
}

func newGraphCanvasHandler(
	repo GraphCanvasRepoIface, role string,
) func(context.Context, *GraphCanvasInput) (*GraphCanvasOutput, error) {
	return func(ctx context.Context, in *GraphCanvasInput) (*GraphCanvasOutput, error) {
		limit := in.Limit
		if limit <= 0 || limit > 500 {
			limit = 500
		}

		var view store.CanvasView
		if repo != nil {
			var err error
			view, err = repo.GetCanvas(ctx, role, limit)
			if err != nil {
				return nil, fmt.Errorf("httpapi: getting graph canvas: %w", err)
			}
		}

		out := &GraphCanvasOutput{}
		out.Body.Nodes = mapCanvasNodes(view.Nodes)
		out.Body.Edges = mapCanvasEdges(view.Edges)
		out.Body.Truncated = view.Truncated
		return out, nil
	}
}

// mapCanvasNodes maps store.CanvasNode rows to their wire form. Always
// returns a non-nil slice.
func mapCanvasNodes(nodes []store.CanvasNode) []RestGraphNodeWire {
	out := make([]RestGraphNodeWire, len(nodes))
	for i, n := range nodes {
		out[i] = RestGraphNodeWire{
			ID:         n.ID.String(),
			CorpusID:   n.CorpusID,
			DocumentID: n.DocumentID.String(),
			Label:      n.Label,
			Tier:       n.Tier,
			NodeType:   n.NodeType,
		}
	}
	return out
}

// mapCanvasEdges maps store.CanvasEdge rows to their wire form. Always
// returns a non-nil slice.
func mapCanvasEdges(edges []store.CanvasEdge) []RestGraphEdgeWire {
	out := make([]RestGraphEdgeWire, len(edges))
	for i, e := range edges {
		out[i] = RestGraphEdgeWire{
			ID:             e.ID.String(),
			Source:         e.Source.String(),
			Target:         e.Target.String(),
			EdgeType:       e.EdgeType,
			Confidence:     e.Confidence,
			GroundingScore: e.GroundingScore,
			Promoted:       e.Promoted,
		}
	}
	return out
}
