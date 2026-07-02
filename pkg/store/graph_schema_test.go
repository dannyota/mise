//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"danny.vn/mise/internal/testdb"
	"danny.vn/mise/pkg/graph"
)

// scanDocRef scans one graph.doc_ref row — id, corpus_id, ref_key, label,
// document_id, section_id, created_at, updated_at, in that column order —
// into a graph.DocRef. Every query in this file that RETURNING/SELECTs a
// doc_ref row uses exactly those columns in exactly that order.
func scanDocRef(row pgx.Row) (graph.DocRef, error) {
	var r graph.DocRef
	err := row.Scan(&r.ID, &r.CorpusID, &r.RefKey, &r.Label, &r.DocumentID, &r.SectionID, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

// scanEvidence scans one graph.relation_evidence row — id, edge_id,
// evidence_kind, confidence, rationale, in that column order — into a
// graph.Evidence.
func scanEvidence(row pgx.Row) (graph.Evidence, error) {
	var e graph.Evidence
	var kind string
	if err := row.Scan(&e.ID, &e.EdgeID, &kind, &e.Confidence, &e.Rationale); err != nil {
		return graph.Evidence{}, err
	}
	e.EvidenceKind = graph.EvidenceKind(kind)
	return e, nil
}

// insertDocRef inserts a graph.doc_ref row for corpusID/refKey and returns
// it as a graph.DocRef scanned straight from the row Postgres stored — a
// running proof that graph.DocRef's shape matches graph.doc_ref's columns.
// document_id/section_id are left NULL — the "unresolved ref" shape
// doc_ref_unresolved_idx (migrations/009_graph_tables.sql) exists to find;
// relation_edge.to_ref_id only needs a valid FK target, not a resolved one.
func insertDocRef(t *testing.T, ctx context.Context, pool *pgxpool.Pool, corpusID, refKey string) graph.DocRef {
	t.Helper()
	const q = `
		INSERT INTO graph.doc_ref (corpus_id, ref_key)
		VALUES ($1, $2)
		RETURNING id, corpus_id, ref_key, label, document_id, section_id, created_at, updated_at`
	r, err := scanDocRef(pool.QueryRow(ctx, q, corpusID, refKey))
	if err != nil {
		t.Fatalf("inserting graph.doc_ref (corpus %s): %v", corpusID, err)
	}
	return r
}

// insertEdge inserts a graph.relation_edge row from a synthetic
// from_document_id (relation_edge declares no FK on that column — see the
// migration's column comment) to toRefID, and returns the row's
// access_tier exactly as Postgres derived it.
func insertEdge(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	fromCorpusID, toCorpusID string, toRefID uuid.UUID, edgeType graph.EdgeType,
) graph.Tier {
	t.Helper()
	const q = `
		INSERT INTO graph.relation_edge (from_corpus_id, from_document_id, to_ref_id, to_corpus_id, edge_type)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING access_tier`
	var tier string
	err := pool.QueryRow(ctx, q, fromCorpusID, uuid.New(), toRefID, toCorpusID, string(edgeType)).Scan(&tier)
	if err != nil {
		t.Fatalf("inserting graph.relation_edge (from %s to %s): %v", fromCorpusID, toCorpusID, err)
	}
	return graph.Tier(tier)
}

// TestGraphSchemaTablesExist pins that migration 009 actually created the
// three graph tables the testdb harness's auto-run-all-migrations was
// supposed to apply — the base fact every other test in this file assumes.
func TestGraphSchemaTablesExist(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	for _, tbl := range []string{"doc_ref", "relation_edge", "relation_evidence"} {
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

// TestDocRefUnresolvedThenResolved inserts a doc_ref with no document_id
// (the forward-reference shape doc_ref_unresolved_idx targets), confirms
// graph.DocRef.Resolve reports it unresolved, then resolves it and
// re-selects — the "insert + re-select" idempotency check the brief calls
// for, since goose itself only ever applies migrations/009 once per
// database (tracked in its own version table) and re-invoking goose.Up
// mid-test isn't something the shared testdb.New harness exposes.
func TestDocRefUnresolvedThenResolved(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	refKey := "vn-reg:unresolved-" + uuid.NewString()

	before := insertDocRef(t, ctx, pool, "vn-reg", refKey)
	if before.RefKey != refKey {
		t.Errorf("RefKey = %q, want %q", before.RefKey, refKey)
	}
	if _, ok := before.Resolve(); ok {
		t.Error("Resolve() on a freshly-inserted doc_ref ok = true, want false (document_id is NULL)")
	}

	resolvedDocID := uuid.New()
	const updateQ = `
		UPDATE graph.doc_ref SET document_id = $1 WHERE id = $2
		RETURNING id, corpus_id, ref_key, label, document_id, section_id, created_at, updated_at`
	after, err := scanDocRef(pool.QueryRow(ctx, updateQ, resolvedDocID, before.ID))
	if err != nil {
		t.Fatalf("resolving doc_ref: %v", err)
	}

	node, ok := after.Resolve()
	if !ok {
		t.Fatal("Resolve() after setting document_id ok = false, want true")
	}
	if node.CorpusID != "vn-reg" || node.DocumentID != resolvedDocID {
		t.Errorf("Resolve() = %+v, want CorpusID=vn-reg DocumentID=%v", node, resolvedDocID)
	}
}

// TestRelationEdgeAccessTierLocalPolicyToGroupStdIsLocalConfidential is the
// milestone's headline confidentiality assertion: an edge from a
// local-confidential corpus to a group-confidential target must inherit
// the STRICTER of the two tiers, not the target's own (weaker) tier.
func TestRelationEdgeAccessTierLocalPolicyToGroupStdIsLocalConfidential(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	ref := insertDocRef(t, ctx, pool, "group-std", "group-std:ref-"+uuid.NewString())
	tier := insertEdge(t, ctx, pool, "local-policy", "group-std", ref.ID, graph.EdgeSatisfies)
	if tier != graph.TierLocalConfidential {
		t.Errorf("access_tier = %q, want %q", tier, graph.TierLocalConfidential)
	}
}

// TestRelationEdgeAccessTierVNRegToMYRegIsPublic is the low-sensitivity
// counterpart: both corpora are public, so the stricter-of-two tier must
// also resolve to public — proves the rule isn't a blanket
// escalate-to-confidential fallback.
func TestRelationEdgeAccessTierVNRegToMYRegIsPublic(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	ref := insertDocRef(t, ctx, pool, "my-reg", "my-reg:ref-"+uuid.NewString())
	tier := insertEdge(t, ctx, pool, "vn-reg", "my-reg", ref.ID, graph.EdgeCovers)
	if tier != graph.TierPublic {
		t.Errorf("access_tier = %q, want %q", tier, graph.TierPublic)
	}
}

// TestRelationEdgeAccessTierRejectsDirectInsert proves access_tier really is
// a GENERATED ALWAYS ... STORED column: no app code, however privileged,
// can supply its own value — Postgres rejects the statement outright rather
// than silently ignoring or overwriting the supplied value. This is what
// makes the tier DB-derived instead of merely DB-defaulted.
func TestRelationEdgeAccessTierRejectsDirectInsert(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	ref := insertDocRef(t, ctx, pool, "group-std", "group-std:ref-"+uuid.NewString())

	const q = `
		INSERT INTO graph.relation_edge
			(from_corpus_id, from_document_id, to_ref_id, to_corpus_id, edge_type, access_tier)
		VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := pool.Exec(ctx, q, "local-policy", uuid.New(), ref.ID, "group-std",
		string(graph.EdgeSatisfies), string(graph.TierPublic))
	if err == nil {
		t.Fatal("INSERT with an explicit access_tier value error = nil, want a generated-column error")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("error = %v, want errors.As(_, *pgconn.PgError) to succeed", err)
	}
	t.Logf("got expected generated-column rejection: SQLSTATE %s: %s", pgErr.Code, pgErr.Message)
}

// TestRelationEdgeAccessTierUnknownFromCorpusFailsClosed asserts that
// graph.corpus_tier's fail-closed ELSE branch (migrations/009_graph_tables.sql)
// really does feed the GENERATED access_tier column: an edge whose
// from_corpus_id is a value corpus_tier doesn't recognize must still land at
// the strictest tier, local-confidential — even though the to-side here is a
// known public corpus. A regression that flipped the ELSE branch to 'public'
// (fail-open) would pass every other test in this file but fail this one.
func TestRelationEdgeAccessTierUnknownFromCorpusFailsClosed(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	ref := insertDocRef(t, ctx, pool, "vn-reg", "vn-reg:ref-"+uuid.NewString())
	tier := insertEdge(t, ctx, pool, "nonexistent-corpus", "vn-reg", ref.ID, graph.EdgeDerives)
	if tier != graph.TierLocalConfidential {
		t.Errorf("access_tier = %q, want %q (fail-closed on unmapped from_corpus_id)", tier, graph.TierLocalConfidential)
	}
}

// TestRelationEdgeAccessTierToSideLocalPolicyIsLocalConfidential is the
// mirror of TestRelationEdgeAccessTierLocalPolicyToGroupStdIsLocalConfidential:
// there the FROM side was the stricter one; here the FROM side is public
// (vn-reg) and only the TO side (local-policy) is local-confidential. The
// generated tier must still come out local-confidential, proving
// graph.stricter_tier actually consults the TO side rather than merely
// reflecting whichever side happens to be FROM.
func TestRelationEdgeAccessTierToSideLocalPolicyIsLocalConfidential(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	ref := insertDocRef(t, ctx, pool, "local-policy", "local-policy:ref-"+uuid.NewString())
	tier := insertEdge(t, ctx, pool, "vn-reg", "local-policy", ref.ID, graph.EdgeImplements)
	if tier != graph.TierLocalConfidential {
		t.Errorf("access_tier = %q, want %q", tier, graph.TierLocalConfidential)
	}
}

// TestRelationEdgeAccessTierGroupStdToMYRegIsGroupConfidential exercises the
// middle tier: group-std ranks stricter than a public corpus but looser than
// a local-confidential one, and no other test in this file lands there. A
// regression that collapsed tier_rank's three-way ordering into a two-way
// (public vs. everything-else) one would still pass every other test here
// but fail this one.
func TestRelationEdgeAccessTierGroupStdToMYRegIsGroupConfidential(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	ref := insertDocRef(t, ctx, pool, "my-reg", "my-reg:ref-"+uuid.NewString())
	tier := insertEdge(t, ctx, pool, "group-std", "my-reg", ref.ID, graph.EdgeSatisfies)
	if tier != graph.TierGroupConfidential {
		t.Errorf("access_tier = %q, want %q", tier, graph.TierGroupConfidential)
	}
}

// TestRelationEdgeTriggerOverwritesMislabeledToCorpusID proves the
// relation_edge_set_to_corpus BEFORE trigger (migrations/009_graph_tables.sql)
// — not the app — is authoritative for to_corpus_id. The INSERT below
// deliberately lies: it claims to_corpus_id = 'vn-reg' (public) for a
// to_ref_id whose real doc_ref is 'local-policy' (local-confidential). If the
// trigger didn't overwrite NEW.to_corpus_id before the GENERATED access_tier
// is computed, this row would come back mislabeled with the app's looser
// claimed tier instead of the target's true, stricter one — exactly the
// mislabel-to-looser-tier hole this trigger closes.
func TestRelationEdgeTriggerOverwritesMislabeledToCorpusID(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	ref := insertDocRef(t, ctx, pool, "local-policy", "local-policy:ref-"+uuid.NewString())

	const q = `
		INSERT INTO graph.relation_edge (from_corpus_id, from_document_id, to_ref_id, to_corpus_id, edge_type)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING to_corpus_id, access_tier`
	var gotToCorpus, gotTier string
	err := pool.QueryRow(ctx, q, "vn-reg", uuid.New(), ref.ID, "vn-reg", string(graph.EdgeCovers)).
		Scan(&gotToCorpus, &gotTier)
	if err != nil {
		t.Fatalf("inserting graph.relation_edge with a mislabeled to_corpus_id: %v", err)
	}
	if gotToCorpus != "local-policy" {
		t.Errorf("to_corpus_id = %q, want %q (trigger must overwrite the app's claimed value with the doc_ref's real corpus)",
			gotToCorpus, "local-policy")
	}
	if graph.Tier(gotTier) != graph.TierLocalConfidential {
		t.Errorf("access_tier = %q, want %q", gotTier, graph.TierLocalConfidential)
	}
}

// TestRelationEvidenceRoundTrips exercises the third graph table: a
// relation_evidence row FK'd to a real edge, scanned back into a
// graph.Evidence — proving the join key and the evidence_kind CHECK
// constraint both accept a valid row and that graph.Evidence's shape
// matches graph.relation_evidence's columns.
func TestRelationEvidenceRoundTrips(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	ref := insertDocRef(t, ctx, pool, "my-reg", "my-reg:ref-"+uuid.NewString())
	const edgeQ = `
		INSERT INTO graph.relation_edge (from_corpus_id, from_document_id, to_ref_id, to_corpus_id, edge_type)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`
	var edgeID uuid.UUID
	err := pool.QueryRow(ctx, edgeQ, "vn-reg", uuid.New(), ref.ID, "my-reg", string(graph.EdgeCovers)).Scan(&edgeID)
	if err != nil {
		t.Fatalf("inserting graph.relation_edge fixture: %v", err)
	}

	const evidenceQ = `
		INSERT INTO graph.relation_evidence (edge_id, evidence_kind, confidence, rationale)
		VALUES ($1, $2, 0.87, 'shared control language')
		RETURNING id, edge_id, evidence_kind, confidence, rationale`
	e, err := scanEvidence(pool.QueryRow(ctx, evidenceQ, edgeID, string(graph.EvidenceModelClassification)))
	if err != nil {
		t.Fatalf("inserting graph.relation_evidence: %v", err)
	}
	if e.EdgeID != edgeID {
		t.Errorf("EdgeID = %v, want %v", e.EdgeID, edgeID)
	}
	if e.EvidenceKind != graph.EvidenceModelClassification {
		t.Errorf("EvidenceKind = %q, want %q", e.EvidenceKind, graph.EvidenceModelClassification)
	}
	if e.Confidence != 0.87 {
		t.Errorf("Confidence = %v, want 0.87", e.Confidence)
	}
}
