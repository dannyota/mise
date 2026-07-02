//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"danny.vn/mise/internal/testdb"
	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/store"
)

func TestLedgerUpsertAndUnchanged(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	ledger := store.NewLedger(pool)

	corpusID := corpus.VNReg
	sourceID := "vbpl"
	externalID := uuid.NewString() // unique per run: the pool/container is shared across tests

	if err := ledger.Upsert(ctx, corpusID, sourceID, externalID, "hash-1", "discovered"); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	unchanged, err := ledger.Unchanged(ctx, corpusID, sourceID, externalID, "hash-1")
	if err != nil {
		t.Fatalf("Unchanged() error = %v", err)
	}
	if !unchanged {
		t.Error("Unchanged() = false, want true for the same hash")
	}

	changed, err := ledger.Unchanged(ctx, corpusID, sourceID, externalID, "hash-2")
	if err != nil {
		t.Fatalf("Unchanged() error = %v", err)
	}
	if changed {
		t.Error("Unchanged() = true, want false for a different hash")
	}

	// Re-upserting with the new hash refreshes the stored value.
	if err := ledger.Upsert(ctx, corpusID, sourceID, externalID, "hash-2", "fetched"); err != nil {
		t.Fatalf("second Upsert() error = %v", err)
	}
	unchanged, err = ledger.Unchanged(ctx, corpusID, sourceID, externalID, "hash-2")
	if err != nil {
		t.Fatalf("Unchanged() error = %v", err)
	}
	if !unchanged {
		t.Error("Unchanged() = false after re-upsert, want true for the refreshed hash")
	}
}

func TestLedgerUnchangedUnknownKeyIsFalse(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	ledger := store.NewLedger(pool)

	unchanged, err := ledger.Unchanged(ctx, corpus.VNReg, "vbpl", uuid.NewString(), "any-hash")
	if err != nil {
		t.Fatalf("Unchanged() error = %v", err)
	}
	if unchanged {
		t.Error("Unchanged() = true for a never-seen key, want false")
	}
}

func TestLedgerSetStateAndLinkDocument(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	ledger := store.NewLedger(pool)

	corpusID := corpus.VNReg
	sourceID := "vbpl"
	externalID := uuid.NewString()

	if err := ledger.Upsert(ctx, corpusID, sourceID, externalID, "hash-1", "discovered"); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	if err := ledger.SetState(ctx, corpusID, sourceID, externalID, "failed", "fetch timeout"); err != nil {
		t.Fatalf("SetState() error = %v", err)
	}

	var state string
	var lastError *string
	err := pool.QueryRow(ctx,
		`SELECT state, last_error FROM ingest.doc_ledger WHERE corpus_id = $1 AND source_id = $2 AND external_id = $3`,
		string(corpusID), sourceID, externalID,
	).Scan(&state, &lastError)
	if err != nil {
		t.Fatalf("querying doc_ledger: %v", err)
	}
	if state != "failed" {
		t.Errorf("state = %q, want %q", state, "failed")
	}
	if lastError == nil || *lastError != "fetch timeout" {
		t.Errorf("last_error = %v, want %q", lastError, "fetch timeout")
	}

	// Clearing the error (e.g. a subsequent retry succeeds) sets last_error back to NULL.
	if err := ledger.SetState(ctx, corpusID, sourceID, externalID, "fetched", ""); err != nil {
		t.Fatalf("SetState() clearing error = %v", err)
	}
	err = pool.QueryRow(ctx,
		`SELECT state, last_error FROM ingest.doc_ledger WHERE corpus_id = $1 AND source_id = $2 AND external_id = $3`,
		string(corpusID), sourceID, externalID,
	).Scan(&state, &lastError)
	if err != nil {
		t.Fatalf("querying doc_ledger after clear: %v", err)
	}
	if state != "fetched" {
		t.Errorf("state after clear = %q, want %q", state, "fetched")
	}
	if lastError != nil {
		t.Errorf("last_error after clear = %v, want nil", lastError)
	}

	docID := uuid.New()
	if err := ledger.LinkDocument(ctx, corpusID, sourceID, externalID, docID); err != nil {
		t.Fatalf("LinkDocument() error = %v", err)
	}

	var linked uuid.UUID
	err = pool.QueryRow(ctx,
		`SELECT document_id FROM ingest.doc_ledger WHERE corpus_id = $1 AND source_id = $2 AND external_id = $3`,
		string(corpusID), sourceID, externalID,
	).Scan(&linked)
	if err != nil {
		t.Fatalf("querying document_id: %v", err)
	}
	if linked != docID {
		t.Errorf("document_id = %v, want %v", linked, docID)
	}
}

func TestCursorRoundTrip(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	cursor := store.Cursor(pool)

	corpusID := corpus.VNReg
	sourceID := "cursor-test-" + uuid.NewString()
	keyword := "an toàn thông tin"

	got, err := cursor.Get(ctx, corpusID, sourceID, keyword)
	if err != nil {
		t.Fatalf("Get() before Set error = %v", err)
	}
	if !got.Equal(time.Unix(0, 0).UTC()) {
		t.Errorf("Get() before Set = %v, want epoch", got)
	}

	want := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	if err := cursor.Set(ctx, corpusID, sourceID, keyword, want); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	got, err = cursor.Get(ctx, corpusID, sourceID, keyword)
	if err != nil {
		t.Fatalf("Get() after Set error = %v", err)
	}
	if !got.Equal(want) {
		t.Errorf("Get() after Set = %v, want %v", got, want)
	}

	// Set again — round-trips to the new value (upsert, not insert-only).
	want2 := want.Add(24 * time.Hour)
	if err := cursor.Set(ctx, corpusID, sourceID, keyword, want2); err != nil {
		t.Fatalf("second Set() error = %v", err)
	}
	got, err = cursor.Get(ctx, corpusID, sourceID, keyword)
	if err != nil {
		t.Fatalf("Get() after second Set error = %v", err)
	}
	if !got.Equal(want2) {
		t.Errorf("Get() after second Set = %v, want %v", got, want2)
	}
}

func TestRunStartFinish(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	id, err := store.StartRun(ctx, pool, corpus.VNReg)
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	if id == uuid.Nil {
		t.Fatal("StartRun() returned the nil UUID")
	}

	var status string
	var finishedAt *time.Time
	err = pool.QueryRow(ctx, `SELECT status, finished_at FROM ingest.run WHERE id = $1`, id).Scan(&status, &finishedAt)
	if err != nil {
		t.Fatalf("querying run after start: %v", err)
	}
	if status != "running" {
		t.Errorf("status after start = %q, want %q", status, "running")
	}
	if finishedAt != nil {
		t.Errorf("finished_at after start = %v, want nil", finishedAt)
	}

	stats := map[string]any{"discovered": float64(3), "fetched": float64(2)}
	if err := store.FinishRun(ctx, pool, id, "success", stats); err != nil {
		t.Fatalf("FinishRun() error = %v", err)
	}

	var gotStats map[string]any
	err = pool.QueryRow(ctx, `SELECT status, finished_at, stats FROM ingest.run WHERE id = $1`, id).
		Scan(&status, &finishedAt, &gotStats)
	if err != nil {
		t.Fatalf("querying run after finish: %v", err)
	}
	if status != "success" {
		t.Errorf("status after finish = %q, want %q", status, "success")
	}
	if finishedAt == nil {
		t.Error("finished_at after finish = nil, want non-nil")
	}
	if gotStats["discovered"] != float64(3) || gotStats["fetched"] != float64(2) {
		t.Errorf("stats = %v, want map with discovered=3, fetched=2", gotStats)
	}
}

func TestRunFinishDefaultsNilStatsToEmptyObject(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	id, err := store.StartRun(ctx, pool, corpus.MYReg)
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	if err := store.FinishRun(ctx, pool, id, "success", nil); err != nil {
		t.Fatalf("FinishRun() with nil stats error = %v", err)
	}

	var gotStats map[string]any
	err = pool.QueryRow(ctx, `SELECT stats FROM ingest.run WHERE id = $1`, id).Scan(&gotStats)
	if err != nil {
		t.Fatalf("querying stats: %v", err)
	}
	if len(gotStats) != 0 {
		t.Errorf("stats = %v, want empty object for nil input", gotStats)
	}
}
