package detect

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/store"
)

// StaleCandidate describes a downstream document that may be outdated by a
// law amendment. Callers build this list by comparing amendment_event dates
// against downstream effective dates, cascading through policy to SOP
// descendants. StaleScan only creates findings when the amendment is strictly
// after the downstream document's effective date.
type StaleCandidate struct {
	AmendedLawRef   graph.NodeRef
	AmendmentDate   time.Time
	DownstreamRef   graph.NodeRef
	DownstreamLabel string
	EffectiveDate   time.Time
}

// StaleScan creates deduplicated staleness findings (severity=high) for each
// candidate whose amendment date is strictly after its downstream effective
// date. It returns the count of new findings created (duplicates are skipped
// via the dedup key). No model calls — only writes findings for
// pre-computed candidates.
func StaleScan(ctx context.Context, fc FindingCreator, candidates []StaleCandidate) (int, error) {
	var created int
	for _, c := range candidates {
		if !c.AmendmentDate.After(c.EffectiveDate) {
			continue
		}

		evidence := mustMarshalJSON(map[string]string{
			"amendment_date":   c.AmendmentDate.Format(time.DateOnly),
			"effective_date":   c.EffectiveDate.Format(time.DateOnly),
			"downstream_label": c.DownstreamLabel,
		})

		f := store.Finding{
			Kind:     "staleness",
			Severity: "high",
			Status:   "open",
			NodeRefs: []store.NodeRefJSON{nodeRefToJSON(c.AmendedLawRef), nodeRefToJSON(c.DownstreamRef)},
			Evidence: evidence,
			DedupKey: fmt.Sprintf("stale:%s:%s", nodeRefKey(c.AmendedLawRef), nodeRefKey(c.DownstreamRef)),
		}

		id, err := fc.CreateFinding(ctx, f)
		if err != nil {
			return created, fmt.Errorf("creating staleness finding for %q: %w", c.DownstreamLabel, err)
		}
		if id != uuid.Nil {
			created++
		}
	}
	return created, nil
}
