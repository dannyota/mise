//go:build integration

package store_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"danny.vn/mise/internal/testdb"
	"danny.vn/mise/pkg/graph"
)

// graphRLSFixture is one local-confidential edge (local-policy ->
// group-std) and one group-confidential edge (group-std -> my-reg), each
// with a relation_evidence row and a doc_ref on its target ("to") side —
// inserted as the pool's connecting owner role, which bypasses RLS on
// write, exactly like rlsFixture (rls_test.go). The two target doc_refs
// double as most of the doc_ref RLS case: groupStdRefID's own corpus
// (group-std) is group-confidential, myRegRefID's (my-reg) is public —
// proving a doc_ref's visibility is anchored to its OWN corpus, not to the
// tier of whichever edge happens to reference it. myRegRefID stays
// public-visible even though groupEdgeID, which points at it, is
// group-confidential (its FROM side, group-std, is the stricter one). A
// third, edge-less doc_ref (localRefID, corpus local-policy) fills out the
// doc_ref tier matrix's remaining boundary: proving mise_group's
// tier_rank(...)<=1 policy actually excludes rank-2 (local-confidential),
// not just admits rank-1 (group-confidential) — a case neither edge's own
// target side exercises.
type graphRLSFixture struct {
	localEdgeID, groupEdgeID         uuid.UUID
	localEvidenceID, groupEvidenceID uuid.UUID
	groupStdRefID, myRegRefID        uuid.UUID
	localRefID                       uuid.UUID
}

// newGraphRLSFixture seeds the fixture described above. doc_refs are
// inserted before their edges since relation_edge_set_to_corpus
// (migrations/009_graph_tables.sql) reads the doc_ref row to resolve
// to_corpus_id; edges are inserted via raw SQL (insertEdgeForRLS) rather
// than graph_schema_test.go's insertEdge, since this suite needs each row's
// id back to query its per-role visibility, not just its computed tier.
func newGraphRLSFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) graphRLSFixture {
	t.Helper()

	groupStdRef := insertDocRef(t, ctx, pool, "group-std", "group-std:rls-"+uuid.NewString())
	myRegRef := insertDocRef(t, ctx, pool, "my-reg", "my-reg:rls-"+uuid.NewString())
	localRef := insertDocRef(t, ctx, pool, "local-policy", "local-policy:rls-"+uuid.NewString())

	localEdgeID, localTier := insertEdgeForRLS(t, ctx, pool, "local-policy", "group-std", groupStdRef.ID)
	if localTier != graph.TierLocalConfidential {
		t.Fatalf("fixture local-policy->group-std edge access_tier = %q, want %q", localTier, graph.TierLocalConfidential)
	}
	groupEdgeID, groupTier := insertEdgeForRLS(t, ctx, pool, "group-std", "my-reg", myRegRef.ID)
	if groupTier != graph.TierGroupConfidential {
		t.Fatalf("fixture group-std->my-reg edge access_tier = %q, want %q", groupTier, graph.TierGroupConfidential)
	}

	return graphRLSFixture{
		localEdgeID:     localEdgeID,
		groupEdgeID:     groupEdgeID,
		localEvidenceID: insertEvidenceForRLS(t, ctx, pool, localEdgeID),
		groupEvidenceID: insertEvidenceForRLS(t, ctx, pool, groupEdgeID),
		groupStdRefID:   groupStdRef.ID,
		myRegRefID:      myRegRef.ID,
		localRefID:      localRef.ID,
	}
}

// insertEdgeForRLS inserts a graph.relation_edge row as the pool's
// connecting owner role (bypasses RLS on write) and returns both its id and
// the Postgres-computed access_tier. edge_type is fixed to "satisfies" —
// irrelevant to RLS, which keys only on access_tier.
func insertEdgeForRLS(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool, fromCorpusID, toCorpusID string, toRefID uuid.UUID,
) (uuid.UUID, graph.Tier) {
	t.Helper()
	const q = `
		INSERT INTO graph.relation_edge (from_corpus_id, from_document_id, to_ref_id, to_corpus_id, edge_type)
		VALUES ($1, $2, $3, $4, 'satisfies')
		RETURNING id, access_tier`
	var id uuid.UUID
	var tier string
	err := pool.QueryRow(ctx, q, fromCorpusID, uuid.New(), toRefID, toCorpusID).Scan(&id, &tier)
	if err != nil {
		t.Fatalf("inserting graph.relation_edge (from %s to %s): %v", fromCorpusID, toCorpusID, err)
	}
	return id, graph.Tier(tier)
}

// insertEvidenceForRLS inserts a minimal graph.relation_evidence row FK'd to
// edgeID, as the pool's connecting owner role, and returns its id.
func insertEvidenceForRLS(t *testing.T, ctx context.Context, pool *pgxpool.Pool, edgeID uuid.UUID) uuid.UUID {
	t.Helper()
	const q = `
		INSERT INTO graph.relation_evidence (edge_id, evidence_kind, confidence, rationale)
		VALUES ($1, $2, 0.9, 'rls fixture evidence')
		RETURNING id`
	var id uuid.UUID
	err := pool.QueryRow(ctx, q, edgeID, string(graph.EvidenceModelClassification)).Scan(&id)
	if err != nil {
		t.Fatalf("inserting graph.relation_evidence for edge %v: %v", edgeID, err)
	}
	return id
}

// TestGraphRLSDenySuite is the M2 confidentiality gate for the graph join
// surface (RISKS R2): a role with USAGE on the graph schema (migrations/
// 004_rls_roles.sql) must still be blocked, row by row, from any
// edge/evidence/doc_ref outside its tier. mise_public must never see a
// group- or local-confidential row on any of the three graph tables;
// mise_group sees group- but not local-confidential; mise_local sees both.
// A raw SELECT count(*) run under SET LOCAL ROLE must agree with every
// targeted assertion above, across an exhaustive (role, table, row) sweep.
// Each assertion group is its own top-level helper (not an inline closure)
// so this driver stays under the cognitive-complexity lint budget,
// mirroring rls_test.go's TestSearchRLSDenySuite.
func TestGraphRLSDenySuite(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	fx := newGraphRLSFixture(t, ctx, pool)

	t.Run("mise_public sees neither confidential edge nor its evidence, only the public-tier doc_ref", func(t *testing.T) {
		assertGraphPublicSeesOnlyPublicTier(t, ctx, pool, fx)
	})
	t.Run("mise_group sees group-confidential but not local-confidential", func(t *testing.T) {
		assertGraphGroupSeesGroupNotLocal(t, ctx, pool, fx)
	})
	t.Run("mise_local sees both tiers", func(t *testing.T) {
		assertGraphLocalSeesBothTiers(t, ctx, pool, fx)
	})
	t.Run("raw SELECT count(*) per role confirms the same visibility", func(t *testing.T) {
		assertGraphRawCountsMatchVisibility(t, ctx, pool, fx)
	})
}

// assertGraphPublicSeesOnlyPublicTier is TestGraphRLSDenySuite's mise_public
// case: neither confidential edge, nor either edge's evidence, nor the
// group-confidential doc_ref may be visible — but myRegRefID (a doc_ref
// anchored to the public my-reg corpus) must still be, since doc_ref
// visibility is keyed on its own corpus, not on the tier of any edge that
// references it.
func assertGraphPublicSeesOnlyPublicTier(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fx graphRLSFixture) {
	t.Helper()
	const role = "mise_public"
	if got := rawGraphVisibleCount(t, ctx, pool, role, "relation_edge", fx.localEdgeID); got != 0 {
		t.Errorf("%s sees local-confidential relation_edge: count = %d, want 0", role, got)
	}
	if got := rawGraphVisibleCount(t, ctx, pool, role, "relation_edge", fx.groupEdgeID); got != 0 {
		t.Errorf("%s sees group-confidential relation_edge: count = %d, want 0", role, got)
	}
	if got := rawGraphVisibleCount(t, ctx, pool, role, "relation_evidence", fx.localEvidenceID); got != 0 {
		t.Errorf("%s sees local-confidential relation_evidence: count = %d, want 0", role, got)
	}
	if got := rawGraphVisibleCount(t, ctx, pool, role, "relation_evidence", fx.groupEvidenceID); got != 0 {
		t.Errorf("%s sees group-confidential relation_evidence: count = %d, want 0", role, got)
	}
	if got := rawGraphVisibleCount(t, ctx, pool, role, "doc_ref", fx.groupStdRefID); got != 0 {
		t.Errorf("%s sees group-confidential doc_ref: count = %d, want 0", role, got)
	}
	if got := rawGraphVisibleCount(t, ctx, pool, role, "doc_ref", fx.localRefID); got != 0 {
		t.Errorf("%s sees local-confidential doc_ref: count = %d, want 0", role, got)
	}
	if got := rawGraphVisibleCount(t, ctx, pool, role, "doc_ref", fx.myRegRefID); got != 1 {
		t.Errorf("%s does not see public-tier doc_ref: count = %d, want 1", role, got)
	}
}

// assertGraphGroupSeesGroupNotLocal is TestGraphRLSDenySuite's mise_group
// case: the group-confidential edge/evidence/doc_ref are visible, the
// local-confidential edge/evidence/doc_ref are not — the doc_ref check is
// the one row (localRefID) that pins tier_rank(...)<=1 as an upper bound,
// not just a group-confidential allowlist.
func assertGraphGroupSeesGroupNotLocal(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fx graphRLSFixture) {
	t.Helper()
	const role = "mise_group"
	if got := rawGraphVisibleCount(t, ctx, pool, role, "relation_edge", fx.localEdgeID); got != 0 {
		t.Errorf("%s sees local-confidential relation_edge: count = %d, want 0", role, got)
	}
	if got := rawGraphVisibleCount(t, ctx, pool, role, "relation_edge", fx.groupEdgeID); got != 1 {
		t.Errorf("%s does not see group-confidential relation_edge: count = %d, want 1", role, got)
	}
	if got := rawGraphVisibleCount(t, ctx, pool, role, "relation_evidence", fx.localEvidenceID); got != 0 {
		t.Errorf("%s sees local-confidential relation_evidence: count = %d, want 0", role, got)
	}
	if got := rawGraphVisibleCount(t, ctx, pool, role, "relation_evidence", fx.groupEvidenceID); got != 1 {
		t.Errorf("%s does not see group-confidential relation_evidence: count = %d, want 1", role, got)
	}
	if got := rawGraphVisibleCount(t, ctx, pool, role, "doc_ref", fx.groupStdRefID); got != 1 {
		t.Errorf("%s does not see group-confidential doc_ref: count = %d, want 1", role, got)
	}
	if got := rawGraphVisibleCount(t, ctx, pool, role, "doc_ref", fx.localRefID); got != 0 {
		t.Errorf("%s sees local-confidential doc_ref: count = %d, want 0", role, got)
	}
	if got := rawGraphVisibleCount(t, ctx, pool, role, "doc_ref", fx.myRegRefID); got != 1 {
		t.Errorf("%s does not see public-tier doc_ref: count = %d, want 1", role, got)
	}
}

// assertGraphLocalSeesBothTiers is TestGraphRLSDenySuite's mise_local case:
// every fixture row across all three graph tables is visible.
func assertGraphLocalSeesBothTiers(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fx graphRLSFixture) {
	t.Helper()
	const role = "mise_local"
	if got := rawGraphVisibleCount(t, ctx, pool, role, "relation_edge", fx.localEdgeID); got != 1 {
		t.Errorf("%s does not see local-confidential relation_edge: count = %d, want 1", role, got)
	}
	if got := rawGraphVisibleCount(t, ctx, pool, role, "relation_edge", fx.groupEdgeID); got != 1 {
		t.Errorf("%s does not see group-confidential relation_edge: count = %d, want 1", role, got)
	}
	if got := rawGraphVisibleCount(t, ctx, pool, role, "relation_evidence", fx.localEvidenceID); got != 1 {
		t.Errorf("%s does not see local-confidential relation_evidence: count = %d, want 1", role, got)
	}
	if got := rawGraphVisibleCount(t, ctx, pool, role, "relation_evidence", fx.groupEvidenceID); got != 1 {
		t.Errorf("%s does not see group-confidential relation_evidence: count = %d, want 1", role, got)
	}
	if got := rawGraphVisibleCount(t, ctx, pool, role, "doc_ref", fx.groupStdRefID); got != 1 {
		t.Errorf("%s does not see group-confidential doc_ref: count = %d, want 1", role, got)
	}
	if got := rawGraphVisibleCount(t, ctx, pool, role, "doc_ref", fx.localRefID); got != 1 {
		t.Errorf("%s does not see local-confidential doc_ref: count = %d, want 1", role, got)
	}
	if got := rawGraphVisibleCount(t, ctx, pool, role, "doc_ref", fx.myRegRefID); got != 1 {
		t.Errorf("%s does not see public-tier doc_ref: count = %d, want 1", role, got)
	}
}

// assertGraphRawCountsMatchVisibility is TestGraphRLSDenySuite's independent
// oracle: a raw SELECT count(*) under SET LOCAL ROLE, per (role, table,
// row) triple, run via rawGraphVisibleCount exactly as the three targeted
// assertions above do — but as one exhaustive table-driven sweep across all
// three graph tables and all three roles, instead of narrated per-role
// cases. Mirrors rls_test.go's assertRawCountsMatchVisibility shape.
func assertGraphRawCountsMatchVisibility(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fx graphRLSFixture) {
	t.Helper()
	cases := []struct {
		role        string
		table       string
		rowID       uuid.UUID
		wantVisible bool
	}{
		{role: "mise_public", table: "relation_edge", rowID: fx.localEdgeID, wantVisible: false},
		{role: "mise_public", table: "relation_edge", rowID: fx.groupEdgeID, wantVisible: false},
		{role: "mise_public", table: "relation_evidence", rowID: fx.localEvidenceID, wantVisible: false},
		{role: "mise_public", table: "relation_evidence", rowID: fx.groupEvidenceID, wantVisible: false},
		{role: "mise_public", table: "doc_ref", rowID: fx.groupStdRefID, wantVisible: false},
		{role: "mise_public", table: "doc_ref", rowID: fx.localRefID, wantVisible: false},
		{role: "mise_public", table: "doc_ref", rowID: fx.myRegRefID, wantVisible: true},

		{role: "mise_group", table: "relation_edge", rowID: fx.localEdgeID, wantVisible: false},
		{role: "mise_group", table: "relation_edge", rowID: fx.groupEdgeID, wantVisible: true},
		{role: "mise_group", table: "relation_evidence", rowID: fx.localEvidenceID, wantVisible: false},
		{role: "mise_group", table: "relation_evidence", rowID: fx.groupEvidenceID, wantVisible: true},
		{role: "mise_group", table: "doc_ref", rowID: fx.groupStdRefID, wantVisible: true},
		{role: "mise_group", table: "doc_ref", rowID: fx.localRefID, wantVisible: false},
		{role: "mise_group", table: "doc_ref", rowID: fx.myRegRefID, wantVisible: true},

		{role: "mise_local", table: "relation_edge", rowID: fx.localEdgeID, wantVisible: true},
		{role: "mise_local", table: "relation_edge", rowID: fx.groupEdgeID, wantVisible: true},
		{role: "mise_local", table: "relation_evidence", rowID: fx.localEvidenceID, wantVisible: true},
		{role: "mise_local", table: "relation_evidence", rowID: fx.groupEvidenceID, wantVisible: true},
		{role: "mise_local", table: "doc_ref", rowID: fx.groupStdRefID, wantVisible: true},
		{role: "mise_local", table: "doc_ref", rowID: fx.localRefID, wantVisible: true},
		{role: "mise_local", table: "doc_ref", rowID: fx.myRegRefID, wantVisible: true},
	}
	for _, tc := range cases {
		got := rawGraphVisibleCount(t, ctx, pool, tc.role, tc.table, tc.rowID)
		want := 0
		if tc.wantVisible {
			want = 1
		}
		if got != want {
			t.Errorf("raw count on graph.%s (id=%v) as %s = %d, want %d", tc.table, tc.rowID, tc.role, got, want)
		}
	}
}

// rawGraphVisibleCount runs SET LOCAL ROLE role; SELECT count(*) FROM
// graph.<table> WHERE id = rowID directly against pool, bypassing any
// future graph read API entirely — this suite's independent oracle. It is
// the graph-schema counterpart of rls_test.go's rawVisibleCount (that
// helper is hardcoded to <schema>.section keyed by document_id; every graph
// table instead has its own id primary key, so this one keys on id and
// takes the table name as a parameter). role/table always come from this
// file's own fixed case tables above, never external input. A role with no
// GRANT USAGE on the graph schema at all (SQLSTATE 42501) folds to 0,
// matching rawVisibleCount's own permission-denied handling.
func rawGraphVisibleCount(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool, role, table string, rowID uuid.UUID,
) int {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("beginning raw count tx: %v", err)
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
	case isSchemaPermissionDenied(err):
		return 0
	default:
		t.Fatalf("raw count on graph.%s as %s: %v", table, role, err)
		return 0
	}
}
