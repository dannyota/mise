package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"danny.vn/mise/pkg/graph"
)

// ErrEdgeNotFound is returned when a relation_edge id does not exist or is
// not visible to the acting role.
var ErrEdgeNotFound = errors.New("edge not found")

// ReviewItem is one unpromoted relation_edge row eligible for human review,
// plus its best evidence's confidence and grounding score.
type ReviewItem struct {
	Edge       graph.Edge
	Confidence float64
	Grounding  float64
}

// ReviewListOpts controls ListReviewQueue's pagination and filters.
type ReviewListOpts struct {
	Cursor string
	Limit  int
	Status string
	Sort   string
}

// ReviewPage is the paginated result of ListReviewQueue.
type ReviewPage struct {
	Items      []ReviewItem
	NextCursor string
}

// ReviewStore is the review-queue read/write path (review endpoints +
// promote/reject/relink). Reads are RLS-scoped; writes run owner-side.
type ReviewStore struct {
	pool *pgxpool.Pool
}

// NewReviewStore returns a ReviewStore backed by pool.
func NewReviewStore(pool *pgxpool.Pool) *ReviewStore {
	return &ReviewStore{pool: pool}
}

// ListReviewQueue returns unpromoted edges (review candidates) visible to
// role, paginated by cursor/limit.
func (s *ReviewStore) ListReviewQueue(
	ctx context.Context, role string, opts ReviewListOpts,
) (ReviewPage, error) {
	validRole, err := resolveRole(role)
	if err != nil {
		return ReviewPage{}, err
	}

	limit := opts.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ReviewPage{}, fmt.Errorf("beginning ListReviewQueue read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	roleQ := `SET LOCAL ROLE ` + pgx.Identifier{validRole}.Sanitize()
	if _, err := tx.Exec(ctx, roleQ); err != nil {
		return ReviewPage{}, fmt.Errorf("setting local role %q: %w", validRole, err)
	}

	q, args := buildReviewQuery(opts, limit)

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return ReviewPage{}, fmt.Errorf("querying review queue: %w", err)
	}
	defer rows.Close()

	items, err := scanReviewRows(rows)
	if err != nil {
		return ReviewPage{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return ReviewPage{}, fmt.Errorf("committing ListReviewQueue read: %w", err)
	}

	var nextCursor string
	if len(items) > limit {
		nextCursor = items[limit-1].Edge.ID.String()
		items = items[:limit]
	}
	return ReviewPage{Items: items, NextCursor: nextCursor}, nil
}

func buildReviewQuery(opts ReviewListOpts, limit int) (string, []any) {
	orderCol := "e.created_at"
	if opts.Sort == "confidence" {
		orderCol = "COALESCE(ev.confidence, 0) DESC, e.created_at"
	}

	cursorFilter := ""
	args := []any{limit + 1}
	argIdx := 2
	if opts.Cursor != "" {
		cursorID, parseErr := uuid.Parse(opts.Cursor)
		if parseErr == nil {
			cursorFilter = fmt.Sprintf(" AND e.id > $%d", argIdx)
			args = append(args, cursorID)
			argIdx++
		}
	}

	statusFilter := ""
	if opts.Status != "" {
		statusFilter = fmt.Sprintf(" AND e.edge_type = $%d", argIdx)
		args = append(args, opts.Status)
	}

	q := `SELECT ` + relationEdgeSelectCols + `,
		COALESCE(ev.confidence, 0), COALESCE(ev.grounding_score, 0)
	FROM graph.relation_edge e
	LEFT JOIN LATERAL (
		SELECT confidence, grounding_score
		FROM graph.relation_evidence
		WHERE edge_id = e.id ORDER BY confidence DESC LIMIT 1
	) ev ON true
	WHERE e.promoted = false` + statusFilter + cursorFilter +
		` ORDER BY ` + orderCol + `, e.id LIMIT $1`

	return q, args
}

func scanReviewRows(rows pgx.Rows) ([]ReviewItem, error) {
	var items []ReviewItem
	for rows.Next() {
		var e graph.Edge
		var edgeType, tier string
		var conf, grnd float64
		err := rows.Scan(
			&e.ID, &e.From.CorpusID, &e.From.DocumentID,
			&e.From.SectionID, &e.ToRefID, &e.ToCorpusID,
			&edgeType, &e.Direction, &e.Promoted, &tier,
			&e.CreatedAt, &conf, &grnd,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning review item: %w", err)
		}
		e.EdgeType = graph.EdgeType(edgeType)
		e.AccessTier = graph.Tier(tier)
		items = append(items, ReviewItem{
			Edge: e, Confidence: conf, Grounding: grnd,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading review rows: %w", err)
	}
	return items, nil
}

// PromoteEdge sets edgeID's promoted=true and inserts a human_attested
// evidence row with promotedBy as the attestation owner.
func (s *ReviewStore) PromoteEdge(ctx context.Context, edgeID uuid.UUID, promotedBy string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning PromoteEdge: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx,
		`UPDATE graph.relation_edge SET promoted = true WHERE id = $1`, edgeID)
	if err != nil {
		return fmt.Errorf("promoting edge %s: %w", edgeID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("promoting edge %s: %w", edgeID, ErrEdgeNotFound)
	}

	const evQ = `
INSERT INTO graph.relation_evidence
	(edge_id, evidence_kind, confidence, promoted_by, promoted_at)
VALUES ($1, 'human_attested', 1.0, $2, now())
ON CONFLICT (edge_id, evidence_kind) DO NOTHING`

	if _, err := tx.Exec(ctx, evQ, edgeID, promotedBy); err != nil {
		return fmt.Errorf("inserting human_attested evidence for edge %s: %w", edgeID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing PromoteEdge: %w", err)
	}
	return nil
}

// RejectEdge marks edgeID as rejected by deleting it. Returns
// ErrEdgeNotFound if the edge does not exist.
func (s *ReviewStore) RejectEdge(ctx context.Context, edgeID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM graph.relation_edge WHERE id = $1`, edgeID)
	if err != nil {
		return fmt.Errorf("rejecting edge %s: %w", edgeID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("rejecting edge %s: %w", edgeID, ErrEdgeNotFound)
	}
	return nil
}

// RelinkEdge updates edgeID's to_ref_id to newTarget. This is a structural
// update only — re-triggering detection against the new target is a TODO
// for M5.
func (s *ReviewStore) RelinkEdge(ctx context.Context, edgeID, newTarget uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE graph.relation_edge SET to_ref_id = $2 WHERE id = $1`, edgeID, newTarget)
	if err != nil {
		return fmt.Errorf("relinking edge %s: %w", edgeID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("relinking edge %s: %w", edgeID, ErrEdgeNotFound)
	}
	return nil
}
