//go:build integration

package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"danny.vn/mise/internal/testdb"
	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/store"
)

// contractGraphRepo adapts *store.GraphRepo to GraphRepoIface — the same
// shape cmd/serving's real wiring uses (storeGraphRepo, cmd/serving/main.go).
type contractGraphRepo struct {
	repo *store.GraphRepo
}

func (g contractGraphRepo) GetNode(ctx context.Context, role string, ref graph.NodeRef) (store.NodeView, error) {
	return g.repo.GetNode(ctx, role, ref)
}

func (g contractGraphRepo) Chain(
	ctx context.Context, role string, start graph.NodeRef, maxDepth int,
) ([]store.Hop, error) {
	return g.repo.Chain(ctx, role, start, maxDepth)
}

// graphContractFixture is one resolved control-chain edge — local-sop
// --derives--> local-policy, with a model_classification evidence row —
// seeded via raw SQL as the pool's connecting owner role (bypasses RLS on
// write), mirroring graph_rls_test.go/graph_chain_integration_test.go's
// seeding style (those helpers live in pkg/store's own _test.go files and
// aren't importable from this package). The target doc_ref is resolved
// (document_id set at insert, not left as an unresolved stub) so
// GraphRepo.Chain walks a real hop, not just GetNode's own edge list.
type graphContractFixture struct {
	sopRef    graph.NodeRef
	policyRef graph.NodeRef
}

// newGraphContractFixture seeds the fixture described above.
func newGraphContractFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) graphContractFixture {
	t.Helper()

	sopRef := graph.NodeRef{CorpusID: string(corpus.LocalSOP), DocumentID: uuid.New()}
	policyDocID := uuid.New()

	const refQ = `INSERT INTO graph.doc_ref (corpus_id, ref_key, document_id) VALUES ($1, $2, $3) RETURNING id`
	var refID uuid.UUID
	err := pool.QueryRow(ctx, refQ, string(corpus.LocalPolicy), "local-policy:contract-"+uuid.NewString(), policyDocID).
		Scan(&refID)
	if err != nil {
		t.Fatalf("inserting resolved doc_ref: %v", err)
	}

	const edgeQ = `
		INSERT INTO graph.relation_edge (from_corpus_id, from_document_id, to_ref_id, to_corpus_id, edge_type)
		VALUES ($1, $2, $3, $4, 'derives')
		RETURNING id`
	var edgeID uuid.UUID
	err = pool.QueryRow(ctx, edgeQ, string(corpus.LocalSOP), sopRef.DocumentID, refID, string(corpus.LocalPolicy)).
		Scan(&edgeID)
	if err != nil {
		t.Fatalf("inserting relation_edge: %v", err)
	}

	const evQ = `
		INSERT INTO graph.relation_evidence (edge_id, evidence_kind, confidence, grounding_score, rationale)
		VALUES ($1, 'model_classification', 0.87, 0.91, 'contract fixture rationale')`
	if _, err := pool.Exec(ctx, evQ, edgeID); err != nil {
		t.Fatalf("inserting relation_evidence: %v", err)
	}

	return graphContractFixture{
		sopRef:    sopRef,
		policyRef: graph.NodeRef{CorpusID: string(corpus.LocalPolicy), DocumentID: policyDocID},
	}
}

// TestGraphOutputMatchesSchema seeds a real, resolved graph edge via raw
// SQL, calls the graph tool handler directly against a store-backed
// GraphRepoIface, and validates the marshaled GraphOutput against
// api/schemas/graph_output.schema.json.
func TestGraphOutputMatchesSchema(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	fx := newGraphContractFixture(t, ctx, pool)

	repo := contractGraphRepo{repo: store.NewGraphRepo(pool)}
	h := newGraphHandler(repo, "mise_local")

	_, out, err := h(ctx, nil, GraphInput{NodeRef: nodeRefWire(fx.sopRef)})
	if err != nil {
		t.Fatalf("graph handler error = %v", err)
	}
	if len(out.Edges) == 0 {
		t.Fatal("graph handler returned zero edges, want the seeded fixture edge")
	}
	if len(out.Chain) == 0 {
		t.Fatal("graph handler returned zero chain hops, want the seeded fixture hop")
	}
	if out.Chain[0].DocumentID != fx.policyRef.DocumentID.String() {
		t.Errorf("out.Chain[0].DocumentID = %q, want %q", out.Chain[0].DocumentID, fx.policyRef.DocumentID)
	}

	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshaling GraphOutput: %v", err)
	}
	validateAgainstSchema(t, schemaPath(t, "graph_output.schema.json"), data)
}
