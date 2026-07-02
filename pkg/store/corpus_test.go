//go:build integration

package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
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
// the store API. created_at defaults to the DB's now(), which is constant
// for every row written by the same transaction — so two sections from one
// ReplaceSections call always tie on created_at and fall back to sorting by
// their (random) id. Real created_at ties only don't happen across separate
// ingest runs; backdating here makes that realistic, distinct-timestamp case
// deterministic so the "ordered created_at, id" contract can actually be
// asserted. Scoped to docID (not just citationPath): the test container is a
// shared singleton, and other tests reuse the same citation_path text for
// their own, unrelated documents.
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
	}
	if err := c.ReplaceSections(ctx, docID, secs); err != nil {
		t.Fatalf("ReplaceSections() error = %v", err)
	}
	// Both sections above were written by the same ReplaceSections
	// transaction, so they share one created_at (Postgres now() is
	// transaction-constant) — back-date them apart to exercise the "ordered
	// created_at, id" contract deterministically, as distinct ingest runs
	// would produce naturally.
	base := time.Now().UTC()
	setSectionCreatedAt(t, ctx, pool, "vn_reg", docID, "Điều 1", base)
	setSectionCreatedAt(t, ctx, pool, "vn_reg", docID, "Điều 2", base.Add(time.Second))

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
	if len(detail.Sections) != 2 {
		t.Fatalf("len(Sections) = %d, want 2", len(detail.Sections))
	}
	if detail.Sections[0].CitationPath != "Điều 1" || detail.Sections[1].CitationPath != "Điều 2" {
		t.Errorf("Sections order = [%q, %q], want [Điều 1, Điều 2]",
			detail.Sections[0].CitationPath, detail.Sections[1].CitationPath)
	}
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
}
