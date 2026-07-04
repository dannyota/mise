package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"danny.vn/mise/pkg/store"
)

type fakeCanvasRepo struct {
	view    store.CanvasView
	viewErr error

	gotRole  string
	gotLimit int
}

func (f *fakeCanvasRepo) GetCanvas(_ context.Context, role string, limit int) (store.CanvasView, error) {
	f.gotRole, f.gotLimit = role, limit
	return f.view, f.viewErr
}

func newCanvasTestServer(t *testing.T, repo GraphCanvasRepoIface, role string) *httptest.Server {
	t.Helper()
	router := chi.NewRouter()
	api := NewAPI(router)
	RegisterGraphCanvas(api, repo, role)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

func TestGraphCanvasMapsNodesAndEdges(t *testing.T) {
	t.Parallel()
	nodeID := uuid.New()
	docID := uuid.New()
	edgeID := uuid.New()
	sourceID := uuid.New()
	targetID := uuid.New()

	repo := &fakeCanvasRepo{
		view: store.CanvasView{
			Nodes: []store.CanvasNode{{
				ID: nodeID, CorpusID: "vn-reg", DocumentID: docID,
				Label: "Circular 09", Tier: "public", NodeType: "law",
			}},
			Edges: []store.CanvasEdge{{
				ID: edgeID, Source: sourceID, Target: targetID,
				EdgeType: "satisfies", Confidence: 0.88, GroundingScore: 0.92,
				Promoted: true,
			}},
			Truncated: false,
		},
	}

	srv := newCanvasTestServer(t, repo, "mise_group")
	status, ct, body := getJSON(t, srv, "/graph?limit=100")

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, body)
	}
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if repo.gotRole != "mise_group" {
		t.Errorf("repo received role = %q, want mise_group", repo.gotRole)
	}
	if repo.gotLimit != 100 {
		t.Errorf("repo received limit = %d, want 100", repo.gotLimit)
	}

	var got struct {
		Nodes     []RestGraphNodeWire `json:"nodes"`
		Edges     []RestGraphEdgeWire `json:"edges"`
		Truncated bool                `json:"truncated"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshaling: %v; body: %s", err, body)
	}
	if len(got.Nodes) != 1 {
		t.Fatalf("Nodes = %d, want 1", len(got.Nodes))
	}
	if got.Nodes[0].ID != nodeID.String() || got.Nodes[0].Label != "Circular 09" {
		t.Errorf("Nodes[0] = %+v, want id %s label Circular 09", got.Nodes[0], nodeID)
	}
	if len(got.Edges) != 1 {
		t.Fatalf("Edges = %d, want 1", len(got.Edges))
	}
	if got.Edges[0].ID != edgeID.String() || got.Edges[0].EdgeType != "satisfies" {
		t.Errorf("Edges[0] = %+v, want id %s edge_type satisfies", got.Edges[0], edgeID)
	}
	if got.Edges[0].Confidence != 0.88 || got.Edges[0].GroundingScore != 0.92 {
		t.Errorf("Edges[0] confidence/grounding = %v/%v, want 0.88/0.92",
			got.Edges[0].Confidence, got.Edges[0].GroundingScore)
	}
	if got.Truncated {
		t.Error("Truncated = true, want false")
	}
}

func TestGraphCanvasTruncatedFlag(t *testing.T) {
	t.Parallel()
	repo := &fakeCanvasRepo{
		view: store.CanvasView{Truncated: true},
	}

	srv := newCanvasTestServer(t, repo, "mise_public")
	status, _, body := getJSON(t, srv, "/graph")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, body)
	}

	var got struct {
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshaling: %v; body: %s", err, body)
	}
	if !got.Truncated {
		t.Error("Truncated = false, want true")
	}
}

func TestGraphCanvasDefaultLimit(t *testing.T) {
	t.Parallel()
	repo := &fakeCanvasRepo{}
	srv := newCanvasTestServer(t, repo, "mise_public")

	status, _, body := getJSON(t, srv, "/graph")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, body)
	}
	// Omitted limit (0) should be clamped to 500 by the handler.
	if repo.gotLimit != 500 {
		t.Errorf("repo received limit = %d, want 500 (default)", repo.gotLimit)
	}
}

func TestMapCanvasNodesAndEdgesNonNil(t *testing.T) {
	t.Parallel()
	nodes := mapCanvasNodes(nil)
	if nodes == nil {
		t.Error("mapCanvasNodes(nil) = nil, want non-nil empty slice")
	}
	edges := mapCanvasEdges(nil)
	if edges == nil {
		t.Error("mapCanvasEdges(nil) = nil, want non-nil empty slice")
	}

	data, _ := json.Marshal(nodes)
	if string(data) != "[]" {
		t.Errorf("json.Marshal(mapCanvasNodes(nil)) = %s, want []", data)
	}
	data, _ = json.Marshal(edges)
	if string(data) != "[]" {
		t.Errorf("json.Marshal(mapCanvasEdges(nil)) = %s, want []", data)
	}
}
