package pipeline

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// Activity tuning for one ingest run: every activity gets up to 10 minutes
// per attempt and 4 attempts with 2× backoff; ProcessDoc must additionally
// heartbeat at least once a minute (it heartbeats between stages).
const (
	activityStartToClose    = 10 * time.Minute
	processHeartbeatTimeout = time.Minute
	processWindow           = 4 // in-flight ProcessDoc activities per run
	retryMaximumAttempts    = 4
	retryInitialInterval    = 2 * time.Second
	retryBackoffCoefficient = 2.0
	runStatusCompleted      = "completed"
	runStatusFailed         = "failed"
)

// IngestCorpusWorkflow ingests one corpus: it brackets the run in an
// ingest.run row, discovers new/changed documents once, processes them with
// bounded parallelism (processWindow in flight, workflow.Go + a buffered-
// channel semaphore), and aggregates per-document outcomes. One document
// failing — after its activity retries — counts in Failed but never fails the
// workflow; only StartRun/Discover failures do.
func IngestCorpusWorkflow(ctx workflow.Context, p IngestParams) (IngestResult, error) {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: activityStartToClose,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    retryInitialInterval,
			BackoffCoefficient: retryBackoffCoefficient,
			MaximumAttempts:    retryMaximumAttempts,
		},
	})
	var a *Activities // typed nil: only names the activities for the SDK

	var runID string
	if err := workflow.ExecuteActivity(ctx, a.StartRun, p.Corpus).Get(ctx, &runID); err != nil {
		return IngestResult{}, err
	}

	var refs []DocRef
	if err := workflow.ExecuteActivity(ctx, a.Discover, p).Get(ctx, &refs); err != nil {
		finishRun(ctx, a, runID, runStatusFailed, IngestResult{})
		return IngestResult{}, err
	}
	// Explicit run-id attribution (pure data transform over already-decided
	// workflow state — deterministic, safe on replay): see DocRef.RunID.
	for i := range refs {
		refs[i].RunID = runID
	}

	res := processAll(ctx, a, refs)

	finishRun(ctx, a, runID, runStatusCompleted, res)
	workflow.GetLogger(ctx).Info("ingest run complete", "corpus", p.Corpus,
		"discovered", res.Discovered, "processed", res.Processed, "skipped", res.Skipped, "failed", res.Failed)
	return res, nil
}

// processAll fans ProcessDoc out over refs with at most processWindow
// activities in flight, and aggregates their outcomes. Workflow goroutines are
// cooperatively scheduled, so the shared counters need no locking.
func processAll(ctx workflow.Context, a *Activities, refs []DocRef) IngestResult {
	res := IngestResult{Discovered: len(refs)}

	popts := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: activityStartToClose,
		HeartbeatTimeout:    processHeartbeatTimeout,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    retryInitialInterval,
			BackoffCoefficient: retryBackoffCoefficient,
			MaximumAttempts:    retryMaximumAttempts,
		},
	})

	sem := workflow.NewBufferedChannel(ctx, processWindow)
	wg := workflow.NewWaitGroup(ctx)
	for _, ref := range refs {
		sem.Send(ctx, nil) // blocks while processWindow activities are in flight
		wg.Add(1)
		workflow.Go(ctx, func(gctx workflow.Context) {
			defer wg.Done()
			defer sem.Receive(gctx, nil)

			var outcome string
			err := workflow.ExecuteActivity(popts, a.ProcessDoc, ref).Get(gctx, &outcome)
			switch {
			case err != nil:
				workflow.GetLogger(gctx).Error("process doc failed",
					"source", ref.SourceID, "external_id", ref.ExternalID, "error", err)
				res.Failed++
			case outcome == outcomeSkipped:
				res.Skipped++
			case outcome == outcomeFailed:
				res.Failed++
			default:
				res.Processed++
			}
		})
	}
	wg.Wait(ctx)
	return res
}

// finishRun closes the run row best-effort: a failing FinishRun is logged, not
// propagated — the ingest result itself is already decided.
func finishRun(ctx workflow.Context, a *Activities, runID, status string, res IngestResult) {
	p := FinishRunParams{RunID: runID, Status: status, Result: res}
	if err := workflow.ExecuteActivity(ctx, a.FinishRun, p).Get(ctx, nil); err != nil {
		workflow.GetLogger(ctx).Error("finish run failed", "run_id", runID, "error", err)
	}
}
