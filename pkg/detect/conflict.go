// Package detect implements cross-corpus detectors that identify conflicts,
// gaps, and drift between regulatory documents and internal policies.
package detect

import (
	"context"
	"encoding/json"
	"fmt"

	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/store"
	"danny.vn/mise/pkg/vertex"
)

// ConflictCandidate is one pair of texts to check for grounded
// contradiction: an internal document clause (StandardRef/StandardText)
// that implements a standard, and the law clause (LawRef/LawText) it
// satisfies. The caller queries graph edges for documents that carry both
// satisfies and implements edges and materialises these pairs; the
// detector itself is a pure logic layer with no DB access.
type ConflictCandidate struct {
	StandardRef  graph.NodeRef
	StandardText string
	LawRef       graph.NodeRef
	LawText      string
}

// dedupKey builds a stable, deterministic key for this candidate so
// re-runs of the same pair produce the same finding row (ON CONFLICT DO
// NOTHING). The key is the two document IDs in a fixed order (standard
// first, law second), NUL-joined.
func (c ConflictCandidate) dedupKey() string {
	return "conflict\x00" + c.StandardRef.DocumentID.String() +
		"\x00" + c.LawRef.DocumentID.String()
}

// ConflictDeps carries the conflict detector's injectable dependencies:
// the vertex judge (contradiction mode), the grounder (checks the
// rationale), the finding store, and the score thresholds.
type ConflictDeps struct {
	Judge      vertex.Judge
	Grounder   vertex.Grounder
	Findings   FindingCreator
	Thresholds ThresholdConfig
}

// conflictEvidence is the JSON payload stored in Finding.Evidence for a
// conflict finding.
type conflictEvidence struct {
	Rationale      string  `json:"rationale"`
	QuotedFromSpan string  `json:"quoted_from_span"`
	QuotedToSpan   string  `json:"quoted_to_span"`
	GroundingScore float64 `json:"grounding_score"`
}

// ConflictDetect checks each candidate pair for grounded contradictions
// using a two-stage pipeline (locked decision #5):
//  1. Judge: classify the pair in contradiction mode.
//  2. Grounder: ground the judge's rationale against both source texts.
//
// If both stages pass the threshold gate, a "conflict" finding with
// severity "critical" is created via FindingCreator. Returns the count of
// findings created.
func ConflictDetect(
	ctx context.Context, deps ConflictDeps, candidates []ConflictCandidate,
) (int, error) {
	created := 0
	for i := range candidates {
		ok, err := checkCandidate(ctx, deps, &candidates[i])
		if err != nil {
			return created, fmt.Errorf(
				"checking candidate %d (standard %s, law %s): %w",
				i, candidates[i].StandardRef.DocumentID,
				candidates[i].LawRef.DocumentID, err,
			)
		}
		if ok {
			created++
		}
	}
	return created, nil
}

// checkCandidate runs the two-stage pipeline on one candidate pair.
// Returns true if a finding was created.
func checkCandidate(
	ctx context.Context, deps ConflictDeps, c *ConflictCandidate,
) (bool, error) {
	// Stage 1: judge for contradiction.
	jr, err := deps.Judge.Judge(ctx, c.StandardText, c.LawText)
	if err != nil {
		return false, fmt.Errorf("judge: %w", err)
	}
	if jr.EdgeType != "contradiction" {
		return false, nil
	}

	// Stage 2: ground the rationale against both source texts.
	source := c.StandardText + "\n---\n" + c.LawText
	gr, err := deps.Grounder.Ground(ctx, jr.Rationale, source)
	if err != nil {
		return false, fmt.Errorf("grounder: %w", err)
	}
	if !gr.Grounded {
		return false, nil
	}

	// Gate: both scores must meet thresholds.
	if !deps.Thresholds.Gate(jr, gr) {
		return false, nil
	}

	// Build evidence payload.
	ev, err := json.Marshal(conflictEvidence{
		Rationale:      jr.Rationale,
		QuotedFromSpan: jr.FromSpan,
		QuotedToSpan:   jr.ToSpan,
		GroundingScore: gr.Score,
	})
	if err != nil {
		return false, fmt.Errorf("marshalling evidence: %w", err)
	}

	finding := store.Finding{
		Kind:     "conflict",
		Severity: "critical",
		Status:   "open",
		NodeRefs: []store.NodeRefJSON{
			nodeRefToJSON(c.StandardRef),
			nodeRefToJSON(c.LawRef),
		},
		Evidence: ev,
		DedupKey: c.dedupKey(),
	}

	if _, err := deps.Findings.CreateFinding(ctx, finding); err != nil {
		return false, fmt.Errorf("creating finding: %w", err)
	}
	return true, nil
}

func nodeRefToJSON(r graph.NodeRef) store.NodeRefJSON {
	return store.NodeRefJSON{
		CorpusID:   r.CorpusID,
		DocumentID: r.DocumentID,
		SectionID:  r.SectionID,
	}
}
