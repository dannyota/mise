package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/store"
)

// TestParseRef covers parseRef's whole contract: the two valid shapes
// (corpus_id/document_id and .../section_id), and every malformed input —
// missing/extra segments, empty segments, and non-UUID document_id/
// section_id — that must report errMalformedRef rather than silently
// guessing (RISKS R6-style: drop or reject, never guess).
func TestParseRef(t *testing.T) {
	docID := uuid.New()
	secID := uuid.New()

	tests := []struct {
		name    string
		ref     string
		want    graph.NodeRef
		wantErr bool
	}{
		{
			name: "corpus and document",
			ref:  "vn-reg/" + docID.String(),
			want: graph.NodeRef{CorpusID: "vn-reg", DocumentID: docID},
		},
		{
			name: "corpus, document, and section",
			ref:  "vn-reg/" + docID.String() + "/" + secID.String(),
			want: graph.NodeRef{CorpusID: "vn-reg", DocumentID: docID, SectionID: &secID},
		},
		{name: "empty ref", ref: "", wantErr: true},
		{name: "single segment, no slash", ref: "vn-reg", wantErr: true},
		{name: "empty corpus segment", ref: "/" + docID.String(), wantErr: true},
		{name: "empty document segment", ref: "vn-reg/", wantErr: true},
		{name: "malformed document uuid", ref: "vn-reg/not-a-uuid", wantErr: true},
		{name: "empty section segment", ref: "vn-reg/" + docID.String() + "/", wantErr: true},
		{name: "malformed section uuid", ref: "vn-reg/" + docID.String() + "/not-a-uuid", wantErr: true},
		{name: "too many segments", ref: "vn-reg/" + docID.String() + "/" + secID.String() + "/extra", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRef(tt.ref)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseRef(%q) error = nil, want an error", tt.ref)
				}
				if !errors.Is(err, errMalformedRef) {
					t.Errorf("parseRef(%q) error = %v, want errMalformedRef in chain", tt.ref, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRef(%q) error = %v, want nil", tt.ref, err)
			}
			if got.CorpusID != tt.want.CorpusID || got.DocumentID != tt.want.DocumentID {
				t.Errorf("parseRef(%q) = %+v, want %+v", tt.ref, got, tt.want)
			}
			switch {
			case tt.want.SectionID == nil && got.SectionID != nil:
				t.Errorf("parseRef(%q) SectionID = %v, want nil", tt.ref, *got.SectionID)
			case tt.want.SectionID != nil && (got.SectionID == nil || *got.SectionID != *tt.want.SectionID):
				t.Errorf("parseRef(%q) SectionID = %v, want %v", tt.ref, got.SectionID, *tt.want.SectionID)
			}
		})
	}
}

// TestMapEdgesAndHopsAlwaysNonNil pins the "never null" wire convention
// (mirrors pkg/mcp's mapSectionHits doc comment): a nil/empty input must
// still marshal to `[]`, never `null`, so web/reasoning consumers never need
// a null-check before ranging over these arrays.
func TestMapEdgesAndHopsAlwaysNonNil(t *testing.T) {
	if edges := mapEdges(nil, nil); edges == nil {
		t.Error("mapEdges(nil, nil) = nil, want a non-nil empty slice")
	}
	if ev := mapEvidence(nil); ev == nil {
		t.Error("mapEvidence(nil) = nil, want a non-nil empty slice")
	}
	if hops := mapHops(nil); hops == nil {
		t.Error("mapHops(nil) = nil, want a non-nil empty slice")
	}

	data, err := json.Marshal(mapEdges(nil, nil))
	if err != nil {
		t.Fatalf("marshaling mapEdges(nil, nil): %v", err)
	}
	if string(data) != "[]" {
		t.Errorf("json.Marshal(mapEdges(nil, nil)) = %s, want []", data)
	}
}

// fakeGraphRepo is GraphRepoIface's unit-test double: it records the last
// role/ref/maxDepth a handler passed through, and returns whatever the test
// configured — so Register's HTTP-level mapping (parseRef -> repo call ->
// wire shape / error status) is exercised with no database, exactly like
// pkg/store/graph_chain_test.go's fakeChainSource does for walkChain.
type fakeGraphRepo struct {
	nodeView store.NodeView
	nodeErr  error
	hops     []store.Hop
	chainErr error

	gotRole     string
	gotRef      graph.NodeRef
	gotMaxDepth int
}

func (f *fakeGraphRepo) GetNode(_ context.Context, role string, ref graph.NodeRef) (store.NodeView, error) {
	f.gotRole, f.gotRef = role, ref
	return f.nodeView, f.nodeErr
}

func (f *fakeGraphRepo) Chain(_ context.Context, role string, start graph.NodeRef, maxDepth int) ([]store.Hop, error) {
	f.gotRole, f.gotRef, f.gotMaxDepth = role, start, maxDepth
	return f.hops, f.chainErr
}

// newTestServer wires repo into a real chi + huma API (Register, NewAPI —
// the exact production construction) and starts an httptest.Server, so
// these tests exercise the whole huma/chi/JSON pipeline — routing,
// percent-decoding, status-code mapping, RFC 9457 error bodies — not a
// reimplementation of it.
func newTestServer(t *testing.T, repo GraphRepoIface, role string) *httptest.Server {
	t.Helper()
	router := chi.NewRouter()
	api := NewAPI(router)
	Register(api, repo, role)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

// encodeRef builds the {ref} path segment for ref, percent-encoding the "/"
// separators as %2F — the form a real client must send (verified against
// humachi/chi: a raw "/" would split into extra path segments and 404
// before ever reaching the handler; humachi's chiContext.Param decodes the
// still-escaped segment back to its literal form, which is the only reason
// a single {ref} path param can carry a compound value at all).
func encodeRef(ref graph.NodeRef) string {
	s := ref.CorpusID + "/" + ref.DocumentID.String()
	if ref.SectionID != nil {
		s += "/" + ref.SectionID.String()
	}
	return url.PathEscape(s)
}

// getJSON issues a GET to srv.URL+path and returns the status, content type,
// and raw body.
func getJSON(t *testing.T, srv *httptest.Server, path string) (int, string, []byte) {
	t.Helper()
	resp, err := http.Get(srv.URL + path) //nolint:noctx // test helper; url is a local httptest.Server address
	if err != nil {
		t.Fatalf("GET %s error = %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("GET %s: decoding body: %v", path, err)
	}
	return resp.StatusCode, resp.Header.Get("Content-Type"), body
}

// TestNodeEndpointMapsEdgesAndEvidence is the node handler's headline
// mapping test: a fake NodeView with one edge and one evidence row must
// round-trip through real HTTP into exactly the wire shape mapEdges/
// mapEvidence describe — and the fake must have received the exact role
// Register was given and the exact NodeRef parseRef decoded from the
// percent-encoded path.
func TestNodeEndpointMapsEdgesAndEvidence(t *testing.T) {
	fromRef := graph.NodeRef{CorpusID: "group-std", DocumentID: uuid.New()}
	toRefID := uuid.New()
	edgeID := uuid.New()
	createdAt := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	runID := uuid.New()

	repo := &fakeGraphRepo{
		nodeView: store.NodeView{
			Ref: fromRef,
			Edges: []graph.Edge{{
				ID: edgeID, From: fromRef, ToRefID: toRefID, ToCorpusID: "my-reg",
				EdgeType: graph.EdgeSatisfies, Direction: "up", Promoted: true,
				AccessTier: graph.TierGroupConfidential, CreatedAt: createdAt,
			}},
			Evidence: map[uuid.UUID][]graph.Evidence{
				edgeID: {{
					ID: uuid.New(), EdgeID: edgeID, EvidenceKind: graph.EvidenceModelClassification,
					Confidence: 0.87, GroundingScore: 0.92, Rationale: "matches Điều 7",
					RunID: &runID, CreatedAt: createdAt,
				}},
			},
		},
	}

	srv := newTestServer(t, repo, "mise_group")
	status, ct, body := getJSON(t, srv, "/graph/nodes/"+encodeRef(fromRef))

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, body)
	}
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if repo.gotRole != "mise_group" {
		t.Errorf("repo received role = %q, want mise_group", repo.gotRole)
	}
	if repo.gotRef != fromRef {
		t.Errorf("repo received ref = %+v, want %+v", repo.gotRef, fromRef)
	}

	var got NodeBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshaling NodeBody: %v; body: %s", err, body)
	}
	if got.Ref.CorpusID != fromRef.CorpusID || got.Ref.DocumentID != fromRef.DocumentID.String() {
		t.Errorf("Ref = %+v, want corpus %s document %s", got.Ref, fromRef.CorpusID, fromRef.DocumentID)
	}
	if len(got.Edges) != 1 {
		t.Fatalf("Edges = %d, want 1", len(got.Edges))
	}
	edge := got.Edges[0]
	if edge.ID != edgeID.String() || edge.ToRefID != toRefID.String() || edge.ToCorpusID != "my-reg" {
		t.Errorf("Edges[0] = %+v, want id %s to_ref_id %s to_corpus_id my-reg", edge, edgeID, toRefID)
	}
	if edge.EdgeType != "satisfies" || edge.Direction != "up" || !edge.Promoted {
		t.Errorf("Edges[0] type/direction/promoted = %s/%s/%v, want satisfies/up/true",
			edge.EdgeType, edge.Direction, edge.Promoted)
	}
	if edge.AccessTier != "group-confidential" {
		t.Errorf("Edges[0].AccessTier = %q, want group-confidential", edge.AccessTier)
	}
	if len(edge.Evidence) != 1 {
		t.Fatalf("Edges[0].Evidence = %d, want 1", len(edge.Evidence))
	}
	ev := edge.Evidence[0]
	if ev.EvidenceKind != "model_classification" || ev.Confidence != 0.87 || ev.GroundingScore != 0.92 {
		t.Errorf("Evidence[0] = %+v, want kind model_classification confidence 0.87 grounding 0.92", ev)
	}
	if ev.RunID == nil || *ev.RunID != runID.String() {
		t.Errorf("Evidence[0].RunID = %v, want %s", ev.RunID, runID)
	}
}

// TestNodeEndpointSectionScopedRefRoundTrips proves the 3-segment
// corpus_id/document_id/section_id ref form survives percent-encoding,
// routing, and parseRef intact — the section_id the fake receives must
// match exactly what was encoded.
func TestNodeEndpointSectionScopedRefRoundTrips(t *testing.T) {
	secID := uuid.New()
	ref := graph.NodeRef{CorpusID: "vn-reg", DocumentID: uuid.New(), SectionID: &secID}
	repo := &fakeGraphRepo{nodeView: store.NodeView{Ref: ref}}

	srv := newTestServer(t, repo, "mise_public")
	status, _, _ := getJSON(t, srv, "/graph/nodes/"+encodeRef(ref))

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if repo.gotRef.SectionID == nil || *repo.gotRef.SectionID != secID {
		t.Errorf("repo received SectionID = %v, want %s", repo.gotRef.SectionID, secID)
	}
}

// TestNodeEndpointNotFoundReturns404 is the not-found/RLS-denied contract:
// GraphRepoIface reports store.ErrNodeNotFound for both causes
// indistinguishably, and Register must map that — and only that — to a
// plain 404 RFC 9457 problem, not a 500.
func TestNodeEndpointNotFoundReturns404(t *testing.T) {
	repo := &fakeGraphRepo{nodeErr: store.ErrNodeNotFound}
	srv := newTestServer(t, repo, "mise_public")

	ref := graph.NodeRef{CorpusID: "vn-reg", DocumentID: uuid.New()}
	status, ct, body := getJSON(t, srv, "/graph/nodes/"+encodeRef(ref))

	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", status, body)
	}
	if ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
	var problem struct {
		Status int    `json:"status"`
		Title  string `json:"title"`
	}
	if err := json.Unmarshal(body, &problem); err != nil {
		t.Fatalf("unmarshaling problem+json: %v; body: %s", err, body)
	}
	if problem.Status != http.StatusNotFound {
		t.Errorf("problem.status = %d, want 404", problem.Status)
	}
}

// TestNodeEndpointMalformedRefReturns400 covers a handful of malformed refs
// that reach the handler over real HTTP (i.e. they still route as one path
// segment) but fail parseRef — these must report 400, never fold into the
// not-found 404.
func TestNodeEndpointMalformedRefReturns400(t *testing.T) {
	validID := uuid.New().String()
	tests := []struct {
		name string
		ref  string // pre-escaped; sent verbatim after /graph/nodes/
	}{
		{name: "single segment", ref: "onlycorpus"},
		{name: "non-uuid document id", ref: "vn-reg%2Fnot-a-uuid"},
		{name: "trailing empty section", ref: "vn-reg%2F" + validID + "%2F"},
	}

	repo := &fakeGraphRepo{nodeView: store.NodeView{}}
	srv := newTestServer(t, repo, "mise_public")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, ct, body := getJSON(t, srv, "/graph/nodes/"+tt.ref)
			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", status, body)
			}
			if ct != "application/problem+json" {
				t.Errorf("Content-Type = %q, want application/problem+json", ct)
			}
		})
	}
}

// TestChainEndpointMapsHopsAndMaxDepth is the chain handler's headline
// mapping test: a fake hop list must round-trip into exactly the wire shape
// mapHops describes, and max_depth must reach the repo call unchanged
// (store.Chain itself owns clamping — store/graph_chain.go's clampDepth —
// so Register must not second-guess it).
func TestChainEndpointMapsHopsAndMaxDepth(t *testing.T) {
	hopRef := graph.NodeRef{CorpusID: "my-reg", DocumentID: uuid.New()}
	repo := &fakeGraphRepo{hops: []store.Hop{{
		Ref: hopRef, EdgeType: "satisfies", CorpusID: "my-reg", Citation: "s. 143(1)",
		Text: "verbatim law text", Promoted: true, Confidence: 0.95, GroundingScore: 0.9,
	}}}

	srv := newTestServer(t, repo, "mise_local")
	start := graph.NodeRef{CorpusID: "local-sop", DocumentID: uuid.New()}
	status, _, body := getJSON(t, srv, "/graph/chain/"+encodeRef(start)+"?max_depth=3")

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, body)
	}
	if repo.gotMaxDepth != 3 {
		t.Errorf("repo received maxDepth = %d, want 3", repo.gotMaxDepth)
	}
	if repo.gotRole != "mise_local" {
		t.Errorf("repo received role = %q, want mise_local", repo.gotRole)
	}

	var got ChainBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshaling ChainBody: %v; body: %s", err, body)
	}
	if len(got.Hops) != 1 {
		t.Fatalf("Hops = %d, want 1", len(got.Hops))
	}
	hop := got.Hops[0]
	if hop.Ref.CorpusID != "my-reg" || hop.Citation != "s. 143(1)" || hop.Text != "verbatim law text" {
		t.Errorf("Hops[0] = %+v, want corpus my-reg citation \"s. 143(1)\" text \"verbatim law text\"", hop)
	}
	if !hop.Promoted || hop.Confidence != 0.95 || hop.GroundingScore != 0.9 {
		t.Errorf("Hops[0] promoted/confidence/grounding = %v/%v/%v, want true/0.95/0.9",
			hop.Promoted, hop.Confidence, hop.GroundingScore)
	}
}

// TestChainEndpointDefaultMaxDepthIsZero verifies an omitted max_depth
// reaches the repo as Go's int zero value — store.Chain's own clampDepth
// treats <=0 as "use the default cap" (store/graph_chain.go), so Register
// must pass it through unchanged rather than substituting a value itself.
func TestChainEndpointDefaultMaxDepthIsZero(t *testing.T) {
	repo := &fakeGraphRepo{}
	srv := newTestServer(t, repo, "mise_public")
	start := graph.NodeRef{CorpusID: "local-sop", DocumentID: uuid.New()}

	status, _, body := getJSON(t, srv, "/graph/chain/"+encodeRef(start))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, body)
	}
	if repo.gotMaxDepth != 0 {
		t.Errorf("repo received maxDepth = %d, want 0 (omitted)", repo.gotMaxDepth)
	}
}

// TestChainEndpointNotFoundReturns404 is defensive: the real
// *store.GraphRepo.Chain never itself returns store.ErrNodeNotFound (a hop
// role can't see just ends the walk with fewer hops, store/graph_chain.go),
// but GraphRepoIface is an interface — a fake or a future implementation
// could — so Register's mapping must still be correct for that case.
func TestChainEndpointNotFoundReturns404(t *testing.T) {
	repo := &fakeGraphRepo{chainErr: store.ErrNodeNotFound}
	srv := newTestServer(t, repo, "mise_public")
	start := graph.NodeRef{CorpusID: "local-sop", DocumentID: uuid.New()}

	status, ct, body := getJSON(t, srv, "/graph/chain/"+encodeRef(start))
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", status, body)
	}
	if ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
}

// TestChainEndpointZeroHopsReturns200 pins the "0 hops is a normal answer,
// not a 404" contract (store/graph_chain.go's Chain doc comment: a hop role
// can't see, or that doesn't exist, "ends the walk cleanly, not an error") —
// e.g. a law document that is already the top of its chain.
func TestChainEndpointZeroHopsReturns200(t *testing.T) {
	repo := &fakeGraphRepo{hops: []store.Hop{}}
	srv := newTestServer(t, repo, "mise_public")
	start := graph.NodeRef{CorpusID: "my-reg", DocumentID: uuid.New()}

	status, _, body := getJSON(t, srv, "/graph/chain/"+encodeRef(start))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, body)
	}

	var got ChainBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshaling ChainBody: %v; body: %s", err, body)
	}
	if got.Hops == nil || len(got.Hops) != 0 {
		t.Errorf("Hops = %v, want a non-nil empty slice", got.Hops)
	}
}
