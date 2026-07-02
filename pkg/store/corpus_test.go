//go:build integration

package store_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"danny.vn/mise/internal/testdb"
	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/store"
)

// newCorpus returns a store.Corpus bound to id's registered schema, failing
// the test immediately if the descriptor or construction is somehow bad.
func newCorpus(t *testing.T, pool *pgxpool.Pool, id corpus.ID) *store.Corpus {
	t.Helper()
	desc, ok := corpus.Get(id)
	if !ok {
		t.Fatalf("corpus.Get(%s): not registered", id)
	}
	c, err := store.NewCorpus(pool, desc)
	if err != nil {
		t.Fatalf("NewCorpus(%s): %v", id, err)
	}
	return c
}

// mustUpsertVNRegDoc inserts a minimal, valid vn_reg document fixture keyed
// on docNumber and returns its id.
func mustUpsertVNRegDoc(t *testing.T, ctx context.Context, c *store.Corpus, docNumber string) uuid.UUID {
	t.Helper()
	id, err := c.UpsertDocument(ctx, store.Document{
		CorpusID:       string(corpus.VNReg),
		Title:          "Fixture Doc " + docNumber,
		DocNumber:      docNumber,
		Language:       "vi",
		ValidityStatus: "in_force",
		AccessTier:     string(corpus.TierPublic),
		ObservedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("UpsertDocument() fixture error = %v", err)
	}
	return id
}

// countSections returns the live row count in schema.section for docID,
// bypassing store.Corpus entirely so ReplaceSections' rollback guarantee is
// checked independently of the code under test.
func countSections(t *testing.T, ctx context.Context, pool *pgxpool.Pool, schema string, docID uuid.UUID) int {
	t.Helper()
	var n int
	q := `SELECT count(*) FROM ` + schema + `.section WHERE document_id = $1`
	if err := pool.QueryRow(ctx, q, docID).Scan(&n); err != nil {
		t.Fatalf("counting sections: %v", err)
	}
	return n
}

// setSectionCreatedAt directly rewrites one section's created_at, bypassing
// the store API. Sections order by their stamped `position` column
// (ReplaceSections, migrations/006_search_and_write_keys.sql), not
// created_at — this lets a test set created_at in the opposite order from
// position and prove position, not created_at, governs GetDocument's
// section order. Scoped to docID (not just citationPath): the test
// container is a shared singleton, and other tests reuse the same
// citation_path text for their own, unrelated documents.
func setSectionCreatedAt(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	schema string, docID uuid.UUID, citationPath string, at time.Time,
) {
	t.Helper()
	q := `UPDATE ` + schema + `.section SET created_at = $3 WHERE document_id = $1 AND citation_path = $2`
	if _, err := pool.Exec(ctx, q, docID, citationPath, at); err != nil {
		t.Fatalf("backdating section %q: %v", citationPath, err)
	}
}

func TestNewCorpusRejectsUnregisteredSchema(t *testing.T) {
	pool := testdb.New(t)
	if _, err := store.NewCorpus(pool, corpus.Descriptor{SchemaName: "not_a_real_schema"}); err == nil {
		t.Error("NewCorpus() with an unregistered schema error = nil, want error")
	}
}

func TestUpsertDocumentByNumberUpdatesInPlace(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	c := newCorpus(t, pool, corpus.VNReg)

	docNumber := "upsert-number-" + uuid.NewString()
	d := store.Document{
		CorpusID:       string(corpus.VNReg),
		Title:          "Original Title",
		DocNumber:      docNumber,
		Language:       "vi",
		ValidityStatus: "in_force",
		AccessTier:     string(corpus.TierPublic),
		ObservedAt:     time.Now().UTC(),
	}

	id1, err := c.UpsertDocument(ctx, d)
	if err != nil {
		t.Fatalf("first UpsertDocument() error = %v", err)
	}

	d.Title = "Updated Title"
	id2, err := c.UpsertDocument(ctx, d)
	if err != nil {
		t.Fatalf("second UpsertDocument() error = %v", err)
	}
	if id2 != id1 {
		t.Errorf("UpsertDocument() re-upsert id = %v, want %v (same row)", id2, id1)
	}

	detail, err := c.GetDocument(ctx, "", id1)
	if err != nil {
		t.Fatalf("GetDocument() error = %v", err)
	}
	if detail.Doc.Title != "Updated Title" {
		t.Errorf("Title after re-upsert = %q, want %q", detail.Doc.Title, "Updated Title")
	}
}

func TestUpsertDocumentBySourceURLWhenNumberEmpty(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	c := newCorpus(t, pool, corpus.VNReg)

	sourceURL := "https://example.gov.vn/doc/" + uuid.NewString()
	d := store.Document{
		CorpusID:       string(corpus.VNReg),
		Title:          "Original Title",
		SourceURL:      sourceURL,
		Language:       "vi",
		ValidityStatus: "in_force",
		AccessTier:     string(corpus.TierPublic),
		ObservedAt:     time.Now().UTC(),
	}

	id1, err := c.UpsertDocument(ctx, d)
	if err != nil {
		t.Fatalf("first UpsertDocument() error = %v", err)
	}

	d.Title = "Updated Title"
	id2, err := c.UpsertDocument(ctx, d)
	if err != nil {
		t.Fatalf("second UpsertDocument() error = %v", err)
	}
	if id2 != id1 {
		t.Errorf("UpsertDocument() re-upsert by source_url id = %v, want %v (same row)", id2, id1)
	}

	detail, err := c.GetDocument(ctx, "", id1)
	if err != nil {
		t.Fatalf("GetDocument() error = %v", err)
	}
	if detail.Doc.Title != "Updated Title" {
		t.Errorf("Title after re-upsert = %q, want %q", detail.Doc.Title, "Updated Title")
	}
	if detail.Doc.DocNumber != "" {
		t.Errorf("DocNumber = %q, want empty (never set)", detail.Doc.DocNumber)
	}
}

// TestUpsertDocumentConvergesConcurrentInsertsOnSameDocNumber forces the
// select-then-insert race UpsertDocument's retry exists for: every racer
// shares one brand-new doc_number, so none of them can find it via
// findExisting before at least one has to insert. Postgres blocks a losing
// concurrent INSERT against a conflicting unique-index entry until the
// winner commits or rolls back, so every loser resolves to a definitive
// SQLSTATE 23505 the instant the winner commits — meaning UpsertDocument's
// single retry (its fresh-transaction findExisting is guaranteed to see the
// winner's now-committed row) converges deterministically, regardless of
// how many racers there are or exactly how the goroutine scheduler
// interleaves them.
func TestUpsertDocumentConvergesConcurrentInsertsOnSameDocNumber(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	c := newCorpus(t, pool, corpus.VNReg)

	docNumber := "concurrent-upsert-" + uuid.NewString()
	d := store.Document{
		CorpusID:       string(corpus.VNReg),
		Title:          "Concurrent Upsert Doc",
		DocNumber:      docNumber,
		Language:       "vi",
		ValidityStatus: "in_force",
		AccessTier:     string(corpus.TierPublic),
		ObservedAt:     time.Now().UTC(),
	}

	const racers = 6
	ids := make([]uuid.UUID, racers)
	errs := make([]error, racers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(racers)
	for i := range racers {
		go func(i int) {
			defer wg.Done()
			<-start // release every racer as close to simultaneously as possible
			ids[i], errs[i] = c.UpsertDocument(ctx, d)
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("racer %d: UpsertDocument() error = %v, want nil", i, err)
		}
	}
	for i, id := range ids[1:] {
		if id != ids[0] {
			t.Errorf("racer %d: UpsertDocument() id = %v, want %v (same row as racer 0)", i+1, id, ids[0])
		}
	}

	var count int
	q := `SELECT count(*) FROM vn_reg.document WHERE doc_number = $1`
	if err := pool.QueryRow(ctx, q, docNumber).Scan(&count); err != nil {
		t.Fatalf("counting documents for doc_number %q: %v", docNumber, err)
	}
	if count != 1 {
		t.Errorf("document count for doc_number %q = %d, want 1", docNumber, count)
	}
}

func TestReplaceSectionsReplacesFully(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	c := newCorpus(t, pool, corpus.VNReg)

	docID := mustUpsertVNRegDoc(t, ctx, c, "replace-sections-"+uuid.NewString())

	secs2 := []store.Section{
		{
			CorpusID: string(corpus.VNReg), CitationPath: "Điều 1", Body: "first section",
			ValidityStatus: "in_force", AccessTier: string(corpus.TierPublic),
		},
		{
			CorpusID: string(corpus.VNReg), CitationPath: "Điều 2", Body: "second section",
			ValidityStatus: "in_force", AccessTier: string(corpus.TierPublic),
		},
	}
	if err := c.ReplaceSections(ctx, docID, secs2); err != nil {
		t.Fatalf("ReplaceSections() with 2 sections error = %v", err)
	}
	if got := countSections(t, ctx, pool, "vn_reg", docID); got != 2 {
		t.Fatalf("section count after first ReplaceSections() = %d, want 2", got)
	}

	if err := c.ReplaceSections(ctx, docID, secs2[:1]); err != nil {
		t.Fatalf("ReplaceSections() with 1 section error = %v", err)
	}
	if got := countSections(t, ctx, pool, "vn_reg", docID); got != 1 {
		t.Errorf("section count after second ReplaceSections() = %d, want 1", got)
	}
}

func TestReplaceSectionsRejectsWrongEmbeddingDims(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	c := newCorpus(t, pool, corpus.VNReg)

	docID := mustUpsertVNRegDoc(t, ctx, c, "dim-assert-"+uuid.NewString())

	bad := []store.Section{
		{
			CorpusID: string(corpus.VNReg), Body: "bad embedding", ValidityStatus: "in_force",
			AccessTier: string(corpus.TierPublic), Embedding: make([]float32, 512),
		},
	}
	err := c.ReplaceSections(ctx, docID, bad)
	if err == nil {
		t.Fatal("ReplaceSections() with a 512-d embedding error = nil, want an error mentioning 1536")
	}
	if !strings.Contains(err.Error(), "1536") {
		t.Errorf("ReplaceSections() error = %q, want it to mention 1536", err.Error())
	}
	if got := countSections(t, ctx, pool, "vn_reg", docID); got != 0 {
		t.Errorf("section count after rejected ReplaceSections() = %d, want 0 (transaction rolled back)", got)
	}
}

// assertSectionOrder checks that sections' CitationPath and Position exactly
// match wantOrder, by index — used to prove GetDocument orders sections by
// position, not created_at.
func assertSectionOrder(t *testing.T, sections []store.Section, wantOrder []string) {
	t.Helper()
	for i, s := range sections {
		if s.CitationPath != wantOrder[i] {
			t.Errorf("Sections[%d].CitationPath = %q, want %q (order by position, not created_at)",
				i, s.CitationPath, wantOrder[i])
		}
		if s.Position != i {
			t.Errorf("Sections[%d].Position = %d, want %d", i, s.Position, i)
		}
	}
}

func TestGetDocumentReturnsDocSectionsAndEvents(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	c := newCorpus(t, pool, corpus.VNReg)

	docNumber := "full-detail-" + uuid.NewString()
	docID, err := c.UpsertDocument(ctx, store.Document{
		CorpusID:       string(corpus.VNReg),
		Title:          "Full Detail Doc",
		DocNumber:      docNumber,
		Language:       "vi",
		ValidityStatus: "in_force",
		AccessTier:     string(corpus.TierPublic),
		ObservedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("UpsertDocument() error = %v", err)
	}

	secs := []store.Section{
		{
			CorpusID: string(corpus.VNReg), CitationPath: "Điều 1", Body: "first",
			ValidityStatus: "in_force", AccessTier: string(corpus.TierPublic), Embedding: make([]float32, 1536),
		},
		{
			CorpusID: string(corpus.VNReg), CitationPath: "Điều 2", Body: "second",
			ValidityStatus: "in_force", AccessTier: string(corpus.TierPublic),
		},
		{
			CorpusID: string(corpus.VNReg), CitationPath: "Điều 3", Body: "third",
			ValidityStatus: "in_force", AccessTier: string(corpus.TierPublic),
		},
	}
	if err := c.ReplaceSections(ctx, docID, secs); err != nil {
		t.Fatalf("ReplaceSections() error = %v", err)
	}
	// Back-date created_at in the OPPOSITE order from secs' input order: if
	// GetDocument's section order were still driven by created_at (or fell
	// back to it on a tie), this would read back reversed. It won't — order
	// comes from the `position` column ReplaceSections stamps from each
	// section's slice index, independent of created_at.
	base := time.Now().UTC()
	setSectionCreatedAt(t, ctx, pool, "vn_reg", docID, "Điều 1", base.Add(2*time.Second))
	setSectionCreatedAt(t, ctx, pool, "vn_reg", docID, "Điều 2", base.Add(time.Second))
	setSectionCreatedAt(t, ctx, pool, "vn_reg", docID, "Điều 3", base)

	events := []store.AmendmentEvent{
		{TargetDocID: docID, Clause: "Điều 5", EventDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{TargetDocID: docID, Clause: "Điều 6", EventDate: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)},
	}
	if err := c.InsertAmendmentEvents(ctx, events); err != nil {
		t.Fatalf("InsertAmendmentEvents() error = %v", err)
	}

	detail, err := c.GetDocument(ctx, "", docID)
	if err != nil {
		t.Fatalf("GetDocument() error = %v", err)
	}
	if detail.Doc.ID != docID {
		t.Errorf("Doc.ID = %v, want %v", detail.Doc.ID, docID)
	}
	if detail.Doc.Title != "Full Detail Doc" {
		t.Errorf("Doc.Title = %q, want %q", detail.Doc.Title, "Full Detail Doc")
	}
	if len(detail.Sections) != 3 {
		t.Fatalf("len(Sections) = %d, want 3", len(detail.Sections))
	}
	assertSectionOrder(t, detail.Sections, []string{"Điều 1", "Điều 2", "Điều 3"})
	if len(detail.Sections[0].Embedding) != 1536 {
		t.Errorf("Sections[0].Embedding len = %d, want 1536", len(detail.Sections[0].Embedding))
	}
	if len(detail.Events) != 2 {
		t.Fatalf("len(Events) = %d, want 2", len(detail.Events))
	}
	if detail.Events[0].Clause != "Điều 5" || detail.Events[1].Clause != "Điều 6" {
		t.Errorf("Events order = [%q, %q], want [Điều 5, Điều 6] (event_date ascending)",
			detail.Events[0].Clause, detail.Events[1].Clause)
	}
}

// TestInsertAmendmentEventsDedupesIdenticalEvent pins migration 007's
// per-schema unique index + ON CONFLICT DO NOTHING: a changed document
// re-indexed by the ingest pipeline re-derives and re-inserts the same
// relation event (T14 self-review, "Amendment-event duplication on genuine
// re-index") and must not duplicate the row.
func TestInsertAmendmentEventsDedupesIdenticalEvent(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	c := newCorpus(t, pool, corpus.VNReg)

	targetID := mustUpsertVNRegDoc(t, ctx, c, "dedup-target-"+uuid.NewString())
	amendingID := mustUpsertVNRegDoc(t, ctx, c, "dedup-amending-"+uuid.NewString())

	event := store.AmendmentEvent{
		TargetDocID: targetID, AmendingDocID: &amendingID,
		Clause: "Điều 5", EventDate: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := c.InsertAmendmentEvents(ctx, []store.AmendmentEvent{event}); err != nil {
		t.Fatalf("first InsertAmendmentEvents() error = %v", err)
	}
	if err := c.InsertAmendmentEvents(ctx, []store.AmendmentEvent{event}); err != nil {
		t.Fatalf("duplicate InsertAmendmentEvents() error = %v, want nil (ON CONFLICT DO NOTHING)", err)
	}

	detail, err := c.GetDocument(ctx, "", targetID)
	if err != nil {
		t.Fatalf("GetDocument() error = %v", err)
	}
	if len(detail.Events) != 1 {
		t.Fatalf("len(Events) after inserting the same event twice = %d, want 1", len(detail.Events))
	}
}

func TestGetDocumentMasksConfidentialRowFromPublicRole(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	c := newCorpus(t, pool, corpus.GroupStd)

	// Inserted as the pool's connecting role — the table owner in the test
	// container, which bypasses RLS entirely. This is the "inserted as
	// owner" fixture: the row genuinely exists, but mise_public has no
	// GRANT USAGE on group_std (migrations/004) and no policy admits it
	// even if it did.
	docID, err := c.UpsertDocument(ctx, store.Document{
		CorpusID:       string(corpus.GroupStd),
		Title:          "Group Confidential Standard",
		Language:       "en",
		ValidityStatus: "in_force",
		AccessTier:     string(corpus.TierGroupConfidential),
		ObservedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("UpsertDocument() error = %v", err)
	}

	_, err = c.GetDocument(ctx, "mise_public", docID)
	if err == nil {
		t.Fatal("GetDocument() as mise_public on a group-confidential row error = nil, want not-found")
	}
	if !errors.Is(err, store.ErrDocumentNotFound) {
		t.Errorf("GetDocument() error = %v, want errors.Is(_, store.ErrDocumentNotFound)", err)
	}
	// The fold to ErrDocumentNotFound must not discard the underlying
	// SQLSTATE 42501 (insufficient_privilege) that actually happened here —
	// losing it would make a missing GRANT indistinguishable from a clean
	// not-found. errors.As must still reach the *pgconn.PgError through the
	// multi-wrapped chain.
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("GetDocument() error = %v, want errors.As(_, *pgconn.PgError) to succeed", err)
	}
	if pgErr.Code != "42501" {
		t.Errorf("GetDocument() underlying pgconn.PgError.Code = %q, want %q", pgErr.Code, "42501")
	}
}
