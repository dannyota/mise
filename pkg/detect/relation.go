package detect

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/rag/embed"
	"danny.vn/mise/pkg/store"
	"danny.vn/mise/pkg/vertex"
)

// Activity tuning for one detect run.
const (
	detectActivityTimeout = 5 * time.Minute
	detectRetryAttempts   = 4
	detectRetryInterval   = 2 * time.Second
	detectRetryBackoff    = 2.0
	detectWindow          = 4 // max concurrent section goroutines
	candidateTopK         = 10
)

// Params selects what one RelationDetectWorkflow run processes.
type Params struct {
	Corpus string    // the corpus being detected against
	Since  time.Time // only process sections updated since this time
	RunID  string    // unique run identifier for audit trail
}

// Result aggregates one RelationDetectWorkflow run.
type Result struct {
	Candidates int // total candidate pairs found
	Written    int // candidates that passed threshold and were written
	Skipped    int // candidates that failed threshold
}

// Deps carries the side-effecting dependencies the detect activities
// need. Pool is the AlloyDB connection; the Vertex seams (Embedder, Judge,
// Grounder, Ranker) are fake/real per VERTEX env; Graph is the graph
// write-path store; Thresholds gates which candidates become persisted
// edges.
type Deps struct {
	Pool       *pgxpool.Pool
	Embedder   embed.FactEmbedder
	Judge      vertex.Judge
	Grounder   vertex.Grounder
	Ranker     vertex.Ranker
	Graph      *store.GraphStore
	Thresholds ThresholdConfig
}

// Activities holds the detect activity implementations. Register one
// instance per worker; the workflow names activities through a typed-nil
// *Activities.
type Activities struct {
	deps Deps
}

// NewActivities returns Activities backed by d.
func NewActivities(d Deps) *Activities {
	return &Activities{deps: d}
}

// SourceSection is one internal-corpus section that sources relations.
type SourceSection struct {
	CorpusID   string
	DocumentID string
	SectionID  string
	Text       string
}

// CandidateWriteParams carries everything the WriteCandidateEdge activity
// needs to persist one candidate. Fields are serializable for Temporal.
type CandidateWriteParams struct {
	FromCorpusID   string
	FromDocumentID string
	FromSectionID  string // empty if document-level
	ToRefID        string
	ToCorpusID     string
	EdgeType       string
	Direction      string
	Model          string
	PromptHash     string
	Confidence     float64
	GroundingScore float64
	Rationale      string
	QuotedFromSpan string
	QuotedToSpan   string
	RunID          string
	CreatedBy      string
}

// JudgeParams carries the input for the JudgeCandidate activity.
type JudgeParams struct {
	FromText string
	ToText   string
}

// GroundParams carries the input for the GroundCandidate activity.
type GroundParams struct {
	Claim  string
	Source string
}

// RelationDetectWorkflow orchestrates cross-corpus relation detection for
// one corpus: discover source sections in internal corpora whose
// GraphRole.SatisfiesTarget matches the target, find embedding-similar
// candidates for each, classify and ground each candidate, then persist
// those that pass the threshold gate. One section failing counts in
// Skipped but does not fail the workflow.
func RelationDetectWorkflow(ctx workflow.Context, p Params) (Result, error) {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: detectActivityTimeout,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    detectRetryInterval,
			BackoffCoefficient: detectRetryBackoff,
			MaximumAttempts:    detectRetryAttempts,
		},
	})
	var a *Activities // typed nil: only names the activities for the SDK

	var sections []SourceSection
	if err := workflow.ExecuteActivity(ctx, a.DiscoverSources, p).Get(ctx, &sections); err != nil {
		return Result{}, fmt.Errorf("discovering sources: %w", err)
	}

	res := processAllSections(ctx, a, p, sections)

	workflow.GetLogger(ctx).Info("detect run complete",
		"corpus", p.Corpus,
		"candidates", res.Candidates,
		"written", res.Written,
		"skipped", res.Skipped,
	)
	return res, nil
}

// processAllSections fans out section processing with bounded parallelism.
// Workflow goroutines are cooperatively scheduled, so shared counters need
// no locking.
func processAllSections(
	ctx workflow.Context, a *Activities, p Params, sections []SourceSection,
) Result {
	var res Result

	sem := workflow.NewBufferedChannel(ctx, detectWindow)
	wg := workflow.NewWaitGroup(ctx)

	for _, sec := range sections {
		sem.Send(ctx, nil)
		wg.Add(1)
		workflow.Go(ctx, func(gctx workflow.Context) {
			defer wg.Done()
			defer sem.Receive(gctx, nil)

			sr := processSection(gctx, a, p, sec)
			res.Candidates += sr.Candidates
			res.Written += sr.Written
			res.Skipped += sr.Skipped
		})
	}
	wg.Wait(ctx)
	return res
}

// processSection finds candidates for one source section, classifies and
// grounds each, and writes those that pass the threshold.
func processSection(
	ctx workflow.Context, a *Activities, p Params, sec SourceSection,
) Result {
	var res Result

	var candidates []CandidatePair
	if err := workflow.ExecuteActivity(ctx, a.FindCandidates, sec, p.Corpus).Get(ctx, &candidates); err != nil {
		workflow.GetLogger(ctx).Error("find candidates failed",
			"corpus", sec.CorpusID, "document", sec.DocumentID, "error", err)
		return res
	}
	res.Candidates = len(candidates)

	for _, cp := range candidates {
		var jr vertex.JudgeResult
		jp := JudgeParams{FromText: cp.FromText, ToText: cp.ToText}
		if err := workflow.ExecuteActivity(ctx, a.JudgeCandidate, jp).Get(ctx, &jr); err != nil {
			workflow.GetLogger(ctx).Error("judge failed", "error", err)
			res.Skipped++
			continue
		}

		var gr vertex.GroundResult
		gp := GroundParams{Claim: jr.Rationale, Source: cp.ToText}
		if err := workflow.ExecuteActivity(ctx, a.GroundCandidate, gp).Get(ctx, &gr); err != nil {
			workflow.GetLogger(ctx).Error("ground failed", "error", err)
			res.Skipped++
			continue
		}

		var passed bool
		if err := workflow.ExecuteActivity(ctx, a.CheckThreshold, jr, gr).Get(ctx, &passed); err != nil {
			workflow.GetLogger(ctx).Error("threshold check failed", "error", err)
			res.Skipped++
			continue
		}
		if !passed {
			res.Skipped++
			continue
		}

		wp := buildWriteParams(cp, jr, gr, p)
		if err := workflow.ExecuteActivity(ctx, a.WriteCandidate, wp).Get(ctx, nil); err != nil {
			workflow.GetLogger(ctx).Error("write candidate failed", "error", err)
			res.Skipped++
			continue
		}
		res.Written++
	}
	return res
}

// buildWriteParams assembles the CandidateWriteParams from the candidate
// pair, judge result, ground result, and detect params.
func buildWriteParams(
	cp CandidatePair, jr vertex.JudgeResult, gr vertex.GroundResult, p Params,
) CandidateWriteParams {
	wp := CandidateWriteParams{
		FromCorpusID:   cp.FromRef.CorpusID,
		FromDocumentID: cp.FromRef.DocumentID.String(),
		ToRefID:        cp.ToRef.DocumentID.String(),
		ToCorpusID:     string(cp.TargetCorpus),
		EdgeType:       jr.EdgeType,
		Direction:      "up",
		Confidence:     jr.Confidence,
		GroundingScore: gr.Score,
		Rationale:      jr.Rationale,
		QuotedFromSpan: jr.FromSpan,
		QuotedToSpan:   jr.ToSpan,
		RunID:          p.RunID,
		CreatedBy:      "detect/" + p.RunID,
	}
	if cp.FromRef.SectionID != nil {
		wp.FromSectionID = cp.FromRef.SectionID.String()
	}
	return wp
}

// DiscoverSources finds internal-corpus sections whose
// GraphRole.SatisfiesTarget matches the target corpus in p. This activity
// reads the corpus registry (no DB query) and returns stub sections — the
// real section listing will be backed by a DB query in a later task.
func (a *Activities) DiscoverSources(_ context.Context, p Params) ([]SourceSection, error) {
	targetID := corpus.ID(p.Corpus)
	var sections []SourceSection
	for _, desc := range corpus.All() {
		if desc.GraphRole.SatisfiesTarget != targetID {
			continue
		}
		// Stub: a real implementation queries the corpus schema for sections
		// updated since p.Since. For now, return an empty slice per matching
		// corpus so the workflow compiles and the registry wiring is exercised.
		_ = desc
	}
	return sections, nil
}

// FindCandidates wraps the package-level FindCandidates as a Temporal
// activity.
func (a *Activities) FindCandidates(
	ctx context.Context, sec SourceSection, targetCorpus string,
) ([]CandidatePair, error) {
	docID, err := uuid.Parse(sec.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("parsing document_id %q: %w", sec.DocumentID, err)
	}
	var secID *uuid.UUID
	if sec.SectionID != "" {
		parsed, err := uuid.Parse(sec.SectionID)
		if err != nil {
			return nil, fmt.Errorf("parsing section_id %q: %w", sec.SectionID, err)
		}
		secID = &parsed
	}
	from := nodeRef(sec.CorpusID, docID, secID)
	return FindCandidates(ctx, a.deps.Pool, a.deps.Embedder, a.deps.Ranker,
		from, sec.Text, corpus.ID(targetCorpus), candidateTopK)
}

// JudgeCandidate wraps the Judge interface as a Temporal activity.
func (a *Activities) JudgeCandidate(ctx context.Context, p JudgeParams) (vertex.JudgeResult, error) {
	return a.deps.Judge.Judge(ctx, p.FromText, p.ToText)
}

// GroundCandidate wraps the Grounder interface as a Temporal activity.
func (a *Activities) GroundCandidate(ctx context.Context, p GroundParams) (vertex.GroundResult, error) {
	return a.deps.Grounder.Ground(ctx, p.Claim, p.Source)
}

// CheckThreshold applies the threshold gate as an activity, returning
// whether the candidate passes.
func (a *Activities) CheckThreshold(_ context.Context, jr vertex.JudgeResult, gr vertex.GroundResult) (bool, error) {
	return a.deps.Thresholds.Gate(jr, gr), nil
}

// WriteCandidate persists one threshold-passing candidate as a graph edge
// with model_classification evidence.
func (a *Activities) WriteCandidate(ctx context.Context, wp CandidateWriteParams) error {
	docID, err := uuid.Parse(wp.FromDocumentID)
	if err != nil {
		return fmt.Errorf("parsing from_document_id %q: %w", wp.FromDocumentID, err)
	}
	var secID *uuid.UUID
	if wp.FromSectionID != "" {
		parsed, err := uuid.Parse(wp.FromSectionID)
		if err != nil {
			return fmt.Errorf("parsing from_section_id %q: %w", wp.FromSectionID, err)
		}
		secID = &parsed
	}
	toRefID, err := uuid.Parse(wp.ToRefID)
	if err != nil {
		return fmt.Errorf("parsing to_ref_id %q: %w", wp.ToRefID, err)
	}

	edge := store.CandidateEdgeParams{
		FromCorpusID:   wp.FromCorpusID,
		FromDocumentID: docID,
		FromSectionID:  secID,
		ToRefID:        toRefID,
		ToCorpusID:     wp.ToCorpusID,
		EdgeType:       wp.EdgeType,
		Direction:      wp.Direction,
		Model:          wp.Model,
		PromptHash:     wp.PromptHash,
		Confidence:     wp.Confidence,
		GroundingScore: wp.GroundingScore,
		Rationale:      wp.Rationale,
		QuotedFromSpan: wp.QuotedFromSpan,
		QuotedToSpan:   wp.QuotedToSpan,
		RunID:          wp.RunID,
		CreatedBy:      wp.CreatedBy,
	}
	if _, err := a.deps.Graph.WriteCandidateEdge(ctx, edge); err != nil {
		return fmt.Errorf("writing candidate edge: %w", err)
	}
	return nil
}

// nodeRef is a convenience constructor for graph.NodeRef.
func nodeRef(corpusID string, docID uuid.UUID, secID *uuid.UUID) graph.NodeRef {
	return graph.NodeRef{CorpusID: corpusID, DocumentID: docID, SectionID: secID}
}
