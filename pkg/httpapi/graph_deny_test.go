//go:build integration

package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"danny.vn/mise/internal/testdb"
	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/httpapi"
	"danny.vn/mise/pkg/store"
)

// denyFixture is a 3-hop chain: local-sop -> local-policy -> group-std.
// Both edges have access_tier = local-confidential (the stricter of each
// pair's corpora), so mise_public and mise_group must be denied on every
// read path, while mise_local sees everything.
type denyFixture struct {
	sopRef, policyRef, groupRef graph.NodeRef
	sopEdgeID, policyEdgeID     uuid.UUID
}

// newDenyFixture seeds the fixture described above, inserting as the pool's
// connecting owner role (bypasses RLS on write).
func newDenyFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) denyFixture {
	t.Helper()

	sopRef := graph.NodeRef{CorpusID: string(corpus.LocalSOP), DocumentID: uuid.New()}

	policyRefID := insertDocRef(t, ctx, pool, string(corpus.LocalPolicy), "local-policy:deny-"+uuid.NewString())
	groupRefID := insertDocRef(t, ctx, pool, string(corpus.GroupStd), "group-std:deny-"+uuid.NewString())

	policyDocID, groupDocID := uuid.New(), uuid.New()
	resolveDocRef(t, ctx, pool, policyRefID, policyDocID)
	resolveDocRef(t, ctx, pool, groupRefID, groupDocID)

	sopEdgeID := insertEdge(t, ctx, pool, string(corpus.LocalSOP), sopRef.DocumentID,
		string(corpus.LocalPolicy), "derives", policyRefID)
	insertEvidence(t, ctx, pool, sopEdgeID)

	policyEdgeID := insertEdge(t, ctx, pool, string(corpus.LocalPolicy), policyDocID,
		string(corpus.GroupStd), "implements", groupRefID)
	insertEvidence(t, ctx, pool, policyEdgeID)

	return denyFixture{
		sopRef:       sopRef,
		policyRef:    graph.NodeRef{CorpusID: string(corpus.LocalPolicy), DocumentID: policyDocID},
		groupRef:     graph.NodeRef{CorpusID: string(corpus.GroupStd), DocumentID: groupDocID},
		sopEdgeID:    sopEdgeID,
		policyEdgeID: policyEdgeID,
	}
}

// TestGraphDenySuite proves the graph's RLS tier isolation holds across
// every read path: REST nodes, REST chain, the store repo directly, and a
// raw SQL oracle. Both edges in the fixture are local-confidential, so
// mise_public and mise_group must be fully denied; mise_local sees all.
func TestGraphDenySuite(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	fx := newDenyFixture(t, ctx, pool)

	t.Run("REST /graph/nodes/{ref} deny", func(t *testing.T) {
		assertRESTNodesDeny(t, pool, fx)
	})
	t.Run("REST /graph/chain/{ref} deny", func(t *testing.T) {
		assertRESTChainDeny(t, pool, fx)
	})
	t.Run("store repo deny", func(t *testing.T) {
		assertStoreRepoDeny(t, ctx, pool, fx)
	})
	t.Run("raw oracle", func(t *testing.T) {
		assertRawOracleDeny(t, ctx, pool, fx)
	})
}

// assertRESTNodesDeny proves GET /graph/nodes/{sopRef} returns 404 for
// mise_public and mise_group (every edge is local-confidential), and 200
// for mise_local.
func assertRESTNodesDeny(t *testing.T, pool *pgxpool.Pool, fx denyFixture) {
	t.Helper()

	t.Run("mise_public returns 404", func(t *testing.T) {
		_, srv := newTestAPI(t, pool, "mise_public")
		status, body := getBody(t, srv, "/graph/nodes/"+encodeRef(fx.sopRef))
		if status != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body: %s", status, body)
		}
	})

	t.Run("mise_group returns 404", func(t *testing.T) {
		_, srv := newTestAPI(t, pool, "mise_group")
		status, body := getBody(t, srv, "/graph/nodes/"+encodeRef(fx.sopRef))
		if status != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body: %s", status, body)
		}
	})

	t.Run("mise_local returns 200", func(t *testing.T) {
		_, srv := newTestAPI(t, pool, "mise_local")
		status, body := getBody(t, srv, "/graph/nodes/"+encodeRef(fx.sopRef))
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", status, body)
		}
	})
}

// assertRESTChainDeny proves GET /graph/chain/{sopRef} returns 200 with 0
// hops for mise_public and mise_group (the walk stops cleanly at the tier
// boundary), and 200 with 2 hops for mise_local (local-sop -> local-policy
// -> group-std).
func assertRESTChainDeny(t *testing.T, pool *pgxpool.Pool, fx denyFixture) {
	t.Helper()

	t.Run("mise_public returns 200 with 0 hops", func(t *testing.T) {
		_, srv := newTestAPI(t, pool, "mise_public")
		status, body := getBody(t, srv, "/graph/chain/"+encodeRef(fx.sopRef))
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", status, body)
		}
		var got httpapi.ChainBody
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("unmarshaling ChainBody: %v; body: %s", err, body)
		}
		if len(got.Hops) != 0 {
			t.Errorf("hops = %d, want 0 for mise_public", len(got.Hops))
		}
	})

	t.Run("mise_group returns 200 with 0 hops", func(t *testing.T) {
		_, srv := newTestAPI(t, pool, "mise_group")
		status, body := getBody(t, srv, "/graph/chain/"+encodeRef(fx.sopRef))
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", status, body)
		}
		var got httpapi.ChainBody
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("unmarshaling ChainBody: %v; body: %s", err, body)
		}
		if len(got.Hops) != 0 {
			t.Errorf("hops = %d, want 0 for mise_group", len(got.Hops))
		}
	})

	t.Run("mise_local returns 200 with 2 hops", func(t *testing.T) {
		_, srv := newTestAPI(t, pool, "mise_local")
		status, body := getBody(t, srv, "/graph/chain/"+encodeRef(fx.sopRef))
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", status, body)
		}
		var got httpapi.ChainBody
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("unmarshaling ChainBody: %v; body: %s", err, body)
		}
		if len(got.Hops) != 2 {
			t.Fatalf("hops = %d, want 2 for mise_local", len(got.Hops))
		}
		hop0 := got.Hops[0]
		if hop0.Ref.CorpusID != fx.policyRef.CorpusID || hop0.Ref.DocumentID != fx.policyRef.DocumentID.String() {
			t.Errorf("Hops[0].Ref = %+v, want corpus %s document %s",
				hop0.Ref, fx.policyRef.CorpusID, fx.policyRef.DocumentID)
		}
		if got.Hops[1].Ref.CorpusID != fx.groupRef.CorpusID || got.Hops[1].Ref.DocumentID != fx.groupRef.DocumentID.String() {
			t.Errorf("Hops[1].Ref = %+v, want corpus %s document %s",
				got.Hops[1].Ref, fx.groupRef.CorpusID, fx.groupRef.DocumentID)
		}
	})
}

// assertStoreRepoDeny proves the store layer's RLS isolation by calling
// GraphRepo.GetNode and GraphRepo.Chain directly — the MCP handler's own
// read path, tested without going through HTTP.
func assertStoreRepoDeny(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fx denyFixture) {
	t.Helper()
	repo := store.NewGraphRepo(pool)

	t.Run("GetNode mise_public returns ErrNodeNotFound", func(t *testing.T) {
		_, err := repo.GetNode(ctx, "mise_public", fx.sopRef)
		if !errors.Is(err, store.ErrNodeNotFound) {
			t.Errorf("GetNode(mise_public) error = %v, want ErrNodeNotFound", err)
		}
	})

	t.Run("GetNode mise_group returns ErrNodeNotFound", func(t *testing.T) {
		_, err := repo.GetNode(ctx, "mise_group", fx.sopRef)
		if !errors.Is(err, store.ErrNodeNotFound) {
			t.Errorf("GetNode(mise_group) error = %v, want ErrNodeNotFound", err)
		}
	})

	t.Run("GetNode mise_local returns edges", func(t *testing.T) {
		view, err := repo.GetNode(ctx, "mise_local", fx.sopRef)
		if err != nil {
			t.Fatalf("GetNode(mise_local) error = %v, want nil", err)
		}
		if len(view.Edges) == 0 {
			t.Errorf("GetNode(mise_local) returned 0 edges, want >0")
		}
	})

	t.Run("Chain mise_public returns 0 hops", func(t *testing.T) {
		hops, err := repo.Chain(ctx, "mise_public", fx.sopRef, 8)
		if err != nil {
			t.Fatalf("Chain(mise_public) error = %v, want nil", err)
		}
		if len(hops) != 0 {
			t.Errorf("Chain(mise_public) hops = %d, want 0", len(hops))
		}
	})

	t.Run("Chain mise_group returns 0 hops", func(t *testing.T) {
		hops, err := repo.Chain(ctx, "mise_group", fx.sopRef, 8)
		if err != nil {
			t.Fatalf("Chain(mise_group) error = %v, want nil", err)
		}
		if len(hops) != 0 {
			t.Errorf("Chain(mise_group) hops = %d, want 0", len(hops))
		}
	})

	t.Run("Chain mise_local returns 2 hops", func(t *testing.T) {
		hops, err := repo.Chain(ctx, "mise_local", fx.sopRef, 8)
		if err != nil {
			t.Fatalf("Chain(mise_local) error = %v, want nil", err)
		}
		if len(hops) != 2 {
			t.Errorf("Chain(mise_local) hops = %d, want 2", len(hops))
		}
	})
}

// assertRawOracleDeny is the independent oracle: a raw SELECT count(*) run
// under SET LOCAL ROLE for each (role, edge) pair, confirming that both
// local-confidential edges are invisible to mise_public and mise_group, and
// visible to mise_local.
func assertRawOracleDeny(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fx denyFixture) {
	t.Helper()
	cases := []struct {
		role  string
		label string
		rowID uuid.UUID
		want  int
	}{
		{role: "mise_public", label: "sopEdge", rowID: fx.sopEdgeID, want: 0},
		{role: "mise_public", label: "policyEdge", rowID: fx.policyEdgeID, want: 0},
		{role: "mise_group", label: "sopEdge", rowID: fx.sopEdgeID, want: 0},
		{role: "mise_group", label: "policyEdge", rowID: fx.policyEdgeID, want: 0},
		{role: "mise_local", label: "sopEdge", rowID: fx.sopEdgeID, want: 1},
		{role: "mise_local", label: "policyEdge", rowID: fx.policyEdgeID, want: 1},
	}
	for _, tc := range cases {
		t.Run(tc.role+"/"+tc.label, func(t *testing.T) {
			got := rawDenyCount(t, ctx, pool, tc.role, "relation_edge", tc.rowID)
			if got != tc.want {
				t.Errorf("raw count on graph.relation_edge (id=%v) as %s = %d, want %d",
					tc.rowID, tc.role, got, tc.want)
			}
		})
	}
}

// rawDenyCount runs SET LOCAL ROLE role; SELECT count(*) FROM graph.<table>
// WHERE id = rowID directly against pool. A role with no GRANT USAGE on the
// graph schema (SQLSTATE 42501, insufficient_privilege) folds to 0.
func rawDenyCount(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool, role, table string, rowID uuid.UUID,
) int {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("beginning raw deny count tx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SET LOCAL ROLE `+role); err != nil {
		t.Fatalf("SET LOCAL ROLE %s: %v", role, err)
	}

	var n int
	q := `SELECT count(*) FROM graph.` + table + ` WHERE id = $1`
	err = tx.QueryRow(ctx, q, rowID).Scan(&n)
	switch {
	case err == nil:
		return n
	case isDenyPermissionDenied(err):
		return 0
	default:
		t.Fatalf("raw count on graph.%s as %s: %v", table, role, err)
		return 0
	}
}

// isDenyPermissionDenied reports whether err is Postgres SQLSTATE 42501
// (insufficient_privilege). Mirrors store_test's isSchemaPermissionDenied,
// kept as its own copy since that helper is unexported and in a different
// test package.
func isDenyPermissionDenied(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "42501"
}
