package store

import (
	"time"

	"github.com/google/uuid"
)

// Document is one law/policy/SOP row in a corpus schema's document table
// (schema-per-corpus: vn_reg, my_reg, group_std, local_policy, local_sop — see
// migrations/002_document_tables.sql). Fields carry the validity envelope
// (ValidityStatus + the Issued/Effective/Expiry dates) and ingest provenance
// (SourceURL, SourceSystem, IngestRunID, ObservedAt).
type Document struct {
	ID               uuid.UUID
	CorpusID         string
	Title            string
	DocNumber        string
	CitationScheme   string
	CitationPath     string
	Language         string
	ValidityStatus   string
	IssuingAuthority string
	SignerName       string
	SignerRole       string
	OwnerDepartment  string
	OwnerRole        string
	Version          string
	SourceURL        string
	SourceSystem     string
	ContentType      string
	AccessTier       string
	IssuedDate       *time.Time
	EffectiveDate    *time.Time
	ExpiryDate       *time.Time
	IngestRunID      uuid.UUID
	ObservedAt       time.Time
}

// Section is one citable unit of a Document's body — a flattened law.Node
// (pkg/parse/law.Flatten) or a whole-document fallback when structure parsing
// found no nodes. ValidityStatus and AccessTier are inherited from the parent
// Document at normalize time; Embedding is populated by a later ingest stage.
// Position is the section's 0-based order within the document, stamped from
// its index in the slice ReplaceSections writes (migrations/006_search_and_
// write_keys.sql) — GetDocument reads sections back ordered by it, not by
// created_at/id, so document rendering never depends on write timing.
type Section struct {
	ID             uuid.UUID
	DocumentID     uuid.UUID
	CorpusID       string
	CitationPath   string
	HeadingPath    string
	Position       int
	Body           string
	ValidityStatus string
	AccessTier     string
	Embedding      []float32
	EffectiveDate  *time.Time
	ImageRef       string
}

// AmendmentEvent is one dated act on TargetDocID's validity — amended,
// superseded, or repealed (Kind, one of ingest.StatusAmended/
// StatusSuperseded/StatusRepealed) — optionally attributing the act to
// AmendingDocID. Both ids are only known once the target/amending documents
// are resolved in the store (see ingest.RelationEvent for the pre-resolution
// form Normalize produces, which is where Kind actually comes from).
type AmendmentEvent struct {
	TargetDocID   uuid.UUID
	AmendingDocID *uuid.UUID
	Kind          string
	Clause        string
	EventDate     time.Time
}
