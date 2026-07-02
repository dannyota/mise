//go:build integration

package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"danny.vn/mise/internal/testdb"
	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/httpapi"
	"danny.vn/mise/pkg/store"
)

// graphFixture seeds a real, 3-hop control chain — sopRef (local-sop)
// --derives--> policyRef (local-policy) --implements--> groupRef
// (group-std) --satisfies--> lawRef (my-reg) — mirroring
// pkg/store/graph_chain_integration_test.go's newGraphChainFixture, rebuilt
// locally (as raw SQL under the pool's owner role, bypassing RLS on write)
// since that file's unexported helpers aren't reachable from this package.
// sopEdgeID/sopEvidenceID additionally give TestGraphNodeEndpoint... a
// concrete edge+evidence pair to assert field-by-field against the REST
// wire shape.
type graphFixture struct {
	sopRef, policyRef, groupRef, lawRef graph.NodeRef
	sopEdgeID, sopEvidenceID            uuid.UUID
}

func newGraphFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) graphFixture {
	t.Helper()

	sopRef := graph.NodeRef{CorpusID: string(corpus.LocalSOP), DocumentID: uuid.New()}

	policyRefID := insertDocRef(t, ctx, pool, string(corpus.LocalPolicy), "local-policy:httpapi-"+uuid.NewString())
	groupRefID := insertDocRef(t, ctx, pool, string(corpus.GroupStd), "group-std:httpapi-"+uuid.NewString())
	lawRefID := insertDocRef(t, ctx, pool, string(corpus.MYReg), "my-reg:httpapi-"+uuid.NewString())

	policyDocID, groupDocID, lawDocID := uuid.New(), uuid.New(), uuid.New()
	resolveDocRef(t, ctx, pool, policyRefID, policyDocID)
	resolveDocRef(t, ctx, pool, groupRefID, groupDocID)
	resolveDocRef(t, ctx, pool, lawRefID, lawDocID)

	sopEdgeID := insertEdge(t, ctx, pool, string(corpus.LocalSOP), sopRef.DocumentID,
		string(corpus.LocalPolicy), "derives", policyRefID)
	sopEvidenceID := insertEvidence(t, ctx, pool, sopEdgeID)
	insertEdge(t, ctx, pool, string(corpus.LocalPolicy), policyDocID, string(corpus.GroupStd), "implements", groupRefID)
	insertEdge(t, ctx, pool, string(corpus.GroupStd), groupDocID, string(corpus.MYReg), "satisfies", lawRefID)

	return graphFixture{
		sopRef:        sopRef,
		policyRef:     graph.NodeRef{CorpusID: string(corpus.LocalPolicy), DocumentID: policyDocID},
		groupRef:      graph.NodeRef{CorpusID: string(corpus.GroupStd), DocumentID: groupDocID},
		lawRef:        graph.NodeRef{CorpusID: string(corpus.MYReg), DocumentID: lawDocID},
		sopEdgeID:     sopEdgeID,
		sopEvidenceID: sopEvidenceID,
	}
}

// insertDocRef inserts a graph.doc_ref stub row (document_id/section_id
// still NULL) and returns its id.
func insertDocRef(t *testing.T, ctx context.Context, pool *pgxpool.Pool, corpusID, refKey string) uuid.UUID {
	t.Helper()
	const q = `INSERT INTO graph.doc_ref (corpus_id, ref_key) VALUES ($1, $2) RETURNING id`
	var id uuid.UUID
	if err := pool.QueryRow(ctx, q, corpusID, refKey).Scan(&id); err != nil {
		t.Fatalf("inserting doc_ref (corpus %s): %v", corpusID, err)
	}
	return id
}

// resolveDocRef flips refID's doc_ref row from an unresolved stub to
// document_id = docID, so a Chain walk resolves to a real NodeRef instead of
// stopping at the stub (mirrors graph_chain_integration_test.go's
// resolveDocRefRow).
func resolveDocRef(t *testing.T, ctx context.Context, pool *pgxpool.Pool, refID, docID uuid.UUID) {
	t.Helper()
	const q = `UPDATE graph.doc_ref SET document_id = $1 WHERE id = $2`
	if _, err := pool.Exec(ctx, q, docID, refID); err != nil {
		t.Fatalf("resolving doc_ref %s to document %s: %v", refID, docID, err)
	}
}

// insertEdge inserts one graph.relation_edge row and returns its id.
func insertEdge(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	fromCorpusID string, fromDocID uuid.UUID, toCorpusID, edgeType string, toRefID uuid.UUID,
) uuid.UUID {
	t.Helper()
	const q = `
		INSERT INTO graph.relation_edge (from_corpus_id, from_document_id, to_ref_id, to_corpus_id, edge_type)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`
	var id uuid.UUID
	err := pool.QueryRow(ctx, q, fromCorpusID, fromDocID, toRefID, toCorpusID, edgeType).Scan(&id)
	if err != nil {
		t.Fatalf("inserting relation_edge (from %s/%s to %s): %v", fromCorpusID, fromDocID, toCorpusID, err)
	}
	return id
}

// insertEvidence inserts edgeID's evidence_kind='extracted' relation_evidence
// row and returns its id.
func insertEvidence(t *testing.T, ctx context.Context, pool *pgxpool.Pool, edgeID uuid.UUID) uuid.UUID {
	t.Helper()
	const q = `
		INSERT INTO graph.relation_evidence (edge_id, evidence_kind, confidence, rationale)
		VALUES ($1, 'extracted', 1.0, 'httpapi integration fixture')
		RETURNING id`
	var id uuid.UUID
	if err := pool.QueryRow(ctx, q, edgeID).Scan(&id); err != nil {
		t.Fatalf("inserting relation_evidence for edge %s: %v", edgeID, err)
	}
	return id
}

// newTestAPI builds a real chi router + huma API with httpapi.Register wired
// to a real, testdb-backed *store.GraphRepo — the same construction
// cmd/serving's newRouter and cmd/openapi-gen use (httpapi.NewAPI) — and
// starts an httptest.Server. Returning api too lets callers validate real
// response bodies against this exact instance's generated component
// schemas, not a second, possibly-diverged one.
func newTestAPI(t *testing.T, pool *pgxpool.Pool, role string) (huma.API, *httptest.Server) {
	t.Helper()
	router := chi.NewRouter()
	api := httpapi.NewAPI(router)
	httpapi.Register(api, store.NewGraphRepo(pool), role)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return api, srv
}

// encodeRef builds the {ref} path segment for ref, percent-encoding "/" as
// %2F (mirrors graph_test.go's encodeRef).
func encodeRef(ref graph.NodeRef) string {
	s := ref.CorpusID + "/" + ref.DocumentID.String()
	if ref.SectionID != nil {
		s += "/" + ref.SectionID.String()
	}
	return url.PathEscape(s)
}

// validateAgainstComponent asserts body validates against api's generated
// OpenAPI component schema named component (e.g. "NodeBody") — the
// santhosh-tekuri-based provider contract check the task calls for,
// validating the ACTUAL serialized response against the SAME generated
// schema api/openapi.yaml is built from, not a hand-copied schema file that
// could itself drift (mirrors pkg/mcp/contract_test.go's
// validateAgainstSchema, adapted to compile a component out of the live
// *huma.OpenAPI object instead of a standalone api/schemas/*.json file).
func validateAgainstComponent(t *testing.T, api huma.API, component string, body []byte) {
	t.Helper()
	raw, err := json.Marshal(api.OpenAPI())
	if err != nil {
		t.Fatalf("marshaling OpenAPI document: %v", err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("unmarshaling OpenAPI document for schema compilation: %v", err)
	}

	const resourceURL = "mise-openapi.json"
	c := jsonschema.NewCompiler()
	if err := c.AddResource(resourceURL, doc); err != nil {
		t.Fatalf("registering OpenAPI document as a schema resource: %v", err)
	}
	sch, err := c.Compile(resourceURL + "#/components/schemas/" + component)
	if err != nil {
		t.Fatalf("compiling component schema %s: %v", component, err)
	}

	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("unmarshaling response body for schema validation: %v", err)
	}
	if err := sch.Validate(inst); err != nil {
		t.Errorf("response does not validate against component %s: %v\nbody: %s", component, err, body)
	}
}

// getBody issues a GET to srv.URL+path and returns the status and raw body.
func getBody(t *testing.T, srv *httptest.Server, path string) (int, []byte) {
	t.Helper()
	resp, err := http.Get(srv.URL + path) //nolint:noctx // test helper; url is a local httptest.Server address
	if err != nil {
		t.Fatalf("GET %s error = %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("GET %s: reading body: %v", path, err)
	}
	return resp.StatusCode, body
}

// TestGraphNodeEndpointMatchesGeneratedSchema seeds a real edge+evidence
// row, calls GET /graph/nodes/{ref} under mise_local (sees every tier) over
// real HTTP, and validates the response both against the fixture's actual
// values and against the live NodeBody component schema.
func TestGraphNodeEndpointMatchesGeneratedSchema(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	fx := newGraphFixture(t, ctx, pool)

	api, srv := newTestAPI(t, pool, "mise_local")
	status, body := getBody(t, srv, "/graph/nodes/"+encodeRef(fx.sopRef))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, body)
	}
	validateAgainstComponent(t, api, "NodeBody", body)

	var got httpapi.NodeBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshaling NodeBody: %v; body: %s", err, body)
	}
	if got.Ref.CorpusID != fx.sopRef.CorpusID || got.Ref.DocumentID != fx.sopRef.DocumentID.String() {
		t.Errorf("Ref = %+v, want corpus %s document %s", got.Ref, fx.sopRef.CorpusID, fx.sopRef.DocumentID)
	}
	if len(got.Edges) != 1 {
		t.Fatalf("Edges = %d, want 1", len(got.Edges))
	}
	edge := got.Edges[0]
	if edge.ID != fx.sopEdgeID.String() || edge.EdgeType != "derives" || edge.ToCorpusID != string(corpus.LocalPolicy) {
		t.Errorf("Edges[0] = %+v, want id %s edge_type derives to_corpus_id %s",
			edge, fx.sopEdgeID, corpus.LocalPolicy)
	}
	if len(edge.Evidence) != 1 || edge.Evidence[0].ID != fx.sopEvidenceID.String() {
		t.Fatalf("Edges[0].Evidence = %+v, want one row id %s", edge.Evidence, fx.sopEvidenceID)
	}
	if edge.Evidence[0].EvidenceKind != "extracted" || edge.Evidence[0].Confidence != 1.0 {
		t.Errorf("Edges[0].Evidence[0] = %+v, want evidence_kind extracted confidence 1.0", edge.Evidence[0])
	}
}

// TestGraphNodeEndpointRLSDeniedReturns404 proves the REST layer's
// not-found mapping actually rides on the store's real RLS filtering:
// fx.sopRef's only edge is local-confidential (both endpoints are
// local-tier corpora), so mise_group — which cannot see local-confidential
// rows (graph_chain_integration_test.go's boundary test proves this at the
// store layer) — must get a plain 404 here, indistinguishable from a ref
// that has no edges at all.
func TestGraphNodeEndpointRLSDeniedReturns404(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	fx := newGraphFixture(t, ctx, pool)

	_, srv := newTestAPI(t, pool, "mise_group")
	status, body := getBody(t, srv, "/graph/nodes/"+encodeRef(fx.sopRef))
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", status, body)
	}
}

// TestGraphChainEndpointMatchesGeneratedSchema walks the fixture's full
// SOP -> Policy -> Group -> law chain under mise_local over real HTTP and
// validates both the hop sequence and the live ChainBody component schema.
func TestGraphChainEndpointMatchesGeneratedSchema(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	fx := newGraphFixture(t, ctx, pool)

	api, srv := newTestAPI(t, pool, "mise_local")
	status, body := getBody(t, srv, "/graph/chain/"+encodeRef(fx.sopRef))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, body)
	}
	validateAgainstComponent(t, api, "ChainBody", body)

	var got httpapi.ChainBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshaling ChainBody: %v; body: %s", err, body)
	}
	if len(got.Hops) != 3 {
		t.Fatalf("Hops = %d, want 3", len(got.Hops))
	}

	wantRefs := []graph.NodeRef{fx.policyRef, fx.groupRef, fx.lawRef}
	wantEdgeTypes := []string{"derives", "implements", "satisfies"}
	for i, hop := range got.Hops {
		if hop.Ref.CorpusID != wantRefs[i].CorpusID || hop.Ref.DocumentID != wantRefs[i].DocumentID.String() {
			t.Errorf("Hops[%d].Ref = %+v, want corpus %s document %s",
				i, hop.Ref, wantRefs[i].CorpusID, wantRefs[i].DocumentID)
		}
		if hop.EdgeType != wantEdgeTypes[i] {
			t.Errorf("Hops[%d].EdgeType = %q, want %q", i, hop.EdgeType, wantEdgeTypes[i])
		}
	}
}

// TestGraphChainEndpointMaxDepthLimitsHops proves max_depth reaches
// store.Chain (which clamps it, store/graph_chain.go's clampDepth): asking
// for 1 hop over the fixture's 3-hop chain must return exactly 1.
func TestGraphChainEndpointMaxDepthLimitsHops(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	fx := newGraphFixture(t, ctx, pool)

	_, srv := newTestAPI(t, pool, "mise_local")
	status, body := getBody(t, srv, "/graph/chain/"+encodeRef(fx.sopRef)+"?max_depth=1")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", status, body)
	}

	var got httpapi.ChainBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshaling ChainBody: %v; body: %s", err, body)
	}
	if len(got.Hops) != 1 {
		t.Fatalf("Hops = %d, want 1 (max_depth=1)", len(got.Hops))
	}
	if got.Hops[0].Ref.CorpusID != fx.policyRef.CorpusID {
		t.Errorf("Hops[0].Ref.CorpusID = %q, want %q", got.Hops[0].Ref.CorpusID, fx.policyRef.CorpusID)
	}
}

// TestGraphChainEndpointGroupBoundaryReturnsZeroHops mirrors
// graph_chain_integration_test.go's boundary test at the REST layer:
// mise_group walking from fx.sopRef (local-confidential) gets a normal 200
// with zero hops — the walk stops cleanly at the tier boundary, which is
// not the same thing as the ref itself being not-found.
func TestGraphChainEndpointGroupBoundaryReturnsZeroHops(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	fx := newGraphFixture(t, ctx, pool)

	_, srv := newTestAPI(t, pool, "mise_group")
	status, body := getBody(t, srv, "/graph/chain/"+encodeRef(fx.sopRef))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (a hop role can't see ends the walk, not a 404); body: %s", status, body)
	}

	var got httpapi.ChainBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshaling ChainBody: %v; body: %s", err, body)
	}
	if len(got.Hops) != 0 {
		t.Errorf("Hops = %d, want 0", len(got.Hops))
	}
}
