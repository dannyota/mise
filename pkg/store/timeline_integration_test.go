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

// TestListTimelineRespectsRoleTiers is the regression gate for the M10
// timeline 500: ListTimeline's UNION ALL is one statement, so a single arm
// touching a schema the role has no USAGE on used to fail the whole query
// with SQLSTATE 42501 (mise_public vs group_std). The corpus set must be
// tier-filtered per role BEFORE the query is built — public sees only
// public-corpus events and never errors; local sees internal events too.
func TestListTimelineRespectsRoleTiers(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	ts := store.NewTimelineStore(pool)

	when := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	cVN := newCorpus(t, pool, corpus.VNReg)
	vnDoc := mustUpsertVNRegDoc(t, ctx, cVN, "timeline-vn-"+uuid.NewString())
	mustInsertEvent(t, ctx, cVN, vnDoc, when)

	cGroup := newCorpus(t, pool, corpus.GroupStd)
	groupDoc := upsertSearchDoc(t, ctx, cGroup, corpus.GroupStd, corpus.TierGroupConfidential,
		"timeline-group-"+uuid.NewString(), nil)
	mustInsertEvent(t, ctx, cGroup, groupDoc, when)

	t.Run("mise_public lists without error and sees no internal event", func(t *testing.T) {
		page, err := ts.ListTimeline(ctx, "mise_public", store.TimelineListOpts{Limit: 100})
		if err != nil {
			t.Fatalf("ListTimeline(mise_public) error = %v", err)
		}
		if !containsTimelineDoc(page.Items, vnDoc) {
			t.Errorf("ListTimeline(mise_public) missing vn-reg event for %s", vnDoc)
		}
		if containsTimelineDoc(page.Items, groupDoc) {
			t.Errorf("ListTimeline(mise_public) leaked group-std event for %s", groupDoc)
		}
	})

	t.Run("mise_local sees both events", func(t *testing.T) {
		page, err := ts.ListTimeline(ctx, "mise_local", store.TimelineListOpts{Limit: 100})
		if err != nil {
			t.Fatalf("ListTimeline(mise_local) error = %v", err)
		}
		if !containsTimelineDoc(page.Items, vnDoc) {
			t.Errorf("ListTimeline(mise_local) missing vn-reg event for %s", vnDoc)
		}
		if !containsTimelineDoc(page.Items, groupDoc) {
			t.Errorf("ListTimeline(mise_local) missing group-std event for %s", groupDoc)
		}
	})

	t.Run("corpus filter outside the role's tier returns empty, not error", func(t *testing.T) {
		page, err := ts.ListTimeline(ctx, "mise_public", store.TimelineListOpts{
			Corpus: string(corpus.GroupStd), Limit: 100,
		})
		if err != nil {
			t.Fatalf("ListTimeline(mise_public, group-std) error = %v", err)
		}
		if len(page.Items) != 0 {
			t.Errorf("ListTimeline(mise_public, group-std) items = %d, want 0", len(page.Items))
		}
	})
}

// mustInsertEvent writes one amendment event for docID dated when.
func mustInsertEvent(t *testing.T, ctx context.Context, c *store.Corpus, docID uuid.UUID, when time.Time) {
	t.Helper()
	err := c.InsertAmendmentEvents(ctx, []store.AmendmentEvent{{
		TargetDocID: docID, Kind: "amended", Clause: "test", EventDate: when,
	}})
	if err != nil {
		t.Fatalf("InsertAmendmentEvents() fixture error = %v", err)
	}
}

// containsTimelineDoc reports whether any event targets docID.
func containsTimelineDoc(items []store.TimelineEvent, docID uuid.UUID) bool {
	for _, ev := range items {
		if ev.DocumentID != nil && *ev.DocumentID == docID {
			return true
		}
	}
	return false
}
