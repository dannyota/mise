package ingest_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/ingest"
	"danny.vn/mise/pkg/parse/law"
)

// descriptor looks up a registered corpus descriptor, failing the test if the
// registry doesn't have it — Normalize is always called with a real
// descriptor in production, so tests exercise the real registry rather than
// a hand-rolled stand-in that could drift from it.
func descriptor(t *testing.T, id corpus.ID) corpus.Descriptor {
	t.Helper()
	d, ok := corpus.Get(id)
	if !ok {
		t.Fatalf("corpus.Get(%q): not found", id)
	}
	return d
}

// baseSrc returns a representative vbpl-shaped DiscoveredDoc; tests copy and
// mutate the fields they care about.
func baseSrc() ingest.DiscoveredDoc {
	return ingest.DiscoveredDoc{
		SourceID:    "vbpl",
		ExternalID:  "12345",
		Number:      "11/2026/TT-NHNN",
		Title:       "Thông tư quy định về hoạt động cho vay",
		Issuer:      "Ngân hàng Nhà nước Việt Nam",
		Signer:      "Nguyễn Văn A",
		IssuedAt:    time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		EffectiveAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		Status:      "CHL",
		DetailURL:   "https://vbpl.vn/van-ban/chi-tiet/12345",
	}
}

func sampleTree() []*law.Node {
	return []*law.Node{
		{
			Kind: "dieu", Label: "Điều 1", Heading: "Phạm vi điều chỉnh",
			Content:      "Thông tư này quy định về hoạt động cho vay.",
			CitationPath: "dieu-1",
		},
		{
			Kind: "dieu", Label: "Điều 2", Heading: "Đối tượng áp dụng",
			Content:      "Ngân hàng thương mại nhà nước.",
			CitationPath: "dieu-2",
		},
	}
}

func TestNormalizeDocumentMapping(t *testing.T) {
	desc := descriptor(t, corpus.VNReg)
	src := baseSrc()
	runID := uuid.New()
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	got, err := ingest.Normalize(desc, src, sampleTree(), "", runID, now)
	if err != nil {
		t.Fatalf("Normalize() error = %v, want nil", err)
	}

	doc := got.Doc
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"CorpusID", doc.CorpusID, string(corpus.VNReg)},
		{"Title", doc.Title, src.Title},
		{"DocNumber", doc.DocNumber, src.Number},
		{"CitationScheme", doc.CitationScheme, desc.CitationScheme},
		{"Language", doc.Language, "vi"},
		{"ValidityStatus", doc.ValidityStatus, ingest.StatusInForce},
		{"IssuingAuthority", doc.IssuingAuthority, src.Issuer},
		{"SignerName", doc.SignerName, src.Signer},
		{"SourceURL", doc.SourceURL, src.DetailURL},
		{"SourceSystem", doc.SourceSystem, src.SourceID},
		{"AccessTier", doc.AccessTier, string(desc.AccessTier)},
		{"IngestRunID", doc.IngestRunID, runID},
		// Fields with no current DiscoveredDoc source stay at zero value —
		// deliberate, not an oversight (see Normalize's doc comment).
		{"CitationPath", doc.CitationPath, ""},
		{"Version", doc.Version, ""},
		{"ContentType", doc.ContentType, ""},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("Doc.%s = %v, want %v", c.name, c.got, c.want)
		}
	}

	if doc.ID == uuid.Nil {
		t.Error("Doc.ID must be a generated, non-nil uuid")
	}
	if !doc.ObservedAt.Equal(now) {
		t.Errorf("Doc.ObservedAt = %v, want %v", doc.ObservedAt, now)
	}
	if doc.IssuedDate == nil || !doc.IssuedDate.Equal(src.IssuedAt) {
		t.Errorf("Doc.IssuedDate = %v, want %v", doc.IssuedDate, src.IssuedAt)
	}
	if doc.EffectiveDate == nil || !doc.EffectiveDate.Equal(src.EffectiveAt) {
		t.Errorf("Doc.EffectiveDate = %v, want %v", doc.EffectiveDate, src.EffectiveAt)
	}
	if doc.ExpiryDate != nil {
		t.Errorf("Doc.ExpiryDate = %v, want nil (src.ExpireAt is zero)", doc.ExpiryDate)
	}
}

func TestNormalizeLanguageByJurisdiction(t *testing.T) {
	tests := []struct {
		name string
		id   corpus.ID
		want string
	}{
		{"vn jurisdiction maps to Vietnamese", corpus.VNReg, "vi"},
		{"my jurisdiction maps to English", corpus.MYReg, "en"},
	}
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc := descriptor(t, tt.id)
			got, err := ingest.Normalize(desc, baseSrc(), sampleTree(), "", uuid.New(), now)
			if err != nil {
				t.Fatalf("Normalize() error = %v, want nil", err)
			}
			if got.Doc.Language != tt.want {
				t.Errorf("Doc.Language = %q, want %q", got.Doc.Language, tt.want)
			}
		})
	}
}

func TestNormalizeSectionsInheritDocValidityAndTier(t *testing.T) {
	desc := descriptor(t, corpus.VNReg)
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	got, err := ingest.Normalize(desc, baseSrc(), sampleTree(), "", uuid.New(), now)
	if err != nil {
		t.Fatalf("Normalize() error = %v, want nil", err)
	}

	flat := law.Flatten(sampleTree())
	if len(got.Sections) != len(flat) {
		t.Fatalf("got %d sections, want %d", len(got.Sections), len(flat))
	}

	seen := map[uuid.UUID]bool{}
	for i, s := range got.Sections {
		if s.CitationPath != flat[i].CitationPath || s.HeadingPath != flat[i].HeadingPath || s.Body != flat[i].Body {
			t.Errorf("section[%d] = %+v, want citation/heading/body from %+v", i, s, flat[i])
		}
		if s.ID == uuid.Nil || seen[s.ID] {
			t.Errorf("section[%d].ID = %v, want a fresh non-nil uuid", i, s.ID)
		}
		seen[s.ID] = true
		if s.DocumentID != got.Doc.ID {
			t.Errorf("section[%d].DocumentID = %v, want %v", i, s.DocumentID, got.Doc.ID)
		}
		if s.CorpusID != got.Doc.CorpusID {
			t.Errorf("section[%d].CorpusID = %q, want %q", i, s.CorpusID, got.Doc.CorpusID)
		}
		if s.ValidityStatus != got.Doc.ValidityStatus {
			t.Errorf("section[%d].ValidityStatus = %q, want %q", i, s.ValidityStatus, got.Doc.ValidityStatus)
		}
		if s.AccessTier != got.Doc.AccessTier {
			t.Errorf("section[%d].AccessTier = %q, want %q", i, s.AccessTier, got.Doc.AccessTier)
		}
		if s.EffectiveDate == nil || !s.EffectiveDate.Equal(*got.Doc.EffectiveDate) {
			t.Errorf("section[%d].EffectiveDate = %v, want %v", i, s.EffectiveDate, got.Doc.EffectiveDate)
		}
	}
}

func TestNormalizeEmptyTreeFallsBackToFallbackText(t *testing.T) {
	desc := descriptor(t, corpus.VNReg)
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		tree []*law.Node
	}{
		{"nil tree", nil},
		{"empty tree slice", []*law.Node{}},
		{"tree with only structural nodes and no leaf content", []*law.Node{
			{Kind: "chuong", Label: "Chương I", Heading: "QUY ĐỊNH CHUNG", CitationPath: "chuong-I"},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ingest.Normalize(desc, baseSrc(), tt.tree, "fallback body text", uuid.New(), now)
			if err != nil {
				t.Fatalf("Normalize() error = %v, want nil", err)
			}
			if len(got.Sections) != 1 {
				t.Fatalf("got %d sections, want 1 fallback section", len(got.Sections))
			}
			s := got.Sections[0]
			if s.CitationPath != "" {
				t.Errorf("fallback section CitationPath = %q, want empty", s.CitationPath)
			}
			if s.Body != "fallback body text" {
				t.Errorf("fallback section Body = %q, want fallbackText", s.Body)
			}
			if s.DocumentID != got.Doc.ID {
				t.Errorf("fallback section DocumentID = %v, want %v", s.DocumentID, got.Doc.ID)
			}
			if s.ValidityStatus != got.Doc.ValidityStatus {
				t.Errorf("fallback section ValidityStatus = %q, want %q", s.ValidityStatus, got.Doc.ValidityStatus)
			}
		})
	}
}

func TestNormalizeRelationEvents(t *testing.T) {
	desc := descriptor(t, corpus.VNReg)
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	src := baseSrc()
	src.Relations = []ingest.Relation{
		{Type: "replaces", TargetNumber: "5/2020/TT-NHNN"},
		{Type: "legal_basis", TargetNumber: "46/2010/QH12"},  // informational: filtered
		{Type: "amends_supplements", TargetNumber: ""},       // no resolvable target: filtered
		{Type: "amends_supplements", TargetNumber: "  \t  "}, // whitespace-only target: filtered
	}

	got, err := ingest.Normalize(desc, src, sampleTree(), "", uuid.New(), now)
	if err != nil {
		t.Fatalf("Normalize() error = %v, want nil", err)
	}
	if len(got.RelationEvents) != 1 {
		t.Fatalf("got %d relation events, want 1: %+v", len(got.RelationEvents), got.RelationEvents)
	}
	ev := got.RelationEvents[0]
	if ev.TargetDocNumber != "5/2020/TT-NHNN" {
		t.Errorf("RelationEvents[0].TargetDocNumber = %q, want %q", ev.TargetDocNumber, "5/2020/TT-NHNN")
	}
	if ev.Kind != ingest.StatusSuperseded {
		t.Errorf("RelationEvents[0].Kind = %q, want %q", ev.Kind, ingest.StatusSuperseded)
	}
	if ev.Clause != "" {
		t.Errorf("RelationEvents[0].Clause = %q, want empty (no clause-level source data)", ev.Clause)
	}
	if !ev.Date.Equal(src.EffectiveAt) {
		t.Errorf("RelationEvents[0].Date = %v, want src.EffectiveAt %v", ev.Date, src.EffectiveAt)
	}
}

func TestNormalizeRelationEventDateFallsBackToIssuedAt(t *testing.T) {
	desc := descriptor(t, corpus.VNReg)
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	src := baseSrc()
	src.EffectiveAt = time.Time{} // source omitted the effective date
	src.Relations = []ingest.Relation{{Type: "amends_supplements", TargetNumber: "X/2021"}}

	got, err := ingest.Normalize(desc, src, sampleTree(), "", uuid.New(), now)
	if err != nil {
		t.Fatalf("Normalize() error = %v, want nil", err)
	}
	if len(got.RelationEvents) != 1 {
		t.Fatalf("got %d relation events, want 1", len(got.RelationEvents))
	}
	if !got.RelationEvents[0].Date.Equal(src.IssuedAt) {
		t.Errorf("RelationEvents[0].Date = %v, want src.IssuedAt %v", got.RelationEvents[0].Date, src.IssuedAt)
	}
}

// TestNormalizeFutureDatedEventComposesWithTransitionAt demonstrates the
// caller's rule from the task brief: Normalize always produces the event row
// (regardless of date), and it is TransitionAt — applied later by the pipeline
// once the target doc's current status is resolved — that withholds the
// status change until the event date arrives.
func TestNormalizeFutureDatedEventComposesWithTransitionAt(t *testing.T) {
	desc := descriptor(t, corpus.VNReg)
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	src := baseSrc()
	src.EffectiveAt = now.AddDate(0, 1, 0) // future relative to now
	src.Relations = []ingest.Relation{{Type: "repeals", TargetNumber: "OLD-DOC/2015"}}

	got, err := ingest.Normalize(desc, src, sampleTree(), "", uuid.New(), now)
	if err != nil {
		t.Fatalf("Normalize() error = %v, want nil", err)
	}
	if len(got.RelationEvents) != 1 {
		t.Fatalf("got %d relation events, want 1", len(got.RelationEvents))
	}
	ev := got.RelationEvents[0]
	if ev.Kind != ingest.StatusRepealed {
		t.Fatalf("RelationEvents[0].Kind = %q, want %q", ev.Kind, ingest.StatusRepealed)
	}
	if !ev.Date.After(now) {
		t.Fatalf("RelationEvents[0].Date = %v, want it strictly after now %v", ev.Date, now)
	}

	targetCurrent := ingest.StatusInForce
	if got := ingest.TransitionAt(targetCurrent, ev.Kind, ev.Date, now); got != targetCurrent {
		t.Errorf("TransitionAt at now = %q, want unchanged %q (event is future-dated)", got, targetCurrent)
	}
	laterNow := ev.Date.AddDate(0, 0, 1)
	if got := ingest.TransitionAt(targetCurrent, ev.Kind, ev.Date, laterNow); got != ingest.StatusRepealed {
		t.Errorf("TransitionAt once event date has passed = %q, want %q", got, ingest.StatusRepealed)
	}
}

func TestNormalizeValidation(t *testing.T) {
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	desc := descriptor(t, corpus.VNReg)

	noIdentity := baseSrc()
	noIdentity.Number = ""
	noIdentity.DetailURL = ""

	tests := []struct {
		name  string
		desc  corpus.Descriptor
		src   ingest.DiscoveredDoc
		runID uuid.UUID
	}{
		{"zero-value descriptor", corpus.Descriptor{}, baseSrc(), uuid.New()},
		{"nil ingest run id", desc, baseSrc(), uuid.Nil},
		{"doc number and source url both empty", desc, noIdentity, uuid.New()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ingest.Normalize(tt.desc, tt.src, sampleTree(), "", tt.runID, now)
			if err == nil {
				t.Fatal("Normalize() error = nil, want a validation error")
			}
		})
	}
}
