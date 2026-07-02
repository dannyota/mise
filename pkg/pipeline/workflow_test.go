package pipeline_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/testsuite"

	"danny.vn/mise/pkg/pipeline"
)

const testRunID = "a2e7f9d0-0000-4000-8000-000000000001"

func newTestEnv(t *testing.T) *testsuite.TestWorkflowEnvironment {
	t.Helper()
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(pipeline.IngestCorpusWorkflow)
	return env
}

// stampRunID returns ref with RunID set to runID — the shape ProcessDoc
// actually receives once the workflow has stamped it (Fix 1: explicit
// run-id attribution replaces the old store.CurrentRun heuristic). Discover
// itself never sets RunID, so every test below mocks Discover returning
// unstamped refs and expects ProcessDoc to be called with the stamped ones.
func stampRunID(ref pipeline.DocRef, runID string) pipeline.DocRef {
	ref.RunID = runID
	return ref
}

// TestIngestCorpusWorkflowAggregatesOutcomes drives the workflow over three
// mocked documents — one indexed, one skipped, one failed — and checks the
// aggregated IngestResult, that the run is finished as completed, and that
// every ProcessDoc call carries the run id the workflow stamped after
// Discover returned.
func TestIngestCorpusWorkflowAggregatesOutcomes(t *testing.T) {
	env := newTestEnv(t)
	a := pipeline.NewActivities(pipeline.Deps{})

	refs := []pipeline.DocRef{
		{Corpus: "vn-reg", SourceID: "vbpl", ExternalID: "1", ContentHash: "h1"},
		{Corpus: "vn-reg", SourceID: "vbpl", ExternalID: "2", ContentHash: "h2"},
		{Corpus: "vn-reg", SourceID: "congbao", ExternalID: "3", ContentHash: "h3"},
	}
	env.OnActivity(a.StartRun, mock.Anything, "vn-reg").Return(testRunID, nil).Once()
	env.OnActivity(a.Discover, mock.Anything, mock.Anything).Return(refs, nil).Once()
	// Each expectation matches only the RunID-stamped ref (not the bare refs[i]
	// Discover returned) — the assertion that the workflow stamps RunID before
	// dispatch: if it didn't, none of these calls would match and
	// env.AssertExpectations(t) below would fail.
	env.OnActivity(a.ProcessDoc, mock.Anything, mock.MatchedBy(func(r pipeline.DocRef) bool {
		return r == stampRunID(refs[0], testRunID)
	})).Return("indexed", nil).Once()
	env.OnActivity(a.ProcessDoc, mock.Anything, mock.MatchedBy(func(r pipeline.DocRef) bool {
		return r == stampRunID(refs[1], testRunID)
	})).Return("skipped", nil).Once()
	env.OnActivity(a.ProcessDoc, mock.Anything, mock.MatchedBy(func(r pipeline.DocRef) bool {
		return r == stampRunID(refs[2], testRunID)
	})).Return("failed", nil).Once()
	env.OnActivity(a.ApplyDueEvents, mock.Anything, pipeline.IngestParams{Corpus: "vn-reg"}).Return(2, nil).Once()
	env.OnActivity(a.FinishRun, mock.Anything, mock.MatchedBy(func(p pipeline.FinishRunParams) bool {
		return p.RunID == testRunID && p.Status == "completed" &&
			p.Result == pipeline.IngestResult{Discovered: 3, Processed: 1, Skipped: 1, Failed: 1, Transitions: 2}
	})).Return(nil).Once()

	env.ExecuteWorkflow(pipeline.IngestCorpusWorkflow, pipeline.IngestParams{Corpus: "vn-reg"})

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}
	var res pipeline.IngestResult
	if err := env.GetWorkflowResult(&res); err != nil {
		t.Fatalf("workflow result: %v", err)
	}
	want := pipeline.IngestResult{Discovered: 3, Processed: 1, Skipped: 1, Failed: 1, Transitions: 2}
	if res != want {
		t.Errorf("IngestResult = %+v, want %+v", res, want)
	}
	env.AssertExpectations(t)
}

// TestIngestCorpusWorkflowToleratesActivityFailure checks that a ProcessDoc
// that keeps erroring (exhausting its retry policy) counts as Failed without
// failing the workflow.
func TestIngestCorpusWorkflowToleratesActivityFailure(t *testing.T) {
	env := newTestEnv(t)
	a := pipeline.NewActivities(pipeline.Deps{})

	refs := []pipeline.DocRef{
		{Corpus: "vn-reg", SourceID: "vbpl", ExternalID: "1", ContentHash: "h1"},
		{Corpus: "vn-reg", SourceID: "vbpl", ExternalID: "2", ContentHash: "h2"},
	}
	env.OnActivity(a.StartRun, mock.Anything, "vn-reg").Return(testRunID, nil).Once()
	env.OnActivity(a.Discover, mock.Anything, mock.Anything).Return(refs, nil).Once()
	env.OnActivity(a.ProcessDoc, mock.Anything, stampRunID(refs[0], testRunID)).Return("indexed", nil).Once()
	env.OnActivity(a.ProcessDoc, mock.Anything, stampRunID(refs[1], testRunID)).
		Return("", errors.New("source down")).Times(4) // retry policy: 4 attempts
	env.OnActivity(a.ApplyDueEvents, mock.Anything, mock.Anything).Return(0, nil).Once()
	env.OnActivity(a.FinishRun, mock.Anything, mock.Anything).Return(nil).Once()

	env.ExecuteWorkflow(pipeline.IngestCorpusWorkflow, pipeline.IngestParams{Corpus: "vn-reg"})

	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v (per-doc failures must not fail the workflow)", err)
	}
	var res pipeline.IngestResult
	if err := env.GetWorkflowResult(&res); err != nil {
		t.Fatalf("workflow result: %v", err)
	}
	want := pipeline.IngestResult{Discovered: 2, Processed: 1, Failed: 1}
	if res != want {
		t.Errorf("IngestResult = %+v, want %+v", res, want)
	}
	env.AssertExpectations(t)
}

// TestIngestCorpusWorkflowToleratesApplyDueEventsFailure checks that a
// persistently failing ApplyDueEvents (the due-event validity sweep, C3) is
// logged, not propagated: the documents this run already discovered and
// processed are durably indexed regardless, and IngestResult.Transitions
// stays 0 rather than failing the whole run.
func TestIngestCorpusWorkflowToleratesApplyDueEventsFailure(t *testing.T) {
	env := newTestEnv(t)
	a := pipeline.NewActivities(pipeline.Deps{})

	refs := []pipeline.DocRef{{Corpus: "vn-reg", SourceID: "vbpl", ExternalID: "1", ContentHash: "h1"}}
	env.OnActivity(a.StartRun, mock.Anything, "vn-reg").Return(testRunID, nil).Once()
	env.OnActivity(a.Discover, mock.Anything, mock.Anything).Return(refs, nil).Once()
	env.OnActivity(a.ProcessDoc, mock.Anything, stampRunID(refs[0], testRunID)).Return("indexed", nil).Once()
	env.OnActivity(a.ApplyDueEvents, mock.Anything, mock.Anything).
		Return(0, errors.New("db unreachable")).Times(4) // retry policy: 4 attempts
	env.OnActivity(a.FinishRun, mock.Anything, mock.MatchedBy(func(p pipeline.FinishRunParams) bool {
		return p.RunID == testRunID && p.Status == "completed" &&
			p.Result == pipeline.IngestResult{Discovered: 1, Processed: 1}
	})).Return(nil).Once()

	env.ExecuteWorkflow(pipeline.IngestCorpusWorkflow, pipeline.IngestParams{Corpus: "vn-reg"})

	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v (ApplyDueEvents failures must not fail the workflow)", err)
	}
	var res pipeline.IngestResult
	if err := env.GetWorkflowResult(&res); err != nil {
		t.Fatalf("workflow result: %v", err)
	}
	want := pipeline.IngestResult{Discovered: 1, Processed: 1}
	if res != want {
		t.Errorf("IngestResult = %+v, want %+v (Transitions stays 0 on sweep failure)", res, want)
	}
	env.AssertExpectations(t)
}

// TestIngestCorpusWorkflowFailsWhenDiscoverFails checks that a Discover
// failure fails the workflow and still closes the run row as failed.
func TestIngestCorpusWorkflowFailsWhenDiscoverFails(t *testing.T) {
	env := newTestEnv(t)
	a := pipeline.NewActivities(pipeline.Deps{})

	env.OnActivity(a.StartRun, mock.Anything, "vn-reg").Return(testRunID, nil).Once()
	env.OnActivity(a.Discover, mock.Anything, mock.Anything).
		Return(nil, errors.New("every source failed")).Times(4)
	env.OnActivity(a.FinishRun, mock.Anything, mock.MatchedBy(func(p pipeline.FinishRunParams) bool {
		return p.RunID == testRunID && p.Status == "failed"
	})).Return(nil).Once()

	env.ExecuteWorkflow(pipeline.IngestCorpusWorkflow, pipeline.IngestParams{Corpus: "vn-reg"})

	if err := env.GetWorkflowError(); err == nil {
		t.Fatal("workflow error = nil, want the Discover failure")
	}
	env.AssertExpectations(t)
}
