//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"danny.vn/mise/internal/testdb"
	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/store"
)

// findingRLSFixture holds three findings at different tiers and one
// resolution (FK'd to the local-confidential finding) for the RLS deny
// suite.
type findingRLSFixture struct {
	publicFindingID uuid.UUID
	groupFindingID  uuid.UUID
	localFindingID  uuid.UUID
	resolutionID    uuid.UUID // on localFindingID
}

// newFindingRLSFixture seeds findings at each tier level and one
// resolution on the local-confidential finding, all inserted as the
// pool's connecting owner role (bypasses RLS on write).
func newFindingRLSFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) findingRLSFixture {
	t.Helper()
	fs := store.NewFindingStore(pool)

	// Public: only vn-reg nodes.
	pubID, err := fs.CreateFinding(ctx, store.Finding{
		Kind:       "gap",
		Severity:   "medium",
		Status:     "open",
		NodeRefs:   []store.NodeRefJSON{{CorpusID: "vn-reg", DocumentID: uuid.New()}},
		DetectedAt: time.Now(),
		DedupKey:   "rls-pub-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("creating public finding: %v", err)
	}

	// Group-confidential: group-std node.
	grpID, err := fs.CreateFinding(ctx, store.Finding{
		Kind:       "conflict",
		Severity:   "high",
		Status:     "open",
		NodeRefs:   []store.NodeRefJSON{{CorpusID: "group-std", DocumentID: uuid.New()}},
		DetectedAt: time.Now(),
		DedupKey:   "rls-grp-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("creating group finding: %v", err)
	}

	// Local-confidential: local-policy node.
	locID, err := fs.CreateFinding(ctx, store.Finding{
		Kind:       "staleness",
		Severity:   "low",
		Status:     "open",
		NodeRefs:   []store.NodeRefJSON{{CorpusID: "local-policy", DocumentID: uuid.New()}},
		DetectedAt: time.Now(),
		DedupKey:   "rls-loc-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("creating local finding: %v", err)
	}

	// Verify trigger-computed tiers match expectations.
	assertFindingTier(t, ctx, pool, pubID, string(graph.TierPublic))
	assertFindingTier(t, ctx, pool, grpID, string(graph.TierGroupConfidential))
	assertFindingTier(t, ctx, pool, locID, string(graph.TierLocalConfidential))

	// Resolution on the local-confidential finding.
	resID, err := fs.CreateResolution(ctx, locID, store.Resolution{
		Disposition: "accept",
		Status:      "open",
		Rationale:   "rls fixture resolution",
	})
	if err != nil {
		t.Fatalf("creating resolution: %v", err)
	}

	return findingRLSFixture{
		publicFindingID: pubID,
		groupFindingID:  grpID,
		localFindingID:  locID,
		resolutionID:    resID,
	}
}

// assertFindingTier verifies a finding's trigger-computed access_tier.
func assertFindingTier(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, want string) {
	t.Helper()
	var tier string
	if err := pool.QueryRow(ctx, `SELECT access_tier FROM graph.finding WHERE id = $1`, id).Scan(&tier); err != nil {
		t.Fatalf("reading access_tier for finding %v: %v", id, err)
	}
	if tier != want {
		t.Fatalf("finding %v access_tier = %q, want %q", id, tier, want)
	}
}

// TestFindingRLSDenySuite is the M3 confidentiality gate for findings:
// each role sees only findings at or below its tier. finding_resolution
// inherits visibility from its parent finding. Mirrors
// TestGraphRLSDenySuite (graph_rls_test.go).
func TestFindingRLSDenySuite(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	fx := newFindingRLSFixture(t, ctx, pool)

	t.Run("mise_public sees only public findings and no resolutions", func(t *testing.T) {
		assertFindingPublicSeesOnlyPublic(t, ctx, pool, fx)
	})
	t.Run("mise_group sees group-confidential but not local-confidential", func(t *testing.T) {
		assertFindingGroupSeesGroupNotLocal(t, ctx, pool, fx)
	})
	t.Run("mise_local sees all tiers", func(t *testing.T) {
		assertFindingLocalSeesAll(t, ctx, pool, fx)
	})
	t.Run("raw SELECT count(*) per role confirms visibility", func(t *testing.T) {
		assertFindingRawCountsMatchVisibility(t, ctx, pool, fx)
	})
}

// assertFindingPublicSeesOnlyPublic: mise_public sees only the public
// finding, neither group nor local findings, and not the resolution
// (which belongs to a local-confidential finding).
func assertFindingPublicSeesOnlyPublic(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fx findingRLSFixture) {
	t.Helper()
	const role = "mise_public"
	if got := rawFindingVisibleCount(t, ctx, pool, role, "finding", fx.publicFindingID); got != 1 {
		t.Errorf("%s does not see public finding: count = %d, want 1", role, got)
	}
	if got := rawFindingVisibleCount(t, ctx, pool, role, "finding", fx.groupFindingID); got != 0 {
		t.Errorf("%s sees group-confidential finding: count = %d, want 0", role, got)
	}
	if got := rawFindingVisibleCount(t, ctx, pool, role, "finding", fx.localFindingID); got != 0 {
		t.Errorf("%s sees local-confidential finding: count = %d, want 0", role, got)
	}
	if got := rawFindingVisibleCount(t, ctx, pool, role, "finding_resolution", fx.resolutionID); got != 0 {
		t.Errorf("%s sees resolution on local-confidential finding: count = %d, want 0", role, got)
	}
}

// assertFindingGroupSeesGroupNotLocal: mise_group sees public + group
// findings but not local, and not the resolution (local-confidential parent).
func assertFindingGroupSeesGroupNotLocal(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fx findingRLSFixture) {
	t.Helper()
	const role = "mise_group"
	if got := rawFindingVisibleCount(t, ctx, pool, role, "finding", fx.publicFindingID); got != 1 {
		t.Errorf("%s does not see public finding: count = %d, want 1", role, got)
	}
	if got := rawFindingVisibleCount(t, ctx, pool, role, "finding", fx.groupFindingID); got != 1 {
		t.Errorf("%s does not see group-confidential finding: count = %d, want 1", role, got)
	}
	if got := rawFindingVisibleCount(t, ctx, pool, role, "finding", fx.localFindingID); got != 0 {
		t.Errorf("%s sees local-confidential finding: count = %d, want 0", role, got)
	}
	if got := rawFindingVisibleCount(t, ctx, pool, role, "finding_resolution", fx.resolutionID); got != 0 {
		t.Errorf("%s sees resolution on local-confidential finding: count = %d, want 0", role, got)
	}
}

// assertFindingLocalSeesAll: mise_local sees every finding and the resolution.
func assertFindingLocalSeesAll(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fx findingRLSFixture) {
	t.Helper()
	const role = "mise_local"
	if got := rawFindingVisibleCount(t, ctx, pool, role, "finding", fx.publicFindingID); got != 1 {
		t.Errorf("%s does not see public finding: count = %d, want 1", role, got)
	}
	if got := rawFindingVisibleCount(t, ctx, pool, role, "finding", fx.groupFindingID); got != 1 {
		t.Errorf("%s does not see group-confidential finding: count = %d, want 1", role, got)
	}
	if got := rawFindingVisibleCount(t, ctx, pool, role, "finding", fx.localFindingID); got != 1 {
		t.Errorf("%s does not see local-confidential finding: count = %d, want 1", role, got)
	}
	if got := rawFindingVisibleCount(t, ctx, pool, role, "finding_resolution", fx.resolutionID); got != 1 {
		t.Errorf("%s does not see resolution: count = %d, want 1", role, got)
	}
}

// assertFindingRawCountsMatchVisibility is the exhaustive table-driven
// oracle, mirroring graph_rls_test.go's assertGraphRawCountsMatchVisibility.
func assertFindingRawCountsMatchVisibility(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fx findingRLSFixture) {
	t.Helper()
	cases := []struct {
		role        string
		table       string
		rowID       uuid.UUID
		wantVisible bool
	}{
		// mise_public
		{role: "mise_public", table: "finding", rowID: fx.publicFindingID, wantVisible: true},
		{role: "mise_public", table: "finding", rowID: fx.groupFindingID, wantVisible: false},
		{role: "mise_public", table: "finding", rowID: fx.localFindingID, wantVisible: false},
		{role: "mise_public", table: "finding_resolution", rowID: fx.resolutionID, wantVisible: false},

		// mise_group
		{role: "mise_group", table: "finding", rowID: fx.publicFindingID, wantVisible: true},
		{role: "mise_group", table: "finding", rowID: fx.groupFindingID, wantVisible: true},
		{role: "mise_group", table: "finding", rowID: fx.localFindingID, wantVisible: false},
		{role: "mise_group", table: "finding_resolution", rowID: fx.resolutionID, wantVisible: false},

		// mise_local
		{role: "mise_local", table: "finding", rowID: fx.publicFindingID, wantVisible: true},
		{role: "mise_local", table: "finding", rowID: fx.groupFindingID, wantVisible: true},
		{role: "mise_local", table: "finding", rowID: fx.localFindingID, wantVisible: true},
		{role: "mise_local", table: "finding_resolution", rowID: fx.resolutionID, wantVisible: true},
	}
	for _, tc := range cases {
		got := rawFindingVisibleCount(t, ctx, pool, tc.role, tc.table, tc.rowID)
		want := 0
		if tc.wantVisible {
			want = 1
		}
		if got != want {
			t.Errorf("raw count on graph.%s (id=%v) as %s = %d, want %d", tc.table, tc.rowID, tc.role, got, want)
		}
	}
}

// rawFindingVisibleCount runs SET LOCAL ROLE role; SELECT count(*) FROM
// graph.<table> WHERE id = rowID — the findings counterpart of
// rawGraphVisibleCount (graph_rls_test.go). Folds permission-denied
// (SQLSTATE 42501) to 0.
func rawFindingVisibleCount(
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

// insertFindingForRLS inserts a finding with the given node_refs as the
// pool's connecting owner role (bypasses RLS on write) and returns its id
// and the trigger-computed access_tier.
func insertFindingForRLS(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool, refs []store.NodeRefJSON,
) (uuid.UUID, graph.Tier) {
	t.Helper()
	refsJSON, err := json.Marshal(refs)
	if err != nil {
		t.Fatalf("marshalling node_refs: %v", err)
	}
	const q = `
		INSERT INTO graph.finding (kind, severity, status, node_refs, dedup_key)
		VALUES ('gap', 'medium', 'open', $1, $2)
		RETURNING id, access_tier`
	var id uuid.UUID
	var tier string
	err = pool.QueryRow(ctx, q, refsJSON, "rls-direct-"+uuid.NewString()).Scan(&id, &tier)
	if err != nil {
		t.Fatalf("inserting finding for RLS: %v", err)
	}
	return id, graph.Tier(tier)
}
