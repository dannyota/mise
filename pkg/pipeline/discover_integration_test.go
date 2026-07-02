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
)

// TestDiscoverPacesBetweenSourcesSkippingAfterLast drives the real Discover
// activity (cursor/ledger reads need a real pool, hence integration) over
// fake sources that return no documents, so the only meaningful latency is
// the Fix-3 pacing itself. It checks the wiring both ways: pacing does
// happen between multiple sources, and it does NOT happen after the (or a)
// last source.
func TestDiscoverPacesBetweenSourcesSkippingAfterLast(t *testing.T) {
	pool := testdb.New(t)
	const pace = 150 * time.Millisecond

	run := func(t *testing.T, n int) time.Duration {
		t.Helper()
		sources := make([]ingest.Source, n)
		for i := range sources {
			sources[i] = &fakeSource{id: "pace-src-" + uuid.NewString()}
		}
		a := NewActivities(Deps{
			Pool:               pool,
			PaceBetweenSources: pace,
			Sources:            map[corpus.ID][]ingest.Source{corpus.VNReg: sources},
		})

		start := time.Now()
		if _, err := a.Discover(context.Background(), IngestParams{Corpus: string(corpus.VNReg)}); err != nil {
			t.Fatalf("Discover() error = %v", err)
		}
		return time.Since(start)
	}

	t.Run("single source: nothing to pace after the last (and only) one", func(t *testing.T) {
		if got := run(t, 1); got >= pace {
			t.Errorf("Discover() with 1 source took %v, want < %v (no pace after the last source)", got, pace)
		}
	})

	t.Run("three sources: paces between each adjacent pair", func(t *testing.T) {
		if got := run(t, 3); got < 2*pace {
			t.Errorf("Discover() with 3 sources took %v, want >= %v (two pace intervals)", got, 2*pace)
		}
	})
}
