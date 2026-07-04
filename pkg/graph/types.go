package graph

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// EdgeType classifies how a NodeRef relates to a DocRef target in the
// compliance graph. Mirrors the CHECK constraint on
// graph.relation_edge.edge_type (migrations/009_graph_tables.sql).
type EdgeType string

// Predefined edge types.
const (
	EdgeSatisfies  EdgeType = "satisfies"
	EdgeImplements EdgeType = "implements"
	EdgeDerives    EdgeType = "derives"
	EdgeCovers     EdgeType = "covers"
	EdgeConcerns   EdgeType = "concerns"
)

// EvidenceKind classifies how an Evidence row was produced. Mirrors the
// CHECK constraint on graph.relation_evidence.evidence_kind
// (migrations/009_graph_tables.sql).
type EvidenceKind string

// Predefined evidence kinds.
const (
	EvidenceExtracted           EvidenceKind = "extracted"
	EvidenceModelClassification EvidenceKind = "model_classification"
	EvidenceHumanAttested       EvidenceKind = "human_attested"
)

// Tier is a graph edge's confidentiality tier. It is always DB-derived:
// graph.relation_edge.access_tier is a GENERATED ALWAYS AS
// (graph.stricter_tier(graph.corpus_tier(from_corpus_id),
// graph.corpus_tier(to_corpus_id))) STORED column
// (migrations/009_graph_tables.sql) that rejects any direct write, so app
// code only ever reads a Tier back — it never constructs one to persist.
type Tier string

// Predefined tiers, ordered least-to-most strict; see TierRank.
const (
	TierPublic            Tier = "public"
	TierGroupConfidential Tier = "group-confidential"
	TierLocalConfidential Tier = "local-confidential"
)

// TierRank returns t's strictness rank for ordering/display — 0 (public) is
// least strict, 2 (local-confidential) is most. It mirrors
// graph.tier_rank(text) exactly, including that function's fail-closed ELSE
// branch: any unrecognized Tier ranks as strict as TierLocalConfidential.
func TierRank(t Tier) int {
	switch t {
	case TierPublic:
		return 0
	case TierGroupConfidential:
		return 1
	default:
		return 2
	}
}

// NodeRef identifies one resolved location in the compliance graph: a
// document, optionally narrowed to one of its sections, within a corpus.
// It is the shape of relation_edge's "from" side (from_corpus_id,
// from_document_id, from_section_id) — always resolved, since an edge can
// only originate from a document mise has already ingested — and the shape
// a DocRef resolves to once its own DocumentID is known (DocRef.Resolve).
type NodeRef struct {
	CorpusID   string
	DocumentID uuid.UUID
	SectionID  *uuid.UUID
}

// DocRef is one graph.doc_ref row: a named target the graph can point Edges
// at before (or instead of) resolving it to an ingested document/section.
// RefKey is a stable, corpus-scoped citation key; DocumentID/SectionID stay
// nil until ingest matches the citation to a real row —
// doc_ref_unresolved_idx (migrations/009_graph_tables.sql) is the partial
// index that finds exactly the still-nil rows. SrcRef carries the raw
// source citation payload that produced RefKey, for audit/debugging.
type DocRef struct {
	ID         uuid.UUID
	CorpusID   string
	RefKey     string
	Label      string
	SrcRef     json.RawMessage
	DocumentID *uuid.UUID
	SectionID  *uuid.UUID
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Resolve reports the NodeRef a DocRef points at once ingest has matched its
// RefKey to a real document — ok is false while DocumentID is still nil
// (the doc_ref_unresolved_idx case: the target hasn't been ingested yet).
func (r DocRef) Resolve() (ref NodeRef, ok bool) {
	if r.DocumentID == nil {
		return NodeRef{}, false
	}
	return NodeRef{CorpusID: r.CorpusID, DocumentID: *r.DocumentID, SectionID: r.SectionID}, true
}

// Edge is one graph.relation_edge row: a typed, directional relation from a
// resolved NodeRef (From) to a DocRef target (ToRefID/ToCorpusID).
// AccessTier is never set by the app — Postgres derives it as GENERATED
// ALWAYS AS (stricter_tier(corpus_tier(from), corpus_tier(to))) STORED
// (migrations/009_graph_tables.sql) and rejects any attempted direct write,
// so it always reflects the stricter of the two corpora's tiers.
type Edge struct {
	ID         uuid.UUID
	From       NodeRef
	ToRefID    uuid.UUID
	ToCorpusID string
	EdgeType   EdgeType
	Direction  string
	Promoted   bool
	AccessTier Tier
	CreatedAt  time.Time
}

// Evidence is one graph.relation_evidence row: the support for why an Edge
// exists — an extracted citation, a model classification (Confidence /
// GroundingScore / Rationale plus the Model+PromptHash provenance pair), or
// a human attestation (PromotedBy/PromotedAt) that promoted the edge from a
// candidate to a confirmed relation.
type Evidence struct {
	ID             uuid.UUID
	EdgeID         uuid.UUID
	EvidenceKind   EvidenceKind
	Confidence     float64
	GroundingScore float64
	Rationale      string
	QuotedFromSpan string
	QuotedToSpan   string
	Model          string
	PromptHash     string
	CreatedBy      string
	PromotedBy     string
	RunID          *uuid.UUID
	PromotedAt     *time.Time
	CreatedAt      time.Time
}
