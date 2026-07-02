//go:build integration

package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"danny.vn/mise/internal/testdb"
	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/ingest"
	"danny.vn/mise/pkg/store"
)

func TestTransitionValidityAppliesNextUnderRowLock(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	c := newCorpus(t, pool, corpus.VNReg)

	docID := mustUpsertVNRegDoc(t, ctx, c, "transition-basic-"+uuid.NewString())

	got, err := c.TransitionValidity(ctx, docID, func(current string) string {
		if current != "in_force" {
			t.Errorf("next() current = %q, want %q (the fixture's starting status)", current, "in_force")
		}
		return "amended"
	})
	if err != nil {
		t.Fatalf("TransitionValidity() error = %v", err)
	}
	if got != "amended" {
		t.Errorf("TransitionValidity() = %q, want %q", got, "amended")
	}

	stored, err := c.GetValidity(ctx, docID)
	if err != nil {
		t.Fatalf("GetValidity() error = %v", err)
	}
	if stored != "amended" {
		t.Errorf("GetValidity() after transition = %q, want %q", stored, "amended")
	}
}

func TestTransitionValidityUnknownDocumentIsNotFound(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	c := newCorpus(t, pool, corpus.VNReg)

	_, err := c.TransitionValidity(ctx, uuid.New(), func(current string) string { return current })
	if !errors.Is(err, store.ErrDocumentNotFound) {
		t.Errorf("TransitionValidity() on an unknown id error = %v, want errors.Is(_, store.ErrDocumentNotFound)", err)
	}
}

// TestTransitionValidityConvergesOnRepealedRegardlessOfOrder races two
// TransitionValidity calls — one applying an "amended" event, the other a
// "repealed" event — against the same document, simulating two ProcessDoc
// activities resolving overlapping amendment relations onto one target. The
// row lock (SELECT ... FOR UPDATE) serializes them, so whichever transition
// runs second always sees the first's already-committed status rather than
// clobbering it. Per ingest.Transition, repealed is absorbing both ways
// (current == repealed stays repealed; eventKind == repealed always wins),
// so the converged final status must be "repealed" no matter which
// goroutine's SELECT wins the lock. Run several independent iterations since
// the actual interleaving is nondeterministic.
func TestTransitionValidityConvergesOnRepealedRegardlessOfOrder(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	c := newCorpus(t, pool, corpus.VNReg)
	now := time.Now().UTC()
	past := now.Add(-time.Hour)

	const iterations = 20
	for i := range iterations {
		docID := mustUpsertVNRegDoc(t, ctx, c, "transition-race-"+uuid.NewString())

		var wg sync.WaitGroup
		var amendedErr, repealedErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, amendedErr = c.TransitionValidity(ctx, docID, func(current string) string {
				return ingest.TransitionAt(current, ingest.StatusAmended, past, now)
			})
		}()
		go func() {
			defer wg.Done()
			_, repealedErr = c.TransitionValidity(ctx, docID, func(current string) string {
				return ingest.TransitionAt(current, ingest.StatusRepealed, past, now)
			})
		}()
		wg.Wait()

		if amendedErr != nil {
			t.Errorf("iteration %d: amended TransitionValidity() error = %v", i, amendedErr)
		}
		if repealedErr != nil {
			t.Errorf("iteration %d: repealed TransitionValidity() error = %v", i, repealedErr)
		}

		got, err := c.GetValidity(ctx, docID)
		if err != nil {
			t.Fatalf("iteration %d: GetValidity() error = %v", i, err)
		}
		if got != ingest.StatusRepealed {
			t.Errorf("iteration %d: final validity_status = %q, want %q (repealed is terminal/always-wins)",
				i, got, ingest.StatusRepealed)
		}
	}
}
