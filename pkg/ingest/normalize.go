package ingest

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/parse/law"
	"danny.vn/mise/pkg/store"
)

// RelationEvent is one src.Relations edge that EventKind classified as an
// amendment event, paired with the target document's citation number instead
// of its store uuid. Normalize cannot resolve TargetDocNumber to a uuid: the
// target document may not exist in the store yet (or may exist under a uuid
// this call has no way to know), so that resolution — and the eventual
// store.AmendmentEvent insert, with AmendingDocID set to the just-normalized
// Doc.ID — is the ingest pipeline's job at index time, after every document
// batch has been written and doc-number lookups are possible.
type RelationEvent struct {
	TargetDocNumber string
	Kind            string
	Clause          string
	Date            time.Time
}

// NormalizedDoc is Normalize's output: a store.Document with its flattened
// store.Section rows and the pending RelationEvents extracted from
// src.Relations. This shape deviates from the task brief's original
// `Events []store.AmendmentEvent` — see RelationEvent's doc comment for why.
type NormalizedDoc struct {
	Doc            store.Document
	Sections       []store.Section
	RelationEvents []RelationEvent
}

// Normalize maps a source-discovered document and its parsed structure tree
// into the validity-enveloped Document/Section rows for corpus desc, and
// classifies each src.Relations edge into a pending RelationEvent.
//
// Language is "vi" for jurisdiction "vn" and "en" otherwise (matching the
// document table's own "en" default — today the only other registered
// jurisdiction is "my"). CitationScheme and AccessTier come from desc;
// SourceURL/SourceSystem/ObservedAt/IngestRunID are ingest provenance.
// ValidityStatus is MapValidity(src.Status, src.EffectiveAt, now).
//
// The tree is flattened with law.Flatten; when that yields no sections (a
// nil/empty tree, or a tree with no node carrying its own content),
// Sections falls back to one section with an empty CitationPath and Body
// set to fallbackText. Every Section inherits the Document's ValidityStatus,
// AccessTier, and EffectiveDate.
//
// CitationPath, Version, and ContentType have no corresponding field on
// DiscoveredDoc yet and are left at their zero value rather than guessed;
// a later task can populate them once the upstream data exists.
//
// Normalize returns an error only for caller-side misuse: a zero-value desc
// (ID empty — the caller likely forgot a corpus.Get lookup) or a nil runID.
func Normalize(
	desc corpus.Descriptor, src DiscoveredDoc, tree []*law.Node, fallbackText string,
	runID uuid.UUID, now time.Time,
) (NormalizedDoc, error) {
	if desc.ID == "" {
		return NormalizedDoc{}, errors.New("normalize: empty corpus descriptor (did the caller call corpus.Get?)")
	}
	if runID == uuid.Nil {
		return NormalizedDoc{}, errors.New("normalize: runID must not be the nil uuid")
	}

	validity := MapValidity(src.Status, src.EffectiveAt, now)
	docID := uuid.New()
	doc := store.Document{
		ID:               docID,
		CorpusID:         string(desc.ID),
		Title:            src.Title,
		DocNumber:        src.Number,
		CitationScheme:   desc.CitationScheme,
		Language:         language(desc.Jurisdiction),
		ValidityStatus:   validity,
		IssuingAuthority: src.Issuer,
		SignerName:       src.Signer,
		SourceURL:        src.DetailURL,
		SourceSystem:     src.SourceID,
		AccessTier:       string(desc.AccessTier),
		IssuedDate:       datePtr(src.IssuedAt),
		EffectiveDate:    datePtr(src.EffectiveAt),
		ExpiryDate:       datePtr(src.ExpireAt),
		IngestRunID:      runID,
		ObservedAt:       now,
	}

	return NormalizedDoc{
		Doc:            doc,
		Sections:       sections(doc, tree, fallbackText),
		RelationEvents: relationEvents(src),
	}, nil
}

// language derives store.Document.Language from a corpus jurisdiction.
func language(jurisdiction string) string {
	if jurisdiction == "vn" {
		return "vi"
	}
	return "en"
}

// datePtr returns a pointer to t, or nil when t is the zero value.
// DiscoveredDoc uses time.Time{} to mean "the source omitted this date"
// (pkg/ingest/source.go), which must become SQL NULL, not a garbage
// 0001-01-01 timestamp.
func datePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return new(t)
}

// sections flattens tree into store.Section rows inheriting doc's validity
// envelope, falling back to one fallbackText section when flattening yields
// nothing.
func sections(doc store.Document, tree []*law.Node, fallbackText string) []store.Section {
	flat := law.Flatten(tree)
	if len(flat) == 0 {
		flat = []law.FlatSection{{Body: fallbackText}}
	}
	out := make([]store.Section, 0, len(flat))
	for _, f := range flat {
		out = append(out, store.Section{
			ID:             uuid.New(),
			DocumentID:     doc.ID,
			CorpusID:       doc.CorpusID,
			CitationPath:   f.CitationPath,
			HeadingPath:    f.HeadingPath,
			Body:           f.Body,
			ValidityStatus: doc.ValidityStatus,
			AccessTier:     doc.AccessTier,
			EffectiveDate:  doc.EffectiveDate,
		})
	}
	return out
}

// relationEvents classifies src.Relations into RelationEvents, dropping
// informational (non-amending) edges and edges with no target citation
// number to resolve against later. Every kept event shares one Date: the
// acting document's (src's) own EffectiveAt, falling back to IssuedAt when
// the source omitted an effective date — the date this document's amendment/
// supersession/repeal of the target takes effect.
func relationEvents(src DiscoveredDoc) []RelationEvent {
	date := src.EffectiveAt
	if date.IsZero() {
		date = src.IssuedAt
	}

	var out []RelationEvent
	for _, rel := range src.Relations {
		target := strings.TrimSpace(rel.TargetNumber)
		if target == "" {
			continue
		}
		kind, ok := EventKind(rel)
		if !ok {
			continue
		}
		out = append(out, RelationEvent{
			TargetDocNumber: target,
			Kind:            kind,
			Date:            date,
		})
	}
	return out
}
