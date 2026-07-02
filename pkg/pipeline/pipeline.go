// Package pipeline is the Temporal law-ingest pipeline: IngestCorpusWorkflow
// discovers newly published documents from a corpus's sources (workflow.go),
// then fans out per-document processing — fetch, extract, quality-gate,
// structure-parse, normalize, embed, index — with bounded parallelism.
// Activities carry the side effects (discover.go, process.go); the ingest
// ledger (pkg/store) makes every stage idempotent under Temporal's
// at-least-once activity execution.
package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/log"

	"danny.vn/mise/pkg/blob"
	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/ingest"
	"danny.vn/mise/pkg/rag/embed"
	"danny.vn/mise/pkg/store"
)

// Ledger lifecycle states written by this pipeline (ingest.doc_ledger.state).
const (
	stateDiscovered = "discovered"
	stateIndexed    = "indexed"
	stateFailed     = "failed"
	stateOutOfScope = "out_of_scope"
)

// ProcessDoc outcome labels, aggregated by IngestCorpusWorkflow.
const (
	outcomeIndexed = "indexed"
	outcomeSkipped = "skipped"
	outcomeFailed  = "failed"
)

// defaultPaceBetweenSources is Deps.PaceBetweenSources' fallback when a
// caller leaves it at the zero value; NewActivities fills it in.
const defaultPaceBetweenSources = 200 * time.Millisecond

// IngestParams selects what one IngestCorpusWorkflow run ingests.
type IngestParams struct {
	// Corpus is the corpus.ID to ingest ("vn-reg" or "my-reg").
	Corpus string
	// Since overrides the stored per-source discovery watermark when non-zero
	// (operator backfill); the zero value uses each source's stored cursor.
	Since time.Time
	// Keyword is the per-source discovery query term; sources that were
	// selected server-side by a keyword bypass the scope matcher (the keyword
	// is the filter). Empty runs each source's default feed with scope matching.
	Keyword string
	// MaxDocs caps how many documents this run enqueues across all sources;
	// 0 means no cap.
	MaxDocs int
}

// IngestResult aggregates one IngestCorpusWorkflow run.
type IngestResult struct {
	Discovered  int // DocRefs enqueued by Discover (in-scope, new or changed)
	Processed   int // ProcessDoc outcome "indexed"
	Skipped     int // ProcessDoc outcome "skipped" (content unchanged)
	Failed      int // ProcessDoc outcome "failed" or activity failure after retries
	Transitions int // validity_status changes ApplyDueEvents applied sweeping due amendment events
}

// DocRef identifies one discovered document for ProcessDoc. ContentHash is the
// discovery fingerprint Discover stored in the ledger for this enqueue —
// sha256(Number|Title|DetailURL|DocType) — used to detect retried work. RunID
// is stamped by the workflow, not Discover (see IngestCorpusWorkflow), with
// the ingest.run id StartRun opened for this run: explicit attribution so
// ProcessDoc always records Document.IngestRunID against the exact run that
// discovered it, instead of the old "most recently started running run for
// this corpus" heuristic (store.CurrentRun), which misattributed whenever two
// runs of one corpus overlapped (e.g. an operator backfill racing a scheduled
// run).
type DocRef struct {
	Corpus      string
	SourceID    string
	ExternalID  string
	DetailURL   string
	ContentHash string
	RunID       string
}

// Deps carries every side-effecting dependency the activities need.
type Deps struct {
	Pool     *pgxpool.Pool
	Blob     blob.Store
	Embedder embed.Embedder
	Extract  *ingest.Extractor
	Sources  map[corpus.ID][]ingest.Source
	// PaceBetweenSources is the politeness delay Discover waits between
	// sources within one corpus run (skipped after the last source). The
	// zero value means "unset" — NewActivities fills in
	// defaultPaceBetweenSources.
	PaceBetweenSources time.Duration
}

// Activities holds the ingest activity implementations. Register one instance
// per worker (temporal.NewWorkerWith); the workflow names activities through a
// typed-nil *Activities.
type Activities struct {
	deps Deps
}

// NewActivities returns Activities backed by d. A zero d.PaceBetweenSources is
// replaced with defaultPaceBetweenSources.
func NewActivities(d Deps) *Activities {
	if d.PaceBetweenSources == 0 {
		d.PaceBetweenSources = defaultPaceBetweenSources
	}
	return &Activities{deps: d}
}

// FinishRunParams closes the ingest.run row the workflow opened via StartRun.
type FinishRunParams struct {
	RunID  string
	Status string
	Result IngestResult
}

// StartRun opens an ingest.run row for corpusID and returns its id. It is a
// workflow-called activity bracketing the run (see IngestCorpusWorkflow).
func (a *Activities) StartRun(ctx context.Context, corpusID string) (string, error) {
	id, err := store.StartRun(ctx, a.deps.Pool, corpus.ID(corpusID))
	if err != nil {
		return "", fmt.Errorf("start run: %w", err)
	}
	return id.String(), nil
}

// FinishRun closes the ingest.run row with the run's status and counters.
func (a *Activities) FinishRun(ctx context.Context, p FinishRunParams) error {
	id, err := uuid.Parse(p.RunID)
	if err != nil {
		return fmt.Errorf("finish run: parsing run id %q: %w", p.RunID, err)
	}
	stats := map[string]any{
		"discovered":  p.Result.Discovered,
		"processed":   p.Result.Processed,
		"skipped":     p.Result.Skipped,
		"failed":      p.Result.Failed,
		"transitions": p.Result.Transitions,
	}
	if err := store.FinishRun(ctx, a.deps.Pool, id, p.Status, stats); err != nil {
		return fmt.Errorf("finish run: %w", err)
	}
	return nil
}

// ApplyDueEvents sweeps p.Corpus's amendment_event rows for every event whose
// event_date has arrived (store.Corpus.DueEvents) and re-drives
// TransitionValidity for each one, returning how many actually changed a
// target's validity_status. It closes the C3 gap: a future-dated event is
// recorded at index time (applyRelations/reapplyIncomingEvents), but nothing
// else ever revisits it once its date arrives — without a sweep, it sits in
// the store applied-in-name-only forever. Calling TransitionValidity for
// every due event, not just ones a pre-check predicts will change something,
// is deliberate: the write is a no-op when the status doesn't change, ingest
// volumes are law-corpus-small, and this keeps the sweep itself a single
// straightforward pass with no separate "would this matter" query.
func (a *Activities) ApplyDueEvents(ctx context.Context, p IngestParams) (int, error) {
	desc, err := descriptor(p.Corpus)
	if err != nil {
		return 0, fmt.Errorf("apply due events: %w", err)
	}
	c, err := store.NewCorpus(a.deps.Pool, desc)
	if err != nil {
		return 0, fmt.Errorf("apply due events: %w", err)
	}

	now := time.Now().UTC()
	due, err := c.DueEvents(ctx, now)
	if err != nil {
		return 0, fmt.Errorf("apply due events: %w", err)
	}

	applied := 0
	for _, ev := range due {
		var before string
		// before is captured from the same locked read TransitionValidity
		// scans under FOR UPDATE (next runs exactly once per call today), so
		// comparing it to the returned status is exact — no extra round trip,
		// no race with the count.
		after, err := c.TransitionValidity(ctx, ev.TargetDocID, func(current string) string {
			before = current
			return ingest.TransitionAt(current, ev.Kind, ev.EventDate, now)
		})
		if err != nil {
			return applied, fmt.Errorf("apply due events: transitioning %s: %w", ev.TargetDocID, err)
		}
		if after != before {
			applied++
		}
	}
	return applied, nil
}

// descriptor resolves a corpus id string to its registered descriptor.
func descriptor(corpusID string) (corpus.Descriptor, error) {
	desc, ok := corpus.Get(corpus.ID(corpusID))
	if !ok {
		return corpus.Descriptor{}, fmt.Errorf("unknown corpus %q", corpusID)
	}
	return desc, nil
}

// heartbeat records activity progress, and is a no-op outside an activity
// context — activity.RecordHeartbeat panics there, and the activity methods
// stay callable as plain functions (e.g. from the e2e harness).
func heartbeat(ctx context.Context, details ...any) {
	if activity.IsActivity(ctx) {
		activity.RecordHeartbeat(ctx, details...)
	}
}

// heartbeatDetailInterval is how often heartbeatLoop's background goroutine
// re-heartbeats while a single slow external call (Download, Doc AI extract,
// one embed batch) is in flight — well inside processHeartbeatTimeout's
// 1-minute budget (workflow.go), so that call alone can't trigger a spurious
// retry no matter how long it runs.
const heartbeatDetailInterval = 20 * time.Second

// heartbeatLoop starts a goroutine that calls heartbeat(ctx, detail) every
// heartbeatDetailInterval until the returned stop func is called. Wrap it
// around one blocking call that can outrun a single heartbeat's worth of
// progress reporting; heartbeat's own activity.IsActivity(ctx) guard makes
// this safe to start even outside an activity context (e.g. process_test.go's
// direct calls into these helpers) — the ticks just become no-ops there.
func heartbeatLoop(ctx context.Context, detail string) (stop func()) {
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(heartbeatDetailInterval)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				heartbeat(ctx, detail)
			}
		}
	}()
	return func() { close(done) }
}

// logger returns the activity logger inside an activity context and a
// slog-backed fallback outside one (activity.GetLogger panics there).
func logger(ctx context.Context) log.Logger {
	if activity.IsActivity(ctx) {
		return activity.GetLogger(ctx)
	}
	return log.NewStructuredLogger(slog.Default())
}
