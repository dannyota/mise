package detect

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/store"
)

// fakeCreator records CreateFinding calls and deduplicates by DedupKey.
type fakeCreator struct {
	seen     map[string]struct{}
	findings []store.Finding
}

func newFakeCreator() *fakeCreator {
	return &fakeCreator{seen: make(map[string]struct{})}
}

func (f *fakeCreator) CreateFinding(_ context.Context, finding store.Finding) (uuid.UUID, error) {
	if _, dup := f.seen[finding.DedupKey]; dup {
		return uuid.Nil, nil // simulate ON CONFLICT DO NOTHING
	}
	f.seen[finding.DedupKey] = struct{}{}
	f.findings = append(f.findings, finding)
	return uuid.New(), nil
}

func TestGapScan_CreatesGapFinding(t *testing.T) {
	t.Parallel()

	fc := newFakeCreator()
	ctx := context.Background()

	candidates := []GapCandidate{{
		Ref:     graph.NodeRef{CorpusID: "vn-reg", DocumentID: uuid.New()},
		Label:   "Circular 09/2020/TT-NHNN",
		GapType: "no_satisfies",
	}}

	n, err := GapScan(ctx, fc, candidates)
	if err != nil {
		t.Fatalf("GapScan() error = %v", err)
	}
	if n != 1 {
		t.Fatalf("GapScan() created = %d, want 1", n)
	}
	if len(fc.findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(fc.findings))
	}

	f := fc.findings[0]
	if f.Kind != "gap" {
		t.Errorf("Kind = %q, want %q", f.Kind, "gap")
	}
	if f.Severity != "medium" {
		t.Errorf("Severity = %q, want %q", f.Severity, "medium")
	}
	if f.Status != "open" {
		t.Errorf("Status = %q, want %q", f.Status, "open")
	}
	if len(f.NodeRefs) != 1 {
		t.Fatalf("NodeRefs len = %d, want 1", len(f.NodeRefs))
	}
	want := nodeRefToJSON(candidates[0].Ref)
	if f.NodeRefs[0] != want {
		t.Errorf("NodeRefs[0] = %v, want %v", f.NodeRefs[0], want)
	}

	var ev map[string]string
	if err := json.Unmarshal(f.Evidence, &ev); err != nil {
		t.Fatalf("unmarshal evidence: %v", err)
	}
	if ev["gap_type"] != "no_satisfies" {
		t.Errorf("evidence gap_type = %q, want %q", ev["gap_type"], "no_satisfies")
	}
	if ev["label"] != "Circular 09/2020/TT-NHNN" {
		t.Errorf("evidence label = %q, want %q", ev["label"], "Circular 09/2020/TT-NHNN")
	}
}

func TestGapScan_Dedup(t *testing.T) {
	t.Parallel()

	fc := newFakeCreator()
	ctx := context.Background()

	ref := graph.NodeRef{CorpusID: "group-std", DocumentID: uuid.New()}
	candidates := []GapCandidate{{
		Ref:     ref,
		Label:   "ISO 27001 A.12",
		GapType: "no_implements",
	}}

	n1, err := GapScan(ctx, fc, candidates)
	if err != nil {
		t.Fatalf("first GapScan() error = %v", err)
	}
	if n1 != 1 {
		t.Fatalf("first GapScan() created = %d, want 1", n1)
	}

	// Second run with the same candidate — should be deduped.
	n2, err := GapScan(ctx, fc, candidates)
	if err != nil {
		t.Fatalf("second GapScan() error = %v", err)
	}
	if n2 != 0 {
		t.Errorf("second GapScan() created = %d, want 0 (dedup)", n2)
	}
	if len(fc.findings) != 1 {
		t.Errorf("total findings = %d, want 1", len(fc.findings))
	}
}

func TestGapScan_AllGapTypes(t *testing.T) {
	t.Parallel()

	fc := newFakeCreator()
	ctx := context.Background()

	candidates := []GapCandidate{
		{Ref: graph.NodeRef{CorpusID: "vn-reg", DocumentID: uuid.New()}, Label: "Law A", GapType: "no_satisfies"},
		{Ref: graph.NodeRef{CorpusID: "group-std", DocumentID: uuid.New()}, Label: "Standard B", GapType: "no_implements"},
		{Ref: graph.NodeRef{CorpusID: "local-policy", DocumentID: uuid.New()}, Label: "Policy C", GapType: "no_sop_coverage"},
	}

	n, err := GapScan(ctx, fc, candidates)
	if err != nil {
		t.Fatalf("GapScan() error = %v", err)
	}
	if n != 3 {
		t.Errorf("GapScan() created = %d, want 3", n)
	}
}

func TestGapScan_EmptyCandidates(t *testing.T) {
	t.Parallel()

	fc := newFakeCreator()
	n, err := GapScan(context.Background(), fc, nil)
	if err != nil {
		t.Fatalf("GapScan() error = %v", err)
	}
	if n != 0 {
		t.Errorf("GapScan() created = %d, want 0", n)
	}
}
