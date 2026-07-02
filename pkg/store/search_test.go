//go:build integration

package store_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"danny.vn/mise/internal/testdb"
	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/rag/embed"
	"danny.vn/mise/pkg/store"
)

// searchMarker returns a short, unique alphanumeric token (no separators, so
// the 'simple' text-search config and the fake embedder's tokenizer both
// treat it as one lexeme) for building fixture/query pairs that can't
// collide with any other row in the shared test database (testdb.New
// caches one container/pool for the whole test binary).
func searchMarker(prefix string) string {
	return prefix + strings.ReplaceAll(uuid.NewString(), "-", "")
}

// upsertSearchDoc inserts a minimal, valid document fixture for a search
// test and returns its id. effectiveDate may be nil.
func upsertSearchDoc(
	t *testing.T, ctx context.Context, c *store.Corpus, id corpus.ID, tier corpus.AccessTier,
	docNumber string, effectiveDate *time.Time,
) uuid.UUID {
	t.Helper()
	docID, err := c.UpsertDocument(ctx, store.Document{
		CorpusID:       string(id),
		Title:          "Search Fixture " + docNumber,
		DocNumber:      docNumber,
		Language:       "en",
		ValidityStatus: "in_force",
		AccessTier:     string(tier),
		EffectiveDate:  effectiveDate,
		ObservedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("UpsertDocument() search fixture error = %v", err)
	}
	return docID
}

// writeSearchSection replaces docID's sections with a single section: body,
// embedded via emb so the vector CTE can find it (body_tsv is a generated
// column, derived automatically for the lexical CTE), at citationPath, with
// the given section-level validity status.
func writeSearchSection(
	t *testing.T, ctx context.Context, c *store.Corpus, id corpus.ID, tier corpus.AccessTier,
	docID uuid.UUID, citationPath, body, validityStatus string, emb embed.Embedder,
) {
	t.Helper()
	vecs, err := emb.Embed(ctx, []string{body})
	if err != nil {
		t.Fatalf("embedding search fixture section: %v", err)
	}
	secs := []store.Section{{
		CorpusID: string(id), CitationPath: citationPath, Body: body,
		ValidityStatus: validityStatus, AccessTier: string(tier), Embedding: vecs[0],
	}}
	if err := c.ReplaceSections(ctx, docID, secs); err != nil {
		t.Fatalf("ReplaceSections() search fixture error = %v", err)
	}
}

// hitCitationPaths extracts CitationPath from hits, in order — used to
// print a compact, readable failure message instead of dumping full Hit
// structs.
func hitCitationPaths(hits []store.Hit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.CitationPath
	}
	return out
}

// indexOfCitationPath returns hits' index of the first Hit whose
// CitationPath equals path, or -1 if none matches.
func indexOfCitationPath(hits []store.Hit, path string) int {
	for i, h := range hits {
		if h.CitationPath == path {
			return i
		}
	}
	return -1
}

func containsCitationPath(hits []store.Hit, path string) bool {
	return indexOfCitationPath(hits, path) != -1
}

// (a) lexical exact-phrase hit ranks its section first.
func TestSearchLexicalExactPhraseRanksFirst(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	c := newCorpus(t, pool, corpus.VNReg)
	emb := embed.NewFake()

	m := searchMarker("zzalpha")
	phrase := m + " incident reporting deadline obligations"
	pathHit := "search-a-hit-" + m
	pathDisjoint := "search-a-disjoint-" + m

	docHit := upsertSearchDoc(t, ctx, c, corpus.VNReg, corpus.TierPublic, "doc-a-hit-"+m, nil)
	writeSearchSection(t, ctx, c, corpus.VNReg, corpus.TierPublic, docHit, pathHit,
		"Institutions must observe the "+phrase+" set out in this circular.", "in_force", emb)

	docDisjoint := upsertSearchDoc(t, ctx, c, corpus.VNReg, corpus.TierPublic, "doc-a-disjoint-"+m, nil)
	writeSearchSection(t, ctx, c, corpus.VNReg, corpus.TierPublic, docDisjoint, pathDisjoint,
		"zzunrelated"+searchMarker("")+" covers staff leave entitlements and travel claims.", "in_force", emb)

	hits, err := store.Search(ctx, pool, emb, phrase, store.SearchOpts{
		Corpora: []corpus.ID{corpus.VNReg}, TopK: 5, InForceOnly: true,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(hits) == 0 || hits[0].CitationPath != pathHit {
		t.Fatalf("Search() hits = %v, want first hit CitationPath = %q", hitCitationPaths(hits), pathHit)
	}
}

// (b) token-overlap query ranks overlapping section above disjoint one.
func TestSearchTokenOverlapRanksOverlapAboveDisjoint(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	c := newCorpus(t, pool, corpus.VNReg)
	emb := embed.NewFake()

	m := searchMarker("zzbravo")
	query := m + " cloud outsourcing risk assessment"
	pathOverlap := "search-b-overlap-" + m
	pathDisjoint := "search-b-disjoint-" + m

	docOverlap := upsertSearchDoc(t, ctx, c, corpus.VNReg, corpus.TierPublic, "doc-b-overlap-"+m, nil)
	writeSearchSection(t, ctx, c, corpus.VNReg, corpus.TierPublic, docOverlap, pathOverlap,
		"Guidance on "+m+" cloud vendor risk review procedures.", "in_force", emb)

	docDisjoint := upsertSearchDoc(t, ctx, c, corpus.VNReg, corpus.TierPublic, "doc-b-disjoint-"+m, nil)
	writeSearchSection(t, ctx, c, corpus.VNReg, corpus.TierPublic, docDisjoint, pathDisjoint,
		"zzcharlie"+searchMarker("")+" unrelated staff canteen menu rotation schedule.", "in_force", emb)

	hits, err := store.Search(ctx, pool, emb, query, store.SearchOpts{
		Corpora: []corpus.ID{corpus.VNReg}, TopK: 5, InForceOnly: true,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	overlapIdx := indexOfCitationPath(hits, pathOverlap)
	disjointIdx := indexOfCitationPath(hits, pathDisjoint)
	if overlapIdx == -1 {
		t.Fatalf("Search() hits = %v, want overlap section %q present", hitCitationPaths(hits), pathOverlap)
	}
	if disjointIdx != -1 && disjointIdx < overlapIdx {
		t.Errorf("Search() ranked disjoint section (rank %d) above overlap section (rank %d): hits = %v",
			disjointIdx, overlapIdx, hitCitationPaths(hits))
	}
}

// (c) repealed section excluded with InForceOnly:true, included with
// InForceOnly:false; amended included under InForceOnly:true.
func TestSearchValidityFilterExcludesRepealedIncludesAmended(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	c := newCorpus(t, pool, corpus.VNReg)
	emb := embed.NewFake()

	m := searchMarker("zzcharlie")
	query := m + " capital buffer disclosure requirement"
	pathRepealed := "search-c-repealed-" + m
	pathAmended := "search-c-amended-" + m

	docRepealed := upsertSearchDoc(t, ctx, c, corpus.VNReg, corpus.TierPublic, "doc-c-repealed-"+m, nil)
	writeSearchSection(t, ctx, c, corpus.VNReg, corpus.TierPublic, docRepealed, pathRepealed,
		"Historical text on "+query+".", "repealed", emb)

	docAmended := upsertSearchDoc(t, ctx, c, corpus.VNReg, corpus.TierPublic, "doc-c-amended-"+m, nil)
	writeSearchSection(t, ctx, c, corpus.VNReg, corpus.TierPublic, docAmended, pathAmended,
		"Current text on "+query+".", "amended", emb)

	filtered, err := store.Search(ctx, pool, emb, query, store.SearchOpts{
		Corpora: []corpus.ID{corpus.VNReg}, TopK: 10, InForceOnly: true,
	})
	if err != nil {
		t.Fatalf("Search(InForceOnly:true) error = %v", err)
	}
	if containsCitationPath(filtered, pathRepealed) {
		t.Errorf("Search(InForceOnly:true) hits = %v, want repealed section %q excluded",
			hitCitationPaths(filtered), pathRepealed)
	}
	if !containsCitationPath(filtered, pathAmended) {
		t.Errorf("Search(InForceOnly:true) hits = %v, want amended section %q included",
			hitCitationPaths(filtered), pathAmended)
	}

	unfiltered, err := store.Search(ctx, pool, emb, query, store.SearchOpts{
		Corpora: []corpus.ID{corpus.VNReg}, TopK: 10, InForceOnly: false,
	})
	if err != nil {
		t.Fatalf("Search(InForceOnly:false) error = %v", err)
	}
	if !containsCitationPath(unfiltered, pathRepealed) {
		t.Errorf("Search(InForceOnly:false) hits = %v, want repealed section %q included",
			hitCitationPaths(unfiltered), pathRepealed)
	}
	if !containsCitationPath(unfiltered, pathAmended) {
		t.Errorf("Search(InForceOnly:false) hits = %v, want amended section %q included",
			hitCitationPaths(unfiltered), pathAmended)
	}
}

// (d) AsOf excludes a document effective in the future.
func TestSearchAsOfExcludesFutureEffectiveDocument(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	c := newCorpus(t, pool, corpus.VNReg)
	emb := embed.NewFake()

	m := searchMarker("zzdelta")
	query := m + " liquidity coverage ratio phase-in"
	pathFuture := "search-d-future-" + m

	future := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	docFuture := upsertSearchDoc(t, ctx, c, corpus.VNReg, corpus.TierPublic, "doc-d-future-"+m, &future)
	writeSearchSection(t, ctx, c, corpus.VNReg, corpus.TierPublic, docFuture, pathFuture,
		"Not-yet-effective text on "+query+".", "in_force", emb)

	asOf := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	hits, err := store.Search(ctx, pool, emb, query, store.SearchOpts{
		Corpora: []corpus.ID{corpus.VNReg}, TopK: 10, InForceOnly: true, AsOf: &asOf,
	})
	if err != nil {
		t.Fatalf("Search(AsOf=2026-01-01) error = %v", err)
	}
	if containsCitationPath(hits, pathFuture) {
		t.Errorf("Search(AsOf=2026-01-01) hits = %v, want future-effective section %q excluded",
			hitCitationPaths(hits), pathFuture)
	}

	// Control: without AsOf, the same query surfaces the future-effective
	// document — proving its absence above is the AsOf predicate at work,
	// not some other filter or a ranking accident.
	unfiltered, err := store.Search(ctx, pool, emb, query, store.SearchOpts{
		Corpora: []corpus.ID{corpus.VNReg}, TopK: 10, InForceOnly: true,
	})
	if err != nil {
		t.Fatalf("Search() without AsOf error = %v", err)
	}
	if !containsCitationPath(unfiltered, pathFuture) {
		t.Errorf("Search() without AsOf hits = %v, want future-effective section %q present (control)",
			hitCitationPaths(unfiltered), pathFuture)
	}
}

// (e) Corpora:[vn-reg,my-reg] merges results from both.
func TestSearchMergesAcrossCorpora(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()
	cVN := newCorpus(t, pool, corpus.VNReg)
	cMY := newCorpus(t, pool, corpus.MYReg)
	emb := embed.NewFake()

	m := searchMarker("zzecho")
	query := m + " cross-border payment settlement window"
	pathVN := "search-e-vn-" + m
	pathMY := "search-e-my-" + m

	docVN := upsertSearchDoc(t, ctx, cVN, corpus.VNReg, corpus.TierPublic, "doc-e-vn-"+m, nil)
	writeSearchSection(t, ctx, cVN, corpus.VNReg, corpus.TierPublic, docVN, pathVN,
		"VN circular on "+query+".", "in_force", emb)

	docMY := upsertSearchDoc(t, ctx, cMY, corpus.MYReg, corpus.TierPublic, "doc-e-my-"+m, nil)
	writeSearchSection(t, ctx, cMY, corpus.MYReg, corpus.TierPublic, docMY, pathMY,
		"MY policy document on "+query+".", "in_force", emb)

	hits, err := store.Search(ctx, pool, emb, query, store.SearchOpts{
		Corpora: []corpus.ID{corpus.VNReg, corpus.MYReg}, TopK: 10, InForceOnly: true,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	gotCorpora := map[string]bool{}
	for _, h := range hits {
		gotCorpora[h.CorpusID] = true
	}
	if !gotCorpora[string(corpus.VNReg)] {
		t.Errorf("Search() hits = %v, want a %s hit", hitCitationPaths(hits), corpus.VNReg)
	}
	if !gotCorpora[string(corpus.MYReg)] {
		t.Errorf("Search() hits = %v, want a %s hit", hitCitationPaths(hits), corpus.MYReg)
	}
	if !containsCitationPath(hits, pathVN) {
		t.Errorf("Search() hits missing %s fixture %q: %v", corpus.VNReg, pathVN, hitCitationPaths(hits))
	}
	if !containsCitationPath(hits, pathMY) {
		t.Errorf("Search() hits missing %s fixture %q: %v", corpus.MYReg, pathMY, hitCitationPaths(hits))
	}
}
