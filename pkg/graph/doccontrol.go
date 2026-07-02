package graph

import (
	"strings"

	"danny.vn/mise/pkg/corpus"
)

// DocControlHeader is the parsed doc-control envelope for one document: the
// signer/approval block a source's metadata parser (M1b) extracts, carrying
// whatever explicit upstream-control references the document names. An
// absent header (the zero value, with no ControlRefs) is not an error —
// ParseControlRefs yields no edges for it (RISKS R6: an absent or
// unparsable header must never produce a guessed edge).
type DocControlHeader struct {
	Corpus                     corpus.ID
	DocNumber, Title, Version  string
	OwnerDepartment, OwnerRole string
	ControlRefs                []RawControlRef
}

// RawControlRef is one explicit upstream-control reference as named in a
// doc-control header, before resolution to a graph node (M2-6, the
// reference resolver). Relation is the verb the header used, e.g.
// "implements" or "derives"; TargetNumber/TargetTitle identify the
// referenced control document; QuotedSpan is the verbatim header text the
// reference was read from, carried through unchanged for audit
// (DATA-MODEL §5).
type RawControlRef struct {
	Relation                              string
	TargetNumber, TargetTitle, QuotedSpan string
}

// ParseControlRefs extracts the qualifying control references from a parsed
// doc-control header. This is Method A (M2 extraction): deterministic, no
// model call, no guessing.
//
// A ref qualifies when both hold:
//   - its Relation normalizes to "implements" or "derives" (case/whitespace
//     insensitive; "derives from" normalizes to "derives"); anything else is
//     rejected.
//   - it names a target: a non-empty (post-trim) TargetNumber or TargetTitle.
//
// A ref that fails either check is dropped, never guessed. An absent header
// (h.ControlRefs nil or empty) or one with zero qualifying refs returns an
// empty, non-nil slice — never nil, so an unparsed header and a parsed-but-
// empty one both read as "no edges" without a nil check masking the
// distinction (RISKS R6).
func ParseControlRefs(h DocControlHeader) []RawControlRef {
	out := make([]RawControlRef, 0, len(h.ControlRefs))
	for _, ref := range h.ControlRefs {
		relation, ok := normalizeRelation(ref.Relation)
		if !ok {
			continue
		}
		targetNumber := strings.TrimSpace(ref.TargetNumber)
		targetTitle := strings.TrimSpace(ref.TargetTitle)
		if targetNumber == "" && targetTitle == "" {
			continue
		}
		out = append(out, RawControlRef{
			Relation:     relation,
			TargetNumber: targetNumber,
			TargetTitle:  targetTitle,
			QuotedSpan:   ref.QuotedSpan,
		})
	}
	return out
}

// normalizeRelation lowercases and trims rel, then maps it to the canonical
// "implements"/"derives" relation verb. ok is false when rel is neither (nor
// a case/whitespace variant of either) — the caller must drop the ref
// rather than guess which relation was meant.
func normalizeRelation(rel string) (relation string, ok bool) {
	switch strings.ToLower(strings.TrimSpace(rel)) {
	case "implements":
		return "implements", true
	case "derives", "derives from":
		return "derives", true
	default:
		return "", false
	}
}
