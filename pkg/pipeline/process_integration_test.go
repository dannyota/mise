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
	"danny.vn/mise/pkg/parse/law"
	"danny.vn/mise/pkg/rag/embed"
	"danny.vn/mise/pkg/store"
)

// mustUpsertVNRegDoc inserts a minimal, valid vn_reg document fixture keyed
// on docNumber via c and returns its id — this file's and
// sweep_integration_test.go's shared fixture helper.
func mustUpsertVNRegDoc(t *testing.T, ctx context.Context, c *store.Corpus, docNumber string) uuid.UUID {
	t.Helper()
	id, err := c.UpsertDocument(ctx, store.Document{
		CorpusID: string(corpus.VNReg), Title: "Fixture Doc " + docNumber, DocNumber: docNumber,
		Language: "vi", ValidityStatus: ingest.StatusInForce, AccessTier: string(corpus.TierPublic),
		ObservedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("UpsertDocument() fixture error = %v", err)
	}
	return id
}

// TestIndexReappliesIncomingEventsAfterTargetReindex pins the C2 fix:
// UpsertDocument's update path overwrites validity_status with the
// source-derived value on every call, which would otherwise silently regress
// a document's event-derived amended/superseded/repealed status whenever it
// gets re-indexed (e.g. the source republishes it with a metadata change
// that advances its discovery fingerprint).
func TestIndexReappliesIncomingEventsAfterTargetReindex(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	desc, ok := corpus.Get(corpus.VNReg)
	if !ok {
		t.Fatal("corpus.Get(vn-reg): not registered")
	}
	a := NewActivities(Deps{Pool: pool, Embedder: embed.NewFake()})
	c, err := store.NewCorpus(pool, desc)
	if err != nil {
		t.Fatalf("NewCorpus() error = %v", err)
	}

	docNumber := "reindex-target-" + uuid.NewString()
	tree := []*law.Node{{Kind: "dieu", Label: "Điều 1", Content: "nội dung điều một", CitationPath: "dieu-1"}}
	src := ingest.DiscoveredDoc{
		SourceID: "fixture", ExternalID: "target-1", Number: docNumber,
		Title: "Original Title", DetailURL: "https://fixture.test/target-1",
		IssuedAt:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		EffectiveAt: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), // past: MapValidity -> in_force
	}

	docID, err := a.index(ctx, desc, src, tree, "fallback", uuid.NewString(), "")
	if err != nil {
		t.Fatalf("first index() error = %v", err)
	}
	if status, err := c.GetValidity(ctx, docID); err != nil || status != ingest.StatusInForce {
		t.Fatalf("ValidityStatus after first index() = %q, %v, want %q, nil", status, err, ingest.StatusInForce)
	}

	// Simulate an amendment event recorded against docID by some other,
	// already-indexed amending document — applyRelations' own write path,
	// reproduced directly since a second full fixture document isn't this
	// test's concern.
	amendingID := uuid.New() // amending_doc_id has no FK; a plausible foreign id is fine
	event := store.AmendmentEvent{
		TargetDocID: docID, AmendingDocID: &amendingID, Kind: ingest.StatusAmended,
		EventDate: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), // past: due immediately
	}
	if err := c.InsertAmendmentEvents(ctx, []store.AmendmentEvent{event}); err != nil {
		t.Fatalf("InsertAmendmentEvents() error = %v", err)
	}
	if _, err := c.TransitionValidity(ctx, docID, func(string) string { return ingest.StatusAmended }); err != nil {
		t.Fatalf("seeding TransitionValidity() error = %v", err)
	}
	if status, err := c.GetValidity(ctx, docID); err != nil || status != ingest.StatusAmended {
		t.Fatalf("seeded ValidityStatus = %q, %v, want %q, nil", status, err, ingest.StatusAmended)
	}

	// Re-index the SAME document: a changed Title advances the discovery
	// fingerprint in production, forcing UpsertDocument down its update
	// path. Without the C2 fix this overwrites validity_status back to
	// "in_force" (MapValidity has no better signal from this fixture).
	src.Title = "Updated Title"
	reDocID, err := a.index(ctx, desc, src, tree, "fallback", uuid.NewString(), "")
	if err != nil {
		t.Fatalf("second index() error = %v", err)
	}
	if reDocID != docID {
		t.Fatalf("second index() docID = %v, want unchanged %v (same doc_number)", reDocID, docID)
	}

	detail, err := c.GetDocument(ctx, "", docID)
	if err != nil {
		t.Fatalf("GetDocument() after re-index error = %v", err)
	}
	if detail.Doc.ValidityStatus != ingest.StatusAmended {
		t.Errorf("ValidityStatus after re-index = %q, want %q (event-derived status must survive re-index)",
			detail.Doc.ValidityStatus, ingest.StatusAmended)
	}
	if len(detail.Events) != 1 || detail.Events[0].Kind != ingest.StatusAmended {
		t.Errorf("Events after re-index = %+v, want exactly 1 event with Kind %q", detail.Events, ingest.StatusAmended)
	}
}
