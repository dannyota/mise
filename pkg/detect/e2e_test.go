package detect

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/store"
	"danny.vn/mise/pkg/vertex"
)

func e2eFinding(t *testing.T) (store.Finding, *fakeCreator) {
	t.Helper()
	ctx := context.Background()
	controlRef := graph.NodeRef{CorpusID: "local-policy", DocumentID: uuid.New()}
	lawRef := graph.NodeRef{CorpusID: "vn-reg", DocumentID: uuid.New()}
	controlText := "The bank shall implement controls for IT risk management"
	lawText := "Điều 7. Quản lý rủi ro công nghệ thông tin"

	judge := vertex.NewFakeJudge()
	grounder := vertex.NewFakeGrounder()
	tc := ThresholdConfig{ConfidenceMin: 0.7, GroundingMin: 0.6}

	jr, err := judge.Judge(ctx, controlText, lawText)
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}
	gr, err := grounder.Ground(ctx, controlText, lawText)
	if err != nil {
		t.Fatalf("Ground: %v", err)
	}
	if !tc.Gate(jr, gr) {
		t.Fatal("threshold gate rejected a fake judge+grounder pair that should pass")
	}

	ev := mustMarshalJSON(map[string]string{
		"edge_type": jr.EdgeType, "rationale": jr.Rationale, "confidence": "0.92",
	})
	f := store.Finding{
		Kind: "conflict", Severity: "critical", Status: "open",
		NodeRefs: []store.NodeRefJSON{nodeRefToJSON(controlRef), nodeRefToJSON(lawRef)},
		Evidence: ev,
		DedupKey: "e2e:" + controlRef.DocumentID.String() + ":" + lawRef.DocumentID.String(),
	}
	fc := newFakeCreator()
	return f, fc
}

func TestE2EPipelineJudgeGroundThresholdWrite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f, fc := e2eFinding(t)

	id1, err := fc.CreateFinding(ctx, f)
	if err != nil {
		t.Fatalf("CreateFinding: %v", err)
	}
	if id1 == uuid.Nil {
		t.Fatal("first CreateFinding returned nil UUID")
	}
	if len(fc.findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(fc.findings))
	}

	got := fc.findings[0]
	if got.Kind != "conflict" {
		t.Errorf("Kind = %q, want conflict", got.Kind)
	}
	if got.Severity != "critical" {
		t.Errorf("Severity = %q, want critical", got.Severity)
	}
	if len(got.NodeRefs) != 2 {
		t.Fatalf("NodeRefs = %d, want 2", len(got.NodeRefs))
	}
	if got.NodeRefs[0].CorpusID != "local-policy" {
		t.Errorf("NodeRefs[0].CorpusID = %q, want local-policy", got.NodeRefs[0].CorpusID)
	}
	if got.NodeRefs[1].CorpusID != "vn-reg" {
		t.Errorf("NodeRefs[1].CorpusID = %q, want vn-reg", got.NodeRefs[1].CorpusID)
	}
}

func TestE2EPipelineDedup(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f, fc := e2eFinding(t)

	if _, err := fc.CreateFinding(ctx, f); err != nil {
		t.Fatalf("first CreateFinding: %v", err)
	}
	id2, err := fc.CreateFinding(ctx, f)
	if err != nil {
		t.Fatalf("second CreateFinding: %v", err)
	}
	if id2 != uuid.Nil {
		t.Errorf("second CreateFinding returned %s, want nil (dedup)", id2)
	}
	if len(fc.findings) != 1 {
		t.Fatalf("findings after dedup = %d, want 1", len(fc.findings))
	}
}

func TestE2EConflictDetectFullChain(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	fc := newFakeCreator()
	deps := ConflictDeps{
		Judge:    contradictionJudge(),
		Grounder: vertex.NewFakeGrounder(),
		Findings: fc,
		Thresholds: ThresholdConfig{
			ConfidenceMin: 0.7,
			GroundingMin:  0.6,
		},
	}

	candidates := []ConflictCandidate{{
		StandardRef:  graph.NodeRef{CorpusID: "group-std", DocumentID: uuid.New()},
		StandardText: "The institution shall review IT risk quarterly.",
		LawRef:       graph.NodeRef{CorpusID: "vn-reg", DocumentID: uuid.New()},
		LawText:      "IT risk reviews must be conducted monthly.",
	}}

	n, err := ConflictDetect(ctx, deps, candidates)
	if err != nil {
		t.Fatalf("ConflictDetect: %v", err)
	}
	if n != 1 {
		t.Fatalf("created = %d, want 1", n)
	}
	if len(fc.findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(fc.findings))
	}

	f := fc.findings[0]
	if f.Kind != "conflict" {
		t.Errorf("Kind = %q, want conflict", f.Kind)
	}
	if f.Severity != "critical" {
		t.Errorf("Severity = %q, want critical", f.Severity)
	}
	var ev map[string]any
	if err := json.Unmarshal(f.Evidence, &ev); err != nil {
		t.Fatalf("unmarshal evidence: %v", err)
	}
	if ev["rationale"] == nil || ev["rationale"] == "" {
		t.Error("evidence missing rationale")
	}

	n2, err := ConflictDetect(ctx, deps, candidates)
	if err != nil {
		t.Fatalf("second ConflictDetect: %v", err)
	}
	if len(fc.findings) != 1 {
		t.Errorf("findings after rerun = %d, want 1 (dedup)", len(fc.findings))
	}
	_ = n2
}

func TestE2ESeverityAutoSet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name     string
		kind     string
		severity string
		scan     func(*testing.T, context.Context, FindingCreator) int
	}{
		{"gap=medium", "gap", "medium", func(t *testing.T, ctx context.Context, fc FindingCreator) int {
			t.Helper()
			n, err := GapScan(ctx, fc, []GapCandidate{{
				Ref:     graph.NodeRef{CorpusID: "vn-reg", DocumentID: uuid.New()},
				Label:   "Test law",
				GapType: "no_satisfies",
			}})
			if err != nil {
				t.Fatalf("GapScan: %v", err)
			}
			return n
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fc := newFakeCreator()
			n := tt.scan(t, ctx, fc)
			if n != 1 {
				t.Fatalf("created = %d, want 1", n)
			}
			if fc.findings[0].Kind != tt.kind {
				t.Errorf("Kind = %q, want %q", fc.findings[0].Kind, tt.kind)
			}
			if fc.findings[0].Severity != tt.severity {
				t.Errorf("Severity = %q, want %q", fc.findings[0].Severity, tt.severity)
			}
		})
	}
}

func TestE2EThresholdGateBoundaries(t *testing.T) {
	t.Parallel()
	tc := ThresholdConfig{ConfidenceMin: 0.7, GroundingMin: 0.6}

	tests := []struct {
		name string
		jr   vertex.JudgeResult
		gr   vertex.GroundResult
		want bool
	}{
		{"both above", vertex.JudgeResult{Confidence: 0.85}, vertex.GroundResult{Score: 0.95}, true},
		{"at boundary", vertex.JudgeResult{Confidence: 0.7}, vertex.GroundResult{Score: 0.6}, true},
		{"confidence below", vertex.JudgeResult{Confidence: 0.69}, vertex.GroundResult{Score: 0.95}, false},
		{"grounding below", vertex.JudgeResult{Confidence: 0.85}, vertex.GroundResult{Score: 0.59}, false},
		{"both below", vertex.JudgeResult{Confidence: 0.3}, vertex.GroundResult{Score: 0.2}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.Gate(tt.jr, tt.gr); got != tt.want {
				t.Errorf("Gate() = %v, want %v", got, tt.want)
			}
		})
	}
}
