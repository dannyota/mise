package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"danny.vn/mise/pkg/corpus"
)

// CanvasNode is one document endpoint rendered as a node for the whole-graph
// canvas view (GET /graph).
type CanvasNode struct {
	ID         uuid.UUID
	CorpusID   string
	DocumentID uuid.UUID
	Label      string
	Tier       string
	NodeType   string // corpus kind (law/policy/sop/standard)
}

// CanvasEdge is one graph.relation_edge row rendered for the canvas view,
// including the best evidence's confidence and grounding score.
type CanvasEdge struct {
	ID             uuid.UUID
	Source         uuid.UUID // from_document_id
	Target         uuid.UUID // to_ref_id (the doc_ref)
	EdgeType       string
	Confidence     float64
	GroundingScore float64
	Promoted       bool
}

// CanvasView is GetCanvas's result: nodes, edges, and whether the edge cap
// truncated the result set.
type CanvasView struct {
	Nodes     []CanvasNode
	Edges     []CanvasEdge
	Truncated bool
}

// defaultCanvasLimit is the edge cap when limit <= 0.
const defaultCanvasLimit = 500

// GetCanvas returns a graph canvas view: edges capped at limit (default 500),
// plus the unique nodes (documents) those edges reference. Runs inside a
// SET LOCAL ROLE transaction scoped to the caller's resolved tier.
func (r *GraphRepo) GetCanvas(ctx context.Context, role string, limit int) (CanvasView, error) {
	validRole, err := resolveRole(role)
	if err != nil {
		return CanvasView{}, err
	}

	if limit <= 0 || limit > defaultCanvasLimit {
		limit = defaultCanvasLimit
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CanvasView{}, fmt.Errorf("beginning GetCanvas read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	roleQ := `SET LOCAL ROLE ` + pgx.Identifier{validRole}.Sanitize()
	if _, err := tx.Exec(ctx, roleQ); err != nil {
		return CanvasView{}, fmt.Errorf("setting local role %q: %w", validRole, err)
	}

	edges, truncated, err := scanCanvasEdges(ctx, tx, limit)
	if err != nil {
		return CanvasView{}, err
	}

	nodes, err := resolveCanvasNodes(ctx, tx, edges)
	if err != nil {
		return CanvasView{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return CanvasView{}, fmt.Errorf("committing GetCanvas read: %w", err)
	}
	return CanvasView{Nodes: nodes, Edges: edges, Truncated: truncated}, nil
}

// scanCanvasEdges reads up to limit+1 edges (the +1 detects truncation),
// joining each edge's best evidence for confidence/grounding.
func scanCanvasEdges(ctx context.Context, tx pgx.Tx, limit int) ([]CanvasEdge, bool, error) {
	const q = `
SELECT e.id, e.from_document_id, e.to_ref_id, e.edge_type, e.promoted,
       COALESCE(ev.confidence, 0), COALESCE(ev.grounding_score, 0)
FROM graph.relation_edge e
LEFT JOIN LATERAL (
	SELECT confidence, grounding_score
	FROM graph.relation_evidence
	WHERE edge_id = e.id ORDER BY confidence DESC LIMIT 1
) ev ON true
ORDER BY e.created_at DESC
LIMIT $1`

	rows, err := tx.Query(ctx, q, limit+1)
	if err != nil {
		return nil, false, fmt.Errorf("querying canvas edges: %w", err)
	}
	defer rows.Close()

	var edges []CanvasEdge
	for rows.Next() {
		var e CanvasEdge
		err := rows.Scan(&e.ID, &e.Source, &e.Target,
			&e.EdgeType, &e.Promoted, &e.Confidence, &e.GroundingScore)
		if err != nil {
			return nil, false, fmt.Errorf("scanning canvas edge row: %w", err)
		}
		edges = append(edges, e)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("reading canvas edge rows: %w", err)
	}

	truncated := len(edges) > limit
	if truncated {
		edges = edges[:limit]
	}
	return edges, truncated, nil
}

// resolveCanvasNodes collects unique (corpus, document_id) pairs from edges
// and queries each corpus's document table for titles/tiers.
func resolveCanvasNodes(ctx context.Context, tx pgx.Tx, edges []CanvasEdge) ([]CanvasNode, error) {
	// Collect unique document IDs with their corpus from relation_edge.
	const nodesQ = `
SELECT DISTINCT corpus_id, doc_id FROM (
	SELECT from_corpus_id AS corpus_id, from_document_id AS doc_id FROM graph.relation_edge
	UNION
	SELECT to_corpus_id AS corpus_id, to_ref_id AS doc_id FROM graph.relation_edge
) sub`

	nodeRows, err := tx.Query(ctx, nodesQ)
	if err != nil {
		return nil, fmt.Errorf("querying canvas node pairs: %w", err)
	}
	defer nodeRows.Close()

	type pendingNode struct {
		corpusID string
		docID    uuid.UUID
	}
	var pending []pendingNode
	for nodeRows.Next() {
		var n pendingNode
		if err := nodeRows.Scan(&n.corpusID, &n.docID); err != nil {
			return nil, fmt.Errorf("scanning canvas node pair: %w", err)
		}
		pending = append(pending, n)
	}
	if err := nodeRows.Err(); err != nil {
		return nil, fmt.Errorf("reading canvas node pairs: %w", err)
	}

	// Filter to only docs referenced by the actual edges we returned.
	edgeDocs := make(map[uuid.UUID]bool, len(edges)*2)
	for _, e := range edges {
		edgeDocs[e.Source] = true
		edgeDocs[e.Target] = true
	}

	var nodes []CanvasNode
	for _, p := range pending {
		if !edgeDocs[p.docID] {
			continue
		}

		desc, ok := corpus.Get(corpus.ID(p.corpusID))
		if !ok {
			nodes = append(nodes, CanvasNode{
				ID: p.docID, CorpusID: p.corpusID, DocumentID: p.docID,
			})
			continue
		}

		docTable := pgx.Identifier{desc.SchemaName, "document"}.Sanitize()
		var title, accessTier *string
		err := tx.QueryRow(ctx,
			`SELECT title, access_tier FROM `+docTable+` WHERE id = $1`, p.docID,
		).Scan(&title, &accessTier)
		if err != nil {
			// Document may not exist (dangling doc_ref) — include with empty label.
			nodes = append(nodes, CanvasNode{
				ID: p.docID, CorpusID: p.corpusID, DocumentID: p.docID,
				NodeType: string(desc.Kind),
			})
			continue
		}

		nodes = append(nodes, CanvasNode{
			ID:         p.docID,
			CorpusID:   p.corpusID,
			DocumentID: p.docID,
			Label:      derefOr(title),
			Tier:       derefOr(accessTier),
			NodeType:   string(desc.Kind),
		})
	}
	return nodes, nil
}
