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
	Discovered int // DocRefs enqueued by Discover (in-scope, new or changed)
	Processed  int // ProcessDoc outcome "indexed"
	Skipped    int // ProcessDoc outcome "skipped" (content unchanged)
	Failed     int // ProcessDoc outcome "failed" or activity failure after retries
}

// DocRef identifies one discovered document for ProcessDoc. ContentHash is the
// discovery fingerprint Discover stored in the ledger for this enqueue —
// sha256(Number|Title|DetailURL|DocType) — used to detect retried work.
type DocRef struct {
	Corpus      string
	SourceID    string
	ExternalID  string
	DetailURL   string
	ContentHash string
}

// Deps carries every side-effecting dependency the activities need.
type Deps struct {
	Pool     *pgxpool.Pool
	Blob     blob.Store
	Embedder embed.Embedder
	Extract  *ingest.Extractor
	Sources  map[corpus.ID][]ingest.Source
}

// Activities holds the ingest activity implementations. Register one instance
// per worker (temporal.NewWorkerWith); the workflow names activities through a
// typed-nil *Activities.
type Activities struct {
	deps Deps
}

// NewActivities returns Activities backed by d.
func NewActivities(d Deps) *Activities {
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
		"discovered": p.Result.Discovered,
		"processed":  p.Result.Processed,
		"skipped":    p.Result.Skipped,
		"failed":     p.Result.Failed,
	}
	if err := store.FinishRun(ctx, a.deps.Pool, id, p.Status, stats); err != nil {
		return fmt.Errorf("finish run: %w", err)
	}
	return nil
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

// logger returns the activity logger inside an activity context and a
// slog-backed fallback outside one (activity.GetLogger panics there).
func logger(ctx context.Context) log.Logger {
	if activity.IsActivity(ctx) {
		return activity.GetLogger(ctx)
	}
	return log.NewStructuredLogger(slog.Default())
}
