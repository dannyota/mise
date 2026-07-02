package graph

import "danny.vn/mise/pkg/corpus"

// ExtractEdges is Method A's edge-extraction entrypoint (M2-9): deterministic,
// no model/judge/network call. It parses h's doc-control header
// (ParseControlRefs), resolves each qualifying ref via resolve — the caller
// closes resolve over ResolveRef plus a real doc-number lookup against
// already-ingested documents, and h.Corpus as ResolveRef's source corpus —
// and assembles a ready-to-write ExtractedEdge for every ref that resolves.
//
// resolve's own type, func(RawControlRef) (ResolvedRef, bool), takes and
// returns only value types: no context.Context, no store/pool handle, no
// model client. Combined with this function's own signature (desc, h, and
// that pure func — nothing else), ExtractEdges has no seam through which a
// network or model call could occur; it can only do arithmetic over
// resolve's return value. A ref resolve reports unresolved (ok=false) is
// dropped, never guessed (RISKS R6). desc.ID is the source corpus for both
// the produced edges' From.CorpusID and the EdgeTypeForPair lookup; a
// resolved ref whose (desc.ID, ResolvedRef.ToCorpusID) pair
// EdgeTypeForPair doesn't recognize is also dropped — defensive, since a
// resolve built from ResolveRef only ever resolves the pairs
// EdgeTypeForPair itself recognizes.
//
// An absent header (h.ControlRefs nil or empty, or one with zero qualifying
// refs) returns an empty, non-nil slice — resolve is never even called.
func ExtractEdges(
	desc corpus.Descriptor, h DocControlHeader, resolve func(RawControlRef) (ResolvedRef, bool),
) []ExtractedEdge {
	refs := ParseControlRefs(h)
	edges := make([]ExtractedEdge, 0, len(refs))
	for _, ref := range refs {
		resolved, ok := resolve(ref)
		if !ok {
			continue
		}
		edgeType, ok := EdgeTypeForPair(desc.ID, corpus.ID(resolved.ToCorpusID))
		if !ok {
			continue
		}
		edges = append(edges, ExtractedEdge{
			From:           NodeRef{CorpusID: string(desc.ID), DocumentID: h.DocID},
			EdgeType:       edgeType,
			Direction:      "up",
			QuotedFromSpan: ref.QuotedSpan,
			QuotedToSpan:   "",
			CreatedBy:      h.AttestationOwner,
			Target:         resolved,
		})
	}
	return edges
}
