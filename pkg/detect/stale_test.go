package detect

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"danny.vn/mise/pkg/graph"
)

func TestStaleScan_AmendmentAfterEffective(t *testing.T) {
	t.Parallel()

	fc := newFakeCreator()
	ctx := context.Background()

	amendment := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	effective := time.Date(2023, 1, 15, 0, 0, 0, 0, time.UTC)

	candidates := []StaleCandidate{{
		AmendedLawRef:   graph.NodeRef{CorpusID: "vn-reg", DocumentID: uuid.New()},
		AmendmentDate:   amendment,
		DownstreamRef:   graph.NodeRef{CorpusID: "local-policy", DocumentID: uuid.New()},
		DownstreamLabel: "IT Security Policy v2",
		EffectiveDate:   effective,
	}}

	n, err := StaleScan(ctx, fc, candidates)
	if err != nil {
		t.Fatalf("StaleScan() error = %v", err)
	}
	if n != 1 {
		t.Fatalf("StaleScan() created = %d, want 1", n)
	}
	if len(fc.findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(fc.findings))
	}

	f := fc.findings[0]
	if f.Kind != "staleness" {
		t.Errorf("Kind = %q, want %q", f.Kind, "staleness")
	}
	if f.Severity != "high" {
		t.Errorf("Severity = %q, want %q", f.Severity, "high")
	}
	if f.Status != "open" {
		t.Errorf("Status = %q, want %q", f.Status, "open")
	}
	if len(f.NodeRefs) != 2 {
		t.Fatalf("NodeRefs len = %d, want 2", len(f.NodeRefs))
	}

	var ev map[string]string
	if err := json.Unmarshal(f.Evidence, &ev); err != nil {
		t.Fatalf("unmarshal evidence: %v", err)
	}
	if ev["amendment_date"] != "2024-06-01" {
		t.Errorf("evidence amendment_date = %q, want %q", ev["amendment_date"], "2024-06-01")
	}
	if ev["effective_date"] != "2023-01-15" {
		t.Errorf("evidence effective_date = %q, want %q", ev["effective_date"], "2023-01-15")
	}
	if ev["downstream_label"] != "IT Security Policy v2" {
		t.Errorf("evidence downstream_label = %q, want %q", ev["downstream_label"], "IT Security Policy v2")
	}
}

func TestStaleScan_AmendmentBeforeEffective(t *testing.T) {
	t.Parallel()

	fc := newFakeCreator()
	ctx := context.Background()

	amendment := time.Date(2022, 3, 1, 0, 0, 0, 0, time.UTC)
	effective := time.Date(2023, 1, 15, 0, 0, 0, 0, time.UTC)

	candidates := []StaleCandidate{{
		AmendedLawRef:   graph.NodeRef{CorpusID: "vn-reg", DocumentID: uuid.New()},
		AmendmentDate:   amendment,
		DownstreamRef:   graph.NodeRef{CorpusID: "local-policy", DocumentID: uuid.New()},
		DownstreamLabel: "Up-to-date Policy",
		EffectiveDate:   effective,
	}}

	n, err := StaleScan(ctx, fc, candidates)
	if err != nil {
		t.Fatalf("StaleScan() error = %v", err)
	}
	if n != 0 {
		t.Errorf("StaleScan() created = %d, want 0 (not stale)", n)
	}
	if len(fc.findings) != 0 {
		t.Errorf("findings = %d, want 0", len(fc.findings))
	}
}

func TestStaleScan_AmendmentEqualEffective(t *testing.T) {
	t.Parallel()

	fc := newFakeCreator()
	ctx := context.Background()

	same := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	candidates := []StaleCandidate{{
		AmendedLawRef:   graph.NodeRef{CorpusID: "vn-reg", DocumentID: uuid.New()},
		AmendmentDate:   same,
		DownstreamRef:   graph.NodeRef{CorpusID: "local-policy", DocumentID: uuid.New()},
		DownstreamLabel: "Same-day Policy",
		EffectiveDate:   same,
	}}

	n, err := StaleScan(ctx, fc, candidates)
	if err != nil {
		t.Fatalf("StaleScan() error = %v", err)
	}
	if n != 0 {
		t.Errorf("StaleScan() created = %d, want 0 (equal dates = not stale)", n)
	}
}

func TestStaleScan_Dedup(t *testing.T) {
	t.Parallel()

	fc := newFakeCreator()
	ctx := context.Background()

	lawRef := graph.NodeRef{CorpusID: "vn-reg", DocumentID: uuid.New()}
	downRef := graph.NodeRef{CorpusID: "local-policy", DocumentID: uuid.New()}

	candidates := []StaleCandidate{{
		AmendedLawRef:   lawRef,
		AmendmentDate:   time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		DownstreamRef:   downRef,
		DownstreamLabel: "Policy X",
		EffectiveDate:   time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
	}}

	n1, err := StaleScan(ctx, fc, candidates)
	if err != nil {
		t.Fatalf("first StaleScan() error = %v", err)
	}
	if n1 != 1 {
		t.Fatalf("first StaleScan() created = %d, want 1", n1)
	}

	n2, err := StaleScan(ctx, fc, candidates)
	if err != nil {
		t.Fatalf("second StaleScan() error = %v", err)
	}
	if n2 != 0 {
		t.Errorf("second StaleScan() created = %d, want 0 (dedup)", n2)
	}
	if len(fc.findings) != 1 {
		t.Errorf("total findings = %d, want 1", len(fc.findings))
	}
}

func TestStaleScan_EmptyCandidates(t *testing.T) {
	t.Parallel()

	fc := newFakeCreator()
	n, err := StaleScan(context.Background(), fc, nil)
	if err != nil {
		t.Fatalf("StaleScan() error = %v", err)
	}
	if n != 0 {
		t.Errorf("StaleScan() created = %d, want 0", n)
	}
}
