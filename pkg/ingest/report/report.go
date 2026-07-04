// Package report provides the finding-based source for the reports corpus.
// It adapts stored findings into the ingest.Source interface so the standard
// pipeline can embed finding text alongside regulatory provisions.
package report

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	"danny.vn/mise/pkg/ingest"
	"danny.vn/mise/pkg/store"
)

type reportSource struct {
	findings *store.FindingStore
	role     string
}

// New returns a report Source backed by the given FindingStore.
// role is the RLS role used for finding queries.
func New(findings *store.FindingStore, role string) ingest.Source {
	return &reportSource{findings: findings, role: role}
}

func (r *reportSource) ID() string { return "report" }

// Discover returns findings as DiscoveredDocs. The since watermark and
// keyword are unused — reports rely on the finding store's own pagination.
func (r *reportSource) Discover(ctx context.Context, _ time.Time, _ string) ([]ingest.DiscoveredDoc, error) {
	page, err := r.findings.ListFindings(ctx, r.role, store.FindingListOpts{Limit: 100})
	if err != nil {
		return nil, fmt.Errorf("report source discover: %w", err)
	}
	docs := make([]ingest.DiscoveredDoc, len(page.Items))
	for i, f := range page.Items {
		raw, _ := json.Marshal(f)
		docs[i] = ingest.DiscoveredDoc{
			SourceID:    "report",
			ExternalID:  f.ID.String(),
			Number:      f.DedupKey,
			Title:       fmt.Sprintf("Finding: %s [%s]", f.Kind, f.Severity),
			Abstract:    fmt.Sprintf("%s finding (severity: %s, status: %s)", f.Kind, f.Severity, f.Status),
			DocType:     ingest.DocType(f.Kind),
			IssuedAt:    f.DetectedAt,
			PublishedAt: f.DetectedAt,
			HasContent:  true,
			RawMeta:     raw,
		}
	}
	return docs, nil
}

// FetchDetail returns the finding as a fully populated DiscoveredDoc.
func (r *reportSource) FetchDetail(ctx context.Context, ref ingest.DetailRef) (*ingest.DiscoveredDoc, error) {
	id, err := uuid.Parse(ref.ExternalID)
	if err != nil {
		return nil, fmt.Errorf("report source detail: invalid id %q: %w", ref.ExternalID, err)
	}
	f, err := r.findings.GetFinding(ctx, r.role, id)
	if err != nil {
		return nil, fmt.Errorf("report source detail: %w", err)
	}
	raw, _ := json.Marshal(f)
	abstract := fmt.Sprintf("%s finding (severity: %s, status: %s)", f.Kind, f.Severity, f.Status)
	return &ingest.DiscoveredDoc{
		SourceID:    "report",
		ExternalID:  f.ID.String(),
		Number:      f.DedupKey,
		Title:       fmt.Sprintf("Finding: %s [%s]", f.Kind, f.Severity),
		Abstract:    abstract,
		HTML:        abstract,
		DocType:     ingest.DocType(f.Kind),
		IssuedAt:    f.DetectedAt,
		PublishedAt: f.DetectedAt,
		HasContent:  true,
		RawMeta:     raw,
	}, nil
}

// Download is a no-op for findings — the text content is inline.
func (r *reportSource) Download(_ context.Context, _ ingest.FileRef, _ io.Writer) (int64, string, error) {
	return 0, "", nil
}
