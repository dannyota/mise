//go:build integration

package store_test

import (
	"context"
	"testing"

	"danny.vn/mise/internal/testdb"
	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/rag/embed"
	"danny.vn/mise/pkg/store"
)

// TestImageRefRoundTripsThroughDiagramsCorpus is the M6 multimodal gate:
// a diagrams-corpus section written with an ImageRef must surface it back
// through both read paths — GetDocument's section scan and Search's Hit —
// since eval's CaptionHits metric counts non-empty Hit.ImageRef values and
// is dead weight if the column never round-trips. Running against the
// diagrams corpus also exercises migration 013's schema end to end (the
// corpus existed in pkg/corpus since M6 but had no schema until 013).
func TestImageRefRoundTripsThroughDiagramsCorpus(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	c := newCorpus(t, pool, corpus.Diagrams)
	emb := embed.NewFake()

	m := searchMarker("zzimg")
	body := m + " network segmentation diagram caption"
	imageRef := "blob://diagrams/" + m + ".png"

	docID := upsertSearchDoc(t, ctx, c, corpus.Diagrams, corpus.TierLocalConfidential, "img-"+m, nil)
	vecs, err := emb.Embed(ctx, []string{body})
	if err != nil {
		t.Fatalf("embedding diagram fixture section: %v", err)
	}
	secs := []store.Section{{
		CorpusID: string(corpus.Diagrams), CitationPath: "figure-1-" + m, Body: body,
		ValidityStatus: "in_force", AccessTier: string(corpus.TierLocalConfidential),
		Embedding: vecs[0], ImageRef: imageRef,
	}}
	if err := c.ReplaceSections(ctx, docID, secs); err != nil {
		t.Fatalf("ReplaceSections() diagram fixture error = %v", err)
	}

	t.Run("GetDocument returns the section's ImageRef", func(t *testing.T) {
		detail, err := c.GetDocument(ctx, "mise_local", docID)
		if err != nil {
			t.Fatalf("GetDocument(mise_local) error = %v", err)
		}
		if len(detail.Sections) != 1 {
			t.Fatalf("GetDocument(mise_local) sections = %d, want 1", len(detail.Sections))
		}
		if got := detail.Sections[0].ImageRef; got != imageRef {
			t.Errorf("GetDocument(mise_local) section ImageRef = %q, want %q", got, imageRef)
		}
	})

	t.Run("Search hit carries the ImageRef", func(t *testing.T) {
		hits, err := store.Search(ctx, pool, emb, body, store.SearchOpts{
			Corpora: []corpus.ID{corpus.Diagrams}, Role: "mise_local", TopK: 5,
		})
		if err != nil {
			t.Fatalf("Search(mise_local) error = %v", err)
		}
		i := indexOfCitationPath(hits, "figure-1-"+m)
		if i < 0 {
			t.Fatalf("Search(mise_local) hits = %v, want figure section PRESENT", hitCitationPaths(hits))
		}
		if got := hits[i].ImageRef; got != imageRef {
			t.Errorf("Search(mise_local) hit ImageRef = %q, want %q", got, imageRef)
		}
	})

	t.Run("mise_public gets zero hits from the diagrams corpus", func(t *testing.T) {
		hits, err := store.Search(ctx, pool, emb, body, store.SearchOpts{
			Corpora: []corpus.ID{corpus.Diagrams}, Role: "mise_public", TopK: 5,
		})
		if err != nil {
			t.Fatalf("Search(mise_public) error = %v", err)
		}
		if len(hits) != 0 {
			t.Errorf("Search(mise_public) hits = %v, want none (no USAGE on diagrams schema)", hitCitationPaths(hits))
		}
	})
}
