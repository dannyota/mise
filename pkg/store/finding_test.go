//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"danny.vn/mise/internal/testdb"
	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/store"
)

// TestFindingTablesExist pins that migration 012 created the three findings
// tables — the base fact every other test in this file assumes.
func TestFindingTablesExist(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	for _, tbl := range []string{"finding", "finding_resolution", "action_plan"} {
		const q = `SELECT EXISTS (
			SELECT 1 FROM information_schema.tables WHERE table_schema = 'graph' AND table_name = $1
		)`
		var exists bool
		if err := pool.QueryRow(ctx, q, tbl).Scan(&exists); err != nil {
			t.Fatalf("checking graph.%s exists: %v", tbl, err)
		}
		if !exists {
			t.Errorf("graph.%s does not exist after migrations", tbl)
		}
	}
}

// TestCreateFindingRoundTrip inserts a finding and reads it back by
// querying the table directly, proving the store round-trips correctly.
func TestCreateFindingRoundTrip(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	fs := store.NewFindingStore(pool)

	dedupKey := "test-roundtrip-" + uuid.NewString()
	refs := []store.NodeRefJSON{
		{CorpusID: "vn-reg", DocumentID: uuid.New()},
	}

	f := store.Finding{
		Kind:       "gap",
		Severity:   "high",
		Status:     "open",
		NodeRefs:   refs,
		Evidence:   json.RawMessage(`{"detail":"test"}`),
		DetectedAt: time.Now().Truncate(time.Microsecond),
		DedupKey:   dedupKey,
	}

	id, err := fs.CreateFinding(ctx, f)
	if err != nil {
		t.Fatalf("CreateFinding: %v", err)
	}
	if id == uuid.Nil {
		t.Fatal("CreateFinding returned nil UUID")
	}

	// Read back directly to verify.
	const q = `SELECT kind, severity, status, access_tier, dedup_key FROM graph.finding WHERE id = $1`
	var kind, severity, status, tier, dk string
	if err := pool.QueryRow(ctx, q, id).Scan(&kind, &severity, &status, &tier, &dk); err != nil {
		t.Fatalf("reading back finding: %v", err)
	}
	if kind != "gap" {
		t.Errorf("kind = %q, want gap", kind)
	}
	if severity != "high" {
		t.Errorf("severity = %q, want high", severity)
	}
	if status != "open" {
		t.Errorf("status = %q, want open", status)
	}
	if dk != dedupKey {
		t.Errorf("dedup_key = %q, want %q", dk, dedupKey)
	}
}

// TestCreateFindingDedup proves ON CONFLICT DO NOTHING + fallback lookup:
// a second call with the same dedup_key returns the same id without error.
func TestCreateFindingDedup(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	fs := store.NewFindingStore(pool)

	dedupKey := "test-dedup-" + uuid.NewString()
	f := store.Finding{
		Kind:       "conflict",
		Severity:   "medium",
		Status:     "open",
		NodeRefs:   []store.NodeRefJSON{{CorpusID: "vn-reg", DocumentID: uuid.New()}},
		DetectedAt: time.Now(),
		DedupKey:   dedupKey,
	}

	id1, err := fs.CreateFinding(ctx, f)
	if err != nil {
		t.Fatalf("first CreateFinding: %v", err)
	}

	id2, err := fs.CreateFinding(ctx, f)
	if err != nil {
		t.Fatalf("second CreateFinding (dedup): %v", err)
	}
	if id1 != id2 {
		t.Errorf("dedup returned different id: %v vs %v", id1, id2)
	}
}

// TestFindingTriggerMixedTier proves the BEFORE INSERT trigger computes
// access_tier as the stricter-of-all-node_refs corpora. A finding
// spanning local-policy (local-confidential) + my-reg (public) must land
// at local-confidential.
func TestFindingTriggerMixedTier(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	fs := store.NewFindingStore(pool)

	refs := []store.NodeRefJSON{
		{CorpusID: "local-policy", DocumentID: uuid.New()},
		{CorpusID: "my-reg", DocumentID: uuid.New()},
	}
	id, err := fs.CreateFinding(ctx, store.Finding{
		Kind:       "gap",
		Severity:   "medium",
		Status:     "open",
		NodeRefs:   refs,
		DetectedAt: time.Now(),
		DedupKey:   "test-mixed-tier-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("CreateFinding: %v", err)
	}

	const q = `SELECT access_tier FROM graph.finding WHERE id = $1`
	var tier string
	if err := pool.QueryRow(ctx, q, id).Scan(&tier); err != nil {
		t.Fatalf("reading access_tier: %v", err)
	}
	if tier != string(graph.TierLocalConfidential) {
		t.Errorf("access_tier = %q, want %q", tier, graph.TierLocalConfidential)
	}
}

// TestFindingTriggerPublicOnly proves a finding with only public-tier
// nodes gets access_tier='public', not a blanket fail-closed.
func TestFindingTriggerPublicOnly(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	fs := store.NewFindingStore(pool)

	refs := []store.NodeRefJSON{
		{CorpusID: "vn-reg", DocumentID: uuid.New()},
		{CorpusID: "my-reg", DocumentID: uuid.New()},
	}
	id, err := fs.CreateFinding(ctx, store.Finding{
		Kind:       "staleness",
		Severity:   "low",
		Status:     "open",
		NodeRefs:   refs,
		DetectedAt: time.Now(),
		DedupKey:   "test-public-only-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("CreateFinding: %v", err)
	}

	const q = `SELECT access_tier FROM graph.finding WHERE id = $1`
	var tier string
	if err := pool.QueryRow(ctx, q, id).Scan(&tier); err != nil {
		t.Fatalf("reading access_tier: %v", err)
	}
	if tier != string(graph.TierPublic) {
		t.Errorf("access_tier = %q, want %q", tier, graph.TierPublic)
	}
}

// TestFindingTriggerEmptyNodeRefs proves empty node_refs fails closed to
// 'local-confidential'.
func TestFindingTriggerEmptyNodeRefs(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	fs := store.NewFindingStore(pool)

	id, err := fs.CreateFinding(ctx, store.Finding{
		Kind:       "gap",
		Severity:   "info",
		Status:     "open",
		NodeRefs:   []store.NodeRefJSON{},
		DetectedAt: time.Now(),
		DedupKey:   "test-empty-refs-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("CreateFinding: %v", err)
	}

	const q = `SELECT access_tier FROM graph.finding WHERE id = $1`
	var tier string
	if err := pool.QueryRow(ctx, q, id).Scan(&tier); err != nil {
		t.Fatalf("reading access_tier: %v", err)
	}
	if tier != string(graph.TierLocalConfidential) {
		t.Errorf("access_tier = %q, want %q (fail-closed on empty node_refs)", tier, graph.TierLocalConfidential)
	}
}

// TestFindingsByNodeReturnsVisibleFindings proves FindingsByNode returns
// RLS-scoped results: a public-tier finding is visible to mise_public,
// and a local-confidential finding is not.
func TestFindingsByNodeReturnsVisibleFindings(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	fs := store.NewFindingStore(pool)

	docID := uuid.New()
	ref := graph.NodeRef{CorpusID: "vn-reg", DocumentID: docID}

	// Insert a public finding referencing this node.
	pubDedupKey := "test-visible-pub-" + uuid.NewString()
	_, err := fs.CreateFinding(ctx, store.Finding{
		Kind:       "gap",
		Severity:   "medium",
		Status:     "open",
		NodeRefs:   []store.NodeRefJSON{{CorpusID: "vn-reg", DocumentID: docID}},
		DetectedAt: time.Now(),
		DedupKey:   pubDedupKey,
	})
	if err != nil {
		t.Fatalf("CreateFinding (public): %v", err)
	}

	// Insert a local-confidential finding referencing the same node.
	localDedupKey := "test-visible-local-" + uuid.NewString()
	_, err = fs.CreateFinding(ctx, store.Finding{
		Kind:     "conflict",
		Severity: "high",
		Status:   "open",
		NodeRefs: []store.NodeRefJSON{
			{CorpusID: "vn-reg", DocumentID: docID},
			{CorpusID: "local-policy", DocumentID: uuid.New()},
		},
		DetectedAt: time.Now(),
		DedupKey:   localDedupKey,
	})
	if err != nil {
		t.Fatalf("CreateFinding (local): %v", err)
	}

	// As mise_public: should see only the public finding.
	pubFindings, err := fs.FindingsByNode(ctx, "mise_public", ref, store.FindingOpts{})
	if err != nil {
		t.Fatalf("FindingsByNode as mise_public: %v", err)
	}
	if len(pubFindings) != 1 {
		t.Errorf("mise_public sees %d findings, want 1", len(pubFindings))
	} else if pubFindings[0].DedupKey != pubDedupKey {
		t.Errorf("mise_public sees dedup_key %q, want %q", pubFindings[0].DedupKey, pubDedupKey)
	}

	// As mise_local: should see both.
	localFindings, err := fs.FindingsByNode(ctx, "mise_local", ref, store.FindingOpts{})
	if err != nil {
		t.Fatalf("FindingsByNode as mise_local: %v", err)
	}
	if len(localFindings) != 2 {
		t.Errorf("mise_local sees %d findings, want 2", len(localFindings))
	}
}

// TestCreateResolutionRoundTrip inserts a resolution and reads it back.
func TestCreateResolutionRoundTrip(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	fs := store.NewFindingStore(pool)

	// Create a parent finding first.
	findingID, err := fs.CreateFinding(ctx, store.Finding{
		Kind:       "gap",
		Severity:   "medium",
		Status:     "open",
		NodeRefs:   []store.NodeRefJSON{{CorpusID: "vn-reg", DocumentID: uuid.New()}},
		DetectedAt: time.Now(),
		DedupKey:   "test-resolution-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("CreateFinding: %v", err)
	}

	due := time.Now().Add(7 * 24 * time.Hour).Truncate(time.Microsecond)
	resID, err := fs.CreateResolution(ctx, findingID, store.Resolution{
		Disposition: "map",
		OwnerDept:   "compliance",
		OwnerRole:   "analyst",
		Status:      "open",
		Rationale:   "needs mapping to local controls",
		DueDate:     &due,
	})
	if err != nil {
		t.Fatalf("CreateResolution: %v", err)
	}
	if resID == uuid.Nil {
		t.Fatal("CreateResolution returned nil UUID")
	}

	// Read back directly to verify.
	const q = `SELECT finding_id, disposition, owner_department, owner_role, status, rationale
		FROM graph.finding_resolution WHERE id = $1`
	var fid uuid.UUID
	var disp, dept, role, status, rationale string
	if err := pool.QueryRow(ctx, q, resID).Scan(&fid, &disp, &dept, &role, &status, &rationale); err != nil {
		t.Fatalf("reading back resolution: %v", err)
	}
	if fid != findingID {
		t.Errorf("finding_id = %v, want %v", fid, findingID)
	}
	if disp != "map" {
		t.Errorf("disposition = %q, want map", disp)
	}
	if dept != "compliance" {
		t.Errorf("owner_department = %q, want compliance", dept)
	}
}
