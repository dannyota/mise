// Package detect implements M3 detection pipelines: Method-B candidate
// retrieval (embed → ANN → rerank) that feeds the Gemini judge.
package detect

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	pgxvec "github.com/pgvector/pgvector-go/pgx"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/graph"
	"danny.vn/mise/pkg/rag/embed"
	"danny.vn/mise/pkg/vertex"
)

// CandidatePair is one ranked match between a control section (FromRef) and a
// law section (ToRef) in the target corpus, produced by the embed → ANN →
// rerank pipeline. Score is the reranker's relevance score.
type CandidatePair struct {
	FromRef      graph.NodeRef
	FromText     string
	ToRef        graph.NodeRef
	ToText       string
	TargetCorpus corpus.ID
	Score        float64
}

// annRow holds the columns scanned from the ANN-only retrieval query.
type annRow struct {
	id           uuid.UUID
	documentID   uuid.UUID
	corpusID     string
	citationPath string
	headingPath  string
	body         string
}

// FindCandidates embeds fromText with FACT_VERIFICATION, ANN-searches the
// target law corpus for the top-N nearest sections, reranks them, and returns
// the top-k as candidate pairs. The target corpus is determined from the
// source corpus's GraphRole.SatisfiesTarget.
func FindCandidates(
	ctx context.Context,
	pool *pgxpool.Pool,
	emb embed.FactEmbedder,
	ranker vertex.Ranker,
	from graph.NodeRef,
	fromText string,
	targetCorpus corpus.ID,
	topK int,
) ([]CandidatePair, error) {
	if topK <= 0 {
		return nil, fmt.Errorf("detect: topK must be positive, got %d", topK)
	}

	desc, ok := corpus.Get(targetCorpus)
	if !ok {
		return nil, fmt.Errorf("detect: %q is not a registered corpus", targetCorpus)
	}

	// Step 1: embed fromText with FACT_VERIFICATION task type.
	vecs, err := emb.EmbedFact(ctx, []string{fromText})
	if err != nil {
		return nil, fmt.Errorf("detect: embedding from-text: %w", err)
	}
	if len(vecs) != 1 {
		return nil, fmt.Errorf("detect: embedding returned %d vectors, want 1", len(vecs))
	}
	qvec := vecs[0]

	// Step 2: ANN-only search against the target corpus.
	annTopN := topK * 3 // fetch more for the reranker
	rows, err := annSearch(ctx, pool, desc, qvec, annTopN)
	if err != nil {
		return nil, fmt.Errorf("detect: ANN search on %s: %w", targetCorpus, err)
	}
	if len(rows) == 0 {
		return nil, nil
	}

	// Step 3: rerank.
	bodies := make([]string, len(rows))
	for i, r := range rows {
		bodies[i] = r.body
	}
	ranked, err := ranker.Rerank(ctx, fromText, bodies, topK)
	if err != nil {
		return nil, fmt.Errorf("detect: reranking candidates: %w", err)
	}

	// Step 4: build CandidatePairs from reranked results.
	pairs := make([]CandidatePair, len(ranked))
	for i, rd := range ranked {
		r := rows[rd.Index]
		sid := r.id
		pairs[i] = CandidatePair{
			FromRef:      from,
			FromText:     fromText,
			ToRef:        graph.NodeRef{CorpusID: r.corpusID, DocumentID: r.documentID, SectionID: &sid},
			ToText:       r.body,
			TargetCorpus: targetCorpus,
			Score:        rd.Score,
		}
	}
	return pairs, nil
}

// annSearch runs a pure cosine-distance ANN query against the target corpus's
// section table, returning the topN nearest in-force sections. The query runs
// inside a SET LOCAL ROLE mise_public transaction (law corpora are public).
func annSearch(
	ctx context.Context,
	pool *pgxpool.Pool,
	desc corpus.Descriptor,
	qvec []float32,
	topN int,
) ([]annRow, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning ANN tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := pgxvec.RegisterTypes(ctx, tx.Conn()); err != nil {
		return nil, fmt.Errorf("registering pgvector types: %w", err)
	}

	roleQ := "SET LOCAL ROLE " + pgx.Identifier{"mise_public"}.Sanitize()
	if _, err := tx.Exec(ctx, roleQ); err != nil {
		return nil, fmt.Errorf("setting local role: %w", err)
	}

	q := BuildANNSQL(desc.SchemaName)
	pgRows, err := tx.Query(ctx, q, pgvector.NewVector(qvec), topN)
	if err != nil {
		return nil, fmt.Errorf("executing ANN query: %w", err)
	}
	defer pgRows.Close()

	var results []annRow
	for pgRows.Next() {
		var r annRow
		var citationPath, headingPath *string
		if err := pgRows.Scan(
			&r.id, &r.documentID, &r.corpusID,
			&citationPath, &headingPath, &r.body,
		); err != nil {
			return nil, fmt.Errorf("scanning ANN row: %w", err)
		}
		if citationPath != nil {
			r.citationPath = *citationPath
		}
		if headingPath != nil {
			r.headingPath = *headingPath
		}
		results = append(results, r)
	}
	if err := pgRows.Err(); err != nil {
		return nil, fmt.Errorf("reading ANN rows: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing ANN tx: %w", err)
	}
	return results, nil
}

// BuildANNSQL renders the ANN-only retrieval query for the given schema.
// Positional params: $1 the query vector, $2 the topN limit. Only sections
// with a non-null embedding and in-force/amended validity are considered.
// Schema is always from corpus.Get — never raw input.
func BuildANNSQL(schema string) string {
	section := pgx.Identifier{schema, "section"}.Sanitize()
	return `SELECT s.id, s.document_id, s.corpus_id, s.citation_path, s.heading_path, s.body
FROM ` + section + ` s
WHERE s.embedding IS NOT NULL
  AND s.validity_status IN ('in_force','amended')
ORDER BY s.embedding <=> $1::vector
LIMIT $2`
}
