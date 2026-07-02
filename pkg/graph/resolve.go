package graph

import (
	"encoding/json"

	"github.com/google/uuid"

	"danny.vn/mise/pkg/corpus"
)

// ResolvedRef is where a RawControlRef's target resolves to: a graph node
// that's already been ingested (IsStub=false, Target populated), or a
// doc_ref stub key awaiting that document's future ingest (IsStub=true) —
// the doc_ref_unresolved_idx case (types.go's DocRef). ToRefID is always
// left the zero uuid.UUID here: creating or finding the graph.doc_ref row
// (and filling ToRefID with its id) is the writer's job, not this pure
// resolver's — even a resolved Target still needs that row to exist, since
// relation_edge.to_ref_id is a foreign key to doc_ref, not to the document
// directly.
//
// RefKey/Label/SrcRef carry the target's citation metadata through to the
// writer: GraphStore.WriteExtractedEdge reads them to create the target's
// graph.doc_ref row (ref_key/label/src_ref). ResolveRef populates them from
// the RawControlRef so the resolution is complete for both the resolved and
// stub cases.
type ResolvedRef struct {
	Target     NodeRef
	ToRefID    uuid.UUID
	ToCorpusID string
	IsStub     bool

	// RefKey is the target's cited doc number (RawControlRef.TargetNumber),
	// raw/pre-normalization. GraphStore.EnsureDocRef corpus-scopes and
	// upper-cases it into doc_ref.ref_key.
	RefKey string
	// Label is the citation's target title (RawControlRef.TargetTitle),
	// stored as doc_ref.label.
	Label string
	// SrcRef is the raw citation payload (the marshaled RawControlRef)
	// carried through unchanged into doc_ref.src_ref, for audit.
	SrcRef json.RawMessage
}

// ExtractedEdge is one Method A candidate edge: a source document's NodeRef,
// the edge type EdgeTypeForPair derives for its pair, and a RawControlRef's
// resolution (ResolveRef) as the target — ready for a future writer to
// persist as a graph.relation_edge row (From/EdgeType/Direction/Target) plus
// its graph.relation_evidence row (QuotedFromSpan/QuotedToSpan/CreatedBy,
// EvidenceKind=extracted).
type ExtractedEdge struct {
	From           NodeRef
	EdgeType       string
	Direction      string
	QuotedFromSpan string
	QuotedToSpan   string
	CreatedBy      string
	Target         ResolvedRef
}

// controlChainEdgeType is the fixed control-chain (from, to) corpus pairs
// for the two edge types with no corresponding corpus registry field:
// "derives" (local-sop→local-policy) and "implements" (local-policy→
// group-std) — mirrors relation_edge.edge_type's CHECK constraint values
// (this package's EdgeType consts, migrations/009_graph_tables.sql). The
// third recognized edge type, "satisfies", is deliberately NOT hardcoded
// here: EdgeTypeForPair derives it from the registry's canonical
// GraphRole.SatisfiesTarget field instead, so corpus.go stays the one place
// that says group-std satisfies my-reg and local-policy satisfies vn-reg.
// Method A (doc-control header parsing, this package) only ever produces
// "implements"/"derives" refs (doccontrol.go's normalizeRelation), so
// ResolveRef only ever reaches the two pairs below — "satisfies" is resolved
// for completeness; M3 (semantic citation matching, a later milestone)
// writes those from the same corpus registry shape.
var controlChainEdgeType = map[[2]corpus.ID]EdgeType{
	{corpus.LocalSOP, corpus.LocalPolicy}: EdgeDerives,
	{corpus.LocalPolicy, corpus.GroupStd}: EdgeImplements,
}

// EdgeTypeForPair returns the edge type for an edge sourced from corpus from
// and targeting corpus to, and whether that pair is a recognized
// control-chain edge at all. "derives" and "implements" come from the fixed
// table above; "satisfies" is derived rather than hardcoded — to is a
// satisfies target of from iff corpus.Get(from) finds a descriptor whose
// GraphRole.SatisfiesTarget is non-empty and equals to. Any pair outside
// these — including from==to, and either id absent from the corpus
// registry — returns ("", false) rather than guessing.
func EdgeTypeForPair(from, to corpus.ID) (string, bool) {
	if edgeType, ok := controlChainEdgeType[[2]corpus.ID{from, to}]; ok {
		return string(edgeType), true
	}
	desc, ok := corpus.Get(from)
	if !ok || desc.GraphRole.SatisfiesTarget == "" || desc.GraphRole.SatisfiesTarget != to {
		return "", false
	}
	return string(EdgeSatisfies), true
}

// ResolveRef resolves one RawControlRef, read from a doc-control header in a
// corpus-from document, to the graph node it names. lookup resolves a
// target corpus + document number to that document's id; the writer backs
// it with a real query against already-ingested documents — ResolveRef
// itself never touches a store or a model.
//
// The target corpus is decided from ref.Relation and from, per the same two
// Method-A-reachable pairs EdgeTypeForPair maps: "implements" from
// local-policy targets group-std; "derives" from local-sop targets
// local-policy. Any other (from, relation) combination — an unrecognized
// relation, or a source corpus that pair doesn't apply to — is ambiguous:
// ResolveRef returns (_, false) rather than guessing a target, and never
// calls lookup.
//
// Once the target corpus is decided, lookup(target, ref.TargetNumber) finds
// the already-ingested document (a title-only ref, with an empty
// TargetNumber, simply never matches): found reports a resolved Target
// (IsStub=false); not found reports a stub (IsStub=true) for the writer to
// create as a graph.doc_ref row awaiting that document's future ingest.
func ResolveRef(
	from corpus.ID, ref RawControlRef, lookup func(target corpus.ID, number string) (uuid.UUID, bool),
) (ResolvedRef, bool) {
	target, ok := targetCorpusFor(from, ref.Relation)
	if !ok {
		return ResolvedRef{}, false
	}

	if docID, found := lookup(target, ref.TargetNumber); found {
		return ResolvedRef{
			Target:     NodeRef{CorpusID: string(target), DocumentID: docID},
			ToCorpusID: string(target),
			IsStub:     false,
			RefKey:     ref.TargetNumber,
			Label:      ref.TargetTitle,
			SrcRef:     marshalRef(ref),
		}, true
	}
	return ResolvedRef{
		ToCorpusID: string(target),
		IsStub:     true,
		RefKey:     ref.TargetNumber,
		Label:      ref.TargetTitle,
		SrcRef:     marshalRef(ref),
	}, true
}

// marshalRef renders a RawControlRef as the doc_ref.src_ref audit payload.
// RawControlRef is all-string, so json.Marshal cannot fail; a defensive
// fallback keeps the resolver total (no error return in ResolveRef's
// canonical signature).
func marshalRef(ref RawControlRef) json.RawMessage {
	b, err := json.Marshal(ref)
	if err != nil {
		return json.RawMessage("{}")
	}
	return b
}

// targetCorpusFor decides ResolveRef's target corpus from the source corpus
// and the ref's relation verb — the same two Method-A pairs
// controlChainEdgeType maps for "implements"/"derives". ok is false for
// every other (from, relation) combination: no target corpus is derivable,
// so the caller must drop the ref rather than guess one.
func targetCorpusFor(from corpus.ID, relation string) (corpus.ID, bool) {
	switch {
	case from == corpus.LocalPolicy && relation == string(EdgeImplements):
		return corpus.GroupStd, true
	case from == corpus.LocalSOP && relation == string(EdgeDerives):
		return corpus.LocalPolicy, true
	default:
		return "", false
	}
}
