//go:build integration

package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"danny.vn/mise/internal/testdb"
	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/ingest"
	"danny.vn/mise/pkg/store"
)

// TestApplyDueEventsSweepsPastDueOnly drives the real ApplyDueEvents activity
// over two amendment events on two different target documents: one dated in
// the future (must not transition — TransitionAt's own date gate), one dated
// in the past (must transition once swept). This pins the C3 fix: a
// future-dated event recorded by applyRelations at index time is otherwise
// never revisited once its event_date actually arrives, because nothing
// re-drives TransitionValidity for it after the indexing run that recorded
// it returns.
func TestApplyDueEventsSweepsPastDueOnly(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	desc, ok := corpus.Get(corpus.VNReg)
	if !ok {
		t.Fatal("corpus.Get(vn-reg): not registered")
	}
	c, err := store.NewCorpus(pool, desc)
	if err != nil {
		t.Fatalf("NewCorpus() error = %v", err)
	}
	a := NewActivities(Deps{Pool: pool})

	futureTarget := mustUpsertVNRegDoc(t, ctx, c, "sweep-future-"+uuid.NewString())
	pastTarget := mustUpsertVNRegDoc(t, ctx, c, "sweep-past-"+uuid.NewString())
	now := time.Now().UTC()

	events := []store.AmendmentEvent{
		{TargetDocID: futureTarget, Kind: ingest.StatusAmended, EventDate: now.Add(24 * time.Hour)},
		{TargetDocID: pastTarget, Kind: ingest.StatusAmended, EventDate: now.Add(-24 * time.Hour)},
	}
	if err := c.InsertAmendmentEvents(ctx, events); err != nil {
		t.Fatalf("InsertAmendmentEvents() error = %v", err)
	}

	n, err := a.ApplyDueEvents(ctx, IngestParams{Corpus: string(corpus.VNReg)})
	if err != nil {
		t.Fatalf("ApplyDueEvents() error = %v", err)
	}
	if n != 1 {
		t.Errorf("ApplyDueEvents() = %d, want 1 (only the past-dated event transitions)", n)
	}

	futureStatus, err := c.GetValidity(ctx, futureTarget)
	if err != nil {
		t.Fatalf("GetValidity(future) error = %v", err)
	}
	if futureStatus != ingest.StatusInForce {
		t.Errorf("future target validity = %q, want unchanged %q (not due yet)", futureStatus, ingest.StatusInForce)
	}

	pastStatus, err := c.GetValidity(ctx, pastTarget)
	if err != nil {
		t.Fatalf("GetValidity(past) error = %v", err)
	}
	if pastStatus != ingest.StatusAmended {
		t.Errorf("past target validity = %q, want %q (swept)", pastStatus, ingest.StatusAmended)
	}

	// Sweeping again is a no-op count: the already-applied past event no
	// longer changes anything (TransitionValidity's write is a no-op once
	// the status matches), and the future event still isn't due.
	n2, err := a.ApplyDueEvents(ctx, IngestParams{Corpus: string(corpus.VNReg)})
	if err != nil {
		t.Fatalf("second ApplyDueEvents() error = %v", err)
	}
	if n2 != 0 {
		t.Errorf("second ApplyDueEvents() = %d, want 0 (idempotent re-sweep)", n2)
	}
}
