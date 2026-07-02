package mcp

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/store"
)

// stubSearcher is a Searcher test double: it records the query/opts it was
// last called with and returns a fixed hits/err pair.
type stubSearcher struct {
	calls int
	query string
	opts  store.SearchOpts
	hits  []store.Hit
	err   error
}

func (s *stubSearcher) Search(_ context.Context, query string, opts store.SearchOpts) ([]store.Hit, error) {
	s.calls++
	s.query, s.opts = query, opts
	return s.hits, s.err
}

// stubDocGetter is a DocGetter test double: it records the role/corpusID/
// docID it was last called with and returns a fixed detail/err pair.
type stubDocGetter struct {
	calls    int
	role     string
	corpusID string
	docID    uuid.UUID
	detail   store.DocumentDetail
	err      error
}

func (g *stubDocGetter) GetDocument(
	_ context.Context, role, corpusID string, docID uuid.UUID,
) (store.DocumentDetail, error) {
	g.calls++
	g.role, g.corpusID, g.docID = role, corpusID, docID
	return g.detail, g.err
}

// allRegisteredCorpusIDs returns every corpus.All() ID — the search tool's
// documented default scope.
func allRegisteredCorpusIDs() []corpus.ID {
	all := corpus.All()
	out := make([]corpus.ID, len(all))
	for i, d := range all {
		out[i] = d.ID
	}
	return out
}

func containsCorpusID(ids []corpus.ID, want corpus.ID) bool {
	return slices.Contains(ids, want)
}

// --- search tool: defaults + validation ---------------------------------

func TestSearchHandlerAppliesDefaults(t *testing.T) {
	stub := &stubSearcher{}
	h := newSearchHandler(stub, "mise_public")

	_, _, err := h(context.Background(), nil, SearchInput{Query: "capital buffer"})
	if err != nil {
		t.Fatalf("handler error = %v, want nil", err)
	}
	if stub.calls != 1 {
		t.Fatalf("Searcher.Search called %d times, want 1", stub.calls)
	}

	if stub.opts.TopK != defaultSearchTopK {
		t.Errorf("opts.TopK = %d, want %d (default)", stub.opts.TopK, defaultSearchTopK)
	}
	if !stub.opts.InForceOnly {
		t.Error("opts.InForceOnly = false, want true (default)")
	}
	if stub.opts.AsOf != nil {
		t.Errorf("opts.AsOf = %v, want nil (no as_of_date given)", stub.opts.AsOf)
	}
	if stub.opts.Role != "mise_public" {
		t.Errorf("opts.Role = %q, want %q", stub.opts.Role, "mise_public")
	}

	want := allRegisteredCorpusIDs()
	if len(stub.opts.Corpora) != len(want) {
		t.Fatalf("len(opts.Corpora) = %d, want %d (every registered corpus)", len(stub.opts.Corpora), len(want))
	}
	for _, id := range want {
		if !containsCorpusID(stub.opts.Corpora, id) {
			t.Errorf("opts.Corpora = %v, want it to contain %s", stub.opts.Corpora, id)
		}
	}
}

func TestSearchHandlerHonoursExplicitOverrides(t *testing.T) {
	stub := &stubSearcher{}
	h := newSearchHandler(stub, "mise_group")

	inForceOnly := false
	in := SearchInput{
		Query:       "cloud outsourcing",
		Corpora:     []string{string(corpus.VNReg)},
		TopK:        3,
		AsOfDate:    "2024-06-01",
		InForceOnly: &inForceOnly,
	}
	_, _, err := h(context.Background(), nil, in)
	if err != nil {
		t.Fatalf("handler error = %v, want nil", err)
	}

	if stub.opts.TopK != 3 {
		t.Errorf("opts.TopK = %d, want 3", stub.opts.TopK)
	}
	if stub.opts.InForceOnly {
		t.Error("opts.InForceOnly = true, want false (explicit override)")
	}
	if len(stub.opts.Corpora) != 1 || stub.opts.Corpora[0] != corpus.VNReg {
		t.Errorf("opts.Corpora = %v, want [%s]", stub.opts.Corpora, corpus.VNReg)
	}
	wantAsOf := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	if stub.opts.AsOf == nil || !stub.opts.AsOf.Equal(wantAsOf) {
		t.Errorf("opts.AsOf = %v, want %v", stub.opts.AsOf, wantAsOf)
	}
	if stub.query != "cloud outsourcing" {
		t.Errorf("query = %q, want %q", stub.query, "cloud outsourcing")
	}
}

func TestSearchHandlerRejectsUnknownCorpus(t *testing.T) {
	stub := &stubSearcher{}
	h := newSearchHandler(stub, "mise_public")

	_, _, err := h(context.Background(), nil, SearchInput{Query: "x", Corpora: []string{"not-a-corpus"}})
	if err == nil {
		t.Fatal("handler error = nil, want error for unknown corpus")
	}
	if stub.calls != 0 {
		t.Errorf("Searcher.Search called %d times, want 0 (validation should short-circuit)", stub.calls)
	}
}

func TestSearchHandlerRejectsBadAsOfDate(t *testing.T) {
	tests := []struct {
		name string
		date string
	}{
		{"wrong format", "01-01-2024"},
		{"not a date", "banana"},
		{"invalid calendar date", "2024-13-40"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &stubSearcher{}
			h := newSearchHandler(stub, "mise_public")
			_, _, err := h(context.Background(), nil, SearchInput{Query: "x", AsOfDate: tt.date})
			if err == nil {
				t.Fatalf("handler error = nil, want error for as_of_date %q", tt.date)
			}
			if stub.calls != 0 {
				t.Errorf("Searcher.Search called %d times, want 0", stub.calls)
			}
		})
	}
}

func TestSearchHandlerAcceptsValidAsOfDate(t *testing.T) {
	stub := &stubSearcher{}
	h := newSearchHandler(stub, "mise_public")
	_, _, err := h(context.Background(), nil, SearchInput{Query: "x", AsOfDate: "2026-07-01"})
	if err != nil {
		t.Fatalf("handler error = %v, want nil", err)
	}
}

func TestSearchHandlerPropagatesSearcherError(t *testing.T) {
	stub := &stubSearcher{err: errors.New("boom")}
	h := newSearchHandler(stub, "mise_public")

	_, _, err := h(context.Background(), nil, SearchInput{Query: "x"})
	if err == nil {
		t.Fatal("handler error = nil, want the wrapped Searcher error")
	}
}

func TestSearchHandlerMapsHitsToSections(t *testing.T) {
	docID, sectionID := uuid.New(), uuid.New()
	stub := &stubSearcher{hits: []store.Hit{{
		CorpusID: "vn-reg", DocumentID: docID, SectionID: sectionID,
		DocNumber: "12/2024/TT-NHNN", Title: "Circular on X", CitationPath: "Điều 7",
		HeadingPath: "Chương II ▸ Điều 7", Text: "verbatim text", ValidityStatus: "in_force",
		SourceURL: "https://vbpl.vn/x", Score: 0.87,
	}}}
	h := newSearchHandler(stub, "mise_public")

	_, out, err := h(context.Background(), nil, SearchInput{Query: "x"})
	if err != nil {
		t.Fatalf("handler error = %v, want nil", err)
	}
	if len(out.Sections) != 1 {
		t.Fatalf("len(out.Sections) = %d, want 1", len(out.Sections))
	}
	got := out.Sections[0]
	want := SectionHit{
		CorpusID: "vn-reg", DocumentID: docID.String(), SectionID: sectionID.String(),
		DocNumber: "12/2024/TT-NHNN", Title: "Circular on X", CitationPath: "Điều 7",
		HeadingPath: "Chương II ▸ Điều 7", Text: "verbatim text", ValidityStatus: "in_force",
		Score: 0.87, SourceURL: "https://vbpl.vn/x",
	}
	if got != want {
		t.Errorf("out.Sections[0] = %+v, want %+v", got, want)
	}
}

func TestSearchHandlerEmptyHitsProducesEmptySliceNotNil(t *testing.T) {
	stub := &stubSearcher{hits: nil}
	h := newSearchHandler(stub, "mise_public")

	_, out, err := h(context.Background(), nil, SearchInput{Query: "x"})
	if err != nil {
		t.Fatalf("handler error = %v, want nil", err)
	}
	if out.Sections == nil {
		t.Error("out.Sections = nil, want a non-nil empty slice (must marshal to [] not null)")
	}
}

// --- document tool: validation + mapping ---------------------------------

func TestDocumentHandlerRejectsUnknownCorpus(t *testing.T) {
	stub := &stubDocGetter{}
	h := newDocumentHandler(stub, "mise_public")

	_, _, err := h(context.Background(), nil, DocumentInput{CorpusID: "nope", DocumentID: uuid.NewString()})
	if err == nil {
		t.Fatal("handler error = nil, want error for unknown corpus")
	}
	if stub.calls != 0 {
		t.Errorf("DocGetter.GetDocument called %d times, want 0", stub.calls)
	}
}

func TestDocumentHandlerRejectsBadUUID(t *testing.T) {
	stub := &stubDocGetter{}
	h := newDocumentHandler(stub, "mise_public")

	_, _, err := h(context.Background(), nil, DocumentInput{CorpusID: string(corpus.VNReg), DocumentID: "not-a-uuid"})
	if err == nil {
		t.Fatal("handler error = nil, want error for invalid document_id")
	}
	if stub.calls != 0 {
		t.Errorf("DocGetter.GetDocument called %d times, want 0", stub.calls)
	}
}

func TestDocumentHandlerPropagatesNotFound(t *testing.T) {
	stub := &stubDocGetter{err: fmt.Errorf("wrap: %w", store.ErrDocumentNotFound)}
	h := newDocumentHandler(stub, "mise_public")

	_, _, err := h(context.Background(), nil, DocumentInput{CorpusID: string(corpus.VNReg), DocumentID: uuid.NewString()})
	if !errors.Is(err, store.ErrDocumentNotFound) {
		t.Errorf("handler error = %v, want errors.Is(_, store.ErrDocumentNotFound)", err)
	}
}

func TestDocumentHandlerPassesRoleAndIDsThrough(t *testing.T) {
	docID := uuid.New()
	stub := &stubDocGetter{}
	h := newDocumentHandler(stub, "mise_local")

	in := DocumentInput{CorpusID: string(corpus.LocalPolicy), DocumentID: docID.String()}
	_, _, err := h(context.Background(), nil, in)
	if err != nil {
		t.Fatalf("handler error = %v, want nil", err)
	}
	if stub.role != "mise_local" {
		t.Errorf("GetDocument role = %q, want %q", stub.role, "mise_local")
	}
	if stub.corpusID != string(corpus.LocalPolicy) {
		t.Errorf("GetDocument corpusID = %q, want %q", stub.corpusID, corpus.LocalPolicy)
	}
	if stub.docID != docID {
		t.Errorf("GetDocument docID = %v, want %v", stub.docID, docID)
	}
}

func TestDocumentHandlerMapsEnvelopeSectionsAndTimeline(t *testing.T) {
	issued := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	amendingID := uuid.New()
	docID := uuid.New()

	stub := &stubDocGetter{detail: store.DocumentDetail{
		Doc: store.Document{
			ID: docID, CorpusID: "vn-reg", Title: "Circular 12", DocNumber: "12/2024/TT-NHNN",
			CitationScheme: "dieu-khoan-diem", Language: "vi",
			ValidityStatus: "amended", IssuingAuthority: "SBV", SignerName: "Governor",
			Version: "1", SourceURL: "https://vbpl.vn/12", SourceSystem: "vbpl",
			ContentType: "text/html", AccessTier: "public", IssuedDate: &issued,
			ObservedAt: issued,
		},
		Sections: []store.Section{{
			CitationPath: "Điều 1", HeadingPath: "Chương I ▸ Điều 1", Body: "first section text",
			ValidityStatus: "in_force",
		}},
		Events: []store.AmendmentEvent{{
			TargetDocID: docID, AmendingDocID: &amendingID, Kind: "amended", Clause: "Điều 5", EventDate: issued,
		}},
	}}
	h := newDocumentHandler(stub, "mise_public")

	_, out, err := h(context.Background(), nil, DocumentInput{CorpusID: "vn-reg", DocumentID: docID.String()})
	if err != nil {
		t.Fatalf("handler error = %v, want nil", err)
	}

	if out.Document.ID != docID.String() {
		t.Errorf("out.Document.ID = %q, want %q", out.Document.ID, docID.String())
	}
	if out.Document.Title != "Circular 12" || out.Document.ValidityStatus != "amended" {
		t.Errorf("out.Document = %+v, want Title=Circular 12 ValidityStatus=amended", out.Document)
	}
	wantIssued := issued.Format(time.RFC3339)
	if out.Document.IssuedDate == nil || *out.Document.IssuedDate != wantIssued {
		t.Errorf("out.Document.IssuedDate = %v, want %s", out.Document.IssuedDate, wantIssued)
	}
	if out.Document.EffectiveDate != nil {
		t.Errorf("out.Document.EffectiveDate = %v, want nil (unset in fixture)", *out.Document.EffectiveDate)
	}

	if len(out.Sections) != 1 {
		t.Fatalf("len(out.Sections) = %d, want 1", len(out.Sections))
	}
	wantSec := DocSection{
		CitationPath: "Điều 1", HeadingPath: "Chương I ▸ Điều 1",
		Text: "first section text", ValidityStatus: "in_force",
	}
	if out.Sections[0] != wantSec {
		t.Errorf("out.Sections[0] = %+v, want %+v", out.Sections[0], wantSec)
	}

	if len(out.Amendments) != 1 {
		t.Fatalf("len(out.Amendments) = %d, want 1", len(out.Amendments))
	}
	gotAmend := out.Amendments[0]
	if gotAmend.AmendingDocID == nil || *gotAmend.AmendingDocID != amendingID.String() {
		t.Errorf("out.Amendments[0].AmendingDocID = %v, want %s", gotAmend.AmendingDocID, amendingID.String())
	}
	if gotAmend.Clause != "Điều 5" {
		t.Errorf("out.Amendments[0].Clause = %q, want %q", gotAmend.Clause, "Điều 5")
	}
	if gotAmend.Kind != "amended" {
		t.Errorf("out.Amendments[0].Kind = %q, want %q", gotAmend.Kind, "amended")
	}
}

func TestDocumentHandlerNilAmendingDocIDStaysNil(t *testing.T) {
	docID := uuid.New()
	stub := &stubDocGetter{detail: store.DocumentDetail{
		Doc: store.Document{
			ID: docID, CorpusID: "vn-reg", Title: "T", ValidityStatus: "in_force", ObservedAt: time.Now(),
		},
		Events: []store.AmendmentEvent{{TargetDocID: docID, Clause: "Điều 9", EventDate: time.Now()}},
	}}
	h := newDocumentHandler(stub, "mise_public")

	_, out, err := h(context.Background(), nil, DocumentInput{CorpusID: "vn-reg", DocumentID: docID.String()})
	if err != nil {
		t.Fatalf("handler error = %v, want nil", err)
	}
	if len(out.Amendments) != 1 {
		t.Fatalf("len(out.Amendments) = %d, want 1", len(out.Amendments))
	}
	if out.Amendments[0].AmendingDocID != nil {
		t.Errorf("AmendingDocID = %v, want nil (no amending doc on this event)", *out.Amendments[0].AmendingDocID)
	}
}

func TestDocumentHandlerEmptySectionsAndAmendmentsProduceEmptySlicesNotNil(t *testing.T) {
	docID := uuid.New()
	stub := &stubDocGetter{detail: store.DocumentDetail{
		Doc: store.Document{
			ID: docID, CorpusID: "vn-reg", Title: "T", ValidityStatus: "in_force", ObservedAt: time.Now(),
		},
	}}
	h := newDocumentHandler(stub, "mise_public")

	_, out, err := h(context.Background(), nil, DocumentInput{CorpusID: "vn-reg", DocumentID: docID.String()})
	if err != nil {
		t.Fatalf("handler error = %v, want nil", err)
	}
	if out.Sections == nil {
		t.Error("out.Sections = nil, want non-nil empty slice")
	}
	if out.Amendments == nil {
		t.Error("out.Amendments = nil, want non-nil empty slice")
	}
}

// --- wiring: New()/WithEvidence registers exactly the two evidence tools --

func TestNewRegistersEvidenceToolsOnlyWithWithEvidence(t *testing.T) {
	ctx := context.Background()

	bare := New()
	if names := connectAndListToolNames(t, ctx, bare); len(names) != 0 {
		t.Errorf("New() (no options) tools = %v, want none (healthz-only serving must stay zero-dep)", names)
	}

	wired := New(WithEvidence(&stubSearcher{}, &stubDocGetter{}, "mise_public"))
	names := connectAndListToolNames(t, ctx, wired)
	want := map[string]bool{"search": true, "document": true}
	if len(names) != len(want) {
		t.Fatalf("New(WithEvidence(...)) tools = %v, want exactly %v", names, want)
	}
	for _, n := range names {
		if !want[n] {
			t.Errorf("unexpected tool %q registered", n)
		}
	}
}

// connectAndListToolNames connects an in-memory client to srv and returns
// the names of every tool it advertises.
func connectAndListToolNames(t *testing.T, ctx context.Context, srv *Server) []string {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	t1, t2 := mcp.NewInMemoryTransports()

	if _, err := srv.mcp.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("connecting server transport: %v", err)
	}
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("connecting client: %v", err)
	}
	defer func() { _ = session.Close() }()

	res, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := make([]string, len(res.Tools))
	for i, tl := range res.Tools {
		names[i] = tl.Name
	}
	return names
}
