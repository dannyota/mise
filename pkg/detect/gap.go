package detect

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/store"
)

// GapCandidate describes a single node that is missing downstream coverage.
// Callers build this list from graph queries (obligations with 0 satisfies
// edges, standards with 0 implements edges, policies with 0 SOP coverage)
// and pass it to GapScan.
type GapCandidate struct {
	Ref     graph.NodeRef
	Label   string
	GapType string // "no_satisfies", "no_implements", "no_sop_coverage"
}

// GapScan creates deduplicated gap findings (severity=medium) for each
// candidate. It returns the count of new findings created (duplicates from
// prior runs are silently skipped via the dedup key). The function makes no
// model calls — it only writes findings for pre-computed candidates.
func GapScan(ctx context.Context, fc FindingCreator, candidates []GapCandidate) (int, error) {
	var created int
	for _, c := range candidates {
		evidence := mustMarshalJSON(map[string]string{
			"gap_type": c.GapType,
			"label":    c.Label,
		})

		f := store.Finding{
			Kind:     "gap",
			Severity: "medium",
			Status:   "open",
			NodeRefs: []store.NodeRefJSON{nodeRefToJSON(c.Ref)},
			Evidence: evidence,
			DedupKey: fmt.Sprintf("gap:%s:%s", nodeRefKey(c.Ref), c.GapType),
		}

		id, err := fc.CreateFinding(ctx, f)
		if err != nil {
			return created, fmt.Errorf("creating gap finding for %q: %w", c.Label, err)
		}
		if id != uuid.Nil {
			created++
		}
	}
	return created, nil
}
