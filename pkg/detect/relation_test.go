package detect_test

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/testsuite"

	"danny.vn/mise/pkg/detect"
	"danny.vn/mise/pkg/vertex"
)

func newTestEnv(t *testing.T) *testsuite.TestWorkflowEnvironment {
	t.Helper()
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(detect.RelationDetectWorkflow)
	return env
}

// TestRelationDetectWorkflowWritesPassingCandidates verifies the end-to-end
// happy path: DiscoverSources returns one section, FindCandidates returns
// one candidate, Judge and Ground return above-threshold results, and
// WriteCandidate is called exactly once.
func TestRelationDetectWorkflowWritesPassingCandidates(t *testing.T) {
	env := newTestEnv(t)
	a := detect.NewActivities(detect.Deps{})

	sections := []detect.SourceSection{
		{CorpusID: "local-policy", DocumentID: "d1", SectionID: "s1", Text: "policy text"},
	}
	candidates := []detect.CandidatePair{
		{FromText: "policy text", ToText: "regulation text", Score: 0.9},
	}
	jr := vertex.JudgeResult{EdgeType: "satisfies", Confidence: 0.9, Rationale: "matches"}
	gr := vertex.GroundResult{Grounded: true, Score: 0.95}

	env.OnActivity(a.DiscoverSources, mock.Anything, mock.Anything).Return(sections, nil).Once()
	env.OnActivity(a.FindCandidates, mock.Anything, sections[0], "vn-reg").Return(candidates, nil).Once()
	env.OnActivity(a.JudgeCandidate, mock.Anything, mock.Anything).Return(jr, nil).Once()
	env.OnActivity(a.GroundCandidate, mock.Anything, mock.Anything).Return(gr, nil).Once()
	env.OnActivity(a.CheckThreshold, mock.Anything, jr, gr).Return(true, nil).Once()
	env.OnActivity(a.WriteCandidate, mock.Anything, mock.Anything).Return(nil).Once()

	env.ExecuteWorkflow(detect.RelationDetectWorkflow, detect.Params{
		Corpus: "vn-reg", RunID: "run-1",
	})

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}
	var res detect.Result
	if err := env.GetWorkflowResult(&res); err != nil {
		t.Fatalf("workflow result: %v", err)
	}
	want := detect.Result{Candidates: 1, Written: 1, Skipped: 0}
	if res != want {
		t.Errorf("DetectResult = %+v, want %+v", res, want)
	}
	env.AssertExpectations(t)
}

// TestRelationDetectWorkflowSkipsBelowThreshold verifies that candidates
// failing the threshold gate are counted as Skipped and WriteCandidate is
// never called.
func TestRelationDetectWorkflowSkipsBelowThreshold(t *testing.T) {
	env := newTestEnv(t)
	a := detect.NewActivities(detect.Deps{})

	sections := []detect.SourceSection{
		{CorpusID: "local-policy", DocumentID: "d1", SectionID: "s1", Text: "weak match"},
	}
	candidates := []detect.CandidatePair{
		{FromText: "weak match", ToText: "regulation text", Score: 0.5},
	}
	jr := vertex.JudgeResult{EdgeType: "satisfies", Confidence: 0.3, Rationale: "weak"}
	gr := vertex.GroundResult{Grounded: false, Score: 0.2}

	env.OnActivity(a.DiscoverSources, mock.Anything, mock.Anything).Return(sections, nil).Once()
	env.OnActivity(a.FindCandidates, mock.Anything, sections[0], "vn-reg").Return(candidates, nil).Once()
	env.OnActivity(a.JudgeCandidate, mock.Anything, mock.Anything).Return(jr, nil).Once()
	env.OnActivity(a.GroundCandidate, mock.Anything, mock.Anything).Return(gr, nil).Once()
	env.OnActivity(a.CheckThreshold, mock.Anything, jr, gr).Return(false, nil).Once()
	// WriteCandidate must NOT be called — the mock's .Times(0) or absence
	// of expectation will fail AssertExpectations if it is.

	env.ExecuteWorkflow(detect.RelationDetectWorkflow, detect.Params{
		Corpus: "vn-reg", RunID: "run-2",
	})

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}
	var res detect.Result
	if err := env.GetWorkflowResult(&res); err != nil {
		t.Fatalf("workflow result: %v", err)
	}
	want := detect.Result{Candidates: 1, Written: 0, Skipped: 1}
	if res != want {
		t.Errorf("DetectResult = %+v, want %+v", res, want)
	}
	env.AssertExpectations(t)
}

// TestRelationDetectWorkflowEmptySections verifies that a corpus with no
// matching source sections completes cleanly with zero counts.
func TestRelationDetectWorkflowEmptySections(t *testing.T) {
	env := newTestEnv(t)
	a := detect.NewActivities(detect.Deps{})

	env.OnActivity(a.DiscoverSources, mock.Anything, mock.Anything).
		Return([]detect.SourceSection{}, nil).Once()

	env.ExecuteWorkflow(detect.RelationDetectWorkflow, detect.Params{
		Corpus: "vn-reg", RunID: "run-3",
	})

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}
	var res detect.Result
	if err := env.GetWorkflowResult(&res); err != nil {
		t.Fatalf("workflow result: %v", err)
	}
	want := detect.Result{}
	if res != want {
		t.Errorf("DetectResult = %+v, want %+v", res, want)
	}
	env.AssertExpectations(t)
}

// TestThresholdGate verifies ThresholdConfig.Gate at boundary values.
func TestThresholdGate(t *testing.T) {
	tc := detect.ThresholdConfig{ConfidenceMin: 0.7, GroundingMin: 0.8}

	tests := []struct {
		name string
		jr   vertex.JudgeResult
		gr   vertex.GroundResult
		want bool
	}{
		{"both above", vertex.JudgeResult{Confidence: 0.9}, vertex.GroundResult{Score: 0.9}, true},
		{"confidence below", vertex.JudgeResult{Confidence: 0.5}, vertex.GroundResult{Score: 0.9}, false},
		{"grounding below", vertex.JudgeResult{Confidence: 0.9}, vertex.GroundResult{Score: 0.5}, false},
		{"both below", vertex.JudgeResult{Confidence: 0.3}, vertex.GroundResult{Score: 0.3}, false},
		{"exact threshold", vertex.JudgeResult{Confidence: 0.7}, vertex.GroundResult{Score: 0.8}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tc.Gate(tt.jr, tt.gr); got != tt.want {
				t.Errorf("Gate() = %v, want %v", got, tt.want)
			}
		})
	}
}
