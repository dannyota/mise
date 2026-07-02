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

// ErrNodeNotFound is returned by GetNode when ref has no graph.relation_edge
// rows visible to the acting role. Mirrors ErrDocumentNotFound
// (corpus_read.go): "this node has no outgoing edges at all" and "it has
// edges, but every one sits above the caller's tier" are deliberately
// indistinguishable — an RLS-scoped read filters both down to zero visible
// rows (or, defensively, to SQLSTATE 42501, insufficient_privilege, should a
// future migration ever narrow GRANT SELECT itself — see isNotFound), and
// neither case may leak which one happened.
var ErrNodeNotFound = errors.New("graph node not found")

// NodeView is GetNode's read shape: ref's outgoing graph.relation_edge rows,
// already tier-filtered by the SET LOCAL ROLE transaction that read them
// (migrations/010_graph_rls.sql), plus each visible edge's
// graph.relation_evidence rows, keyed by edge id. An edge's target
// (ToRefID/ToCorpusID) travels on graph.Edge itself — resolving the target
// doc_ref's own document/section text is the chain-walk/API layer's job, not
// GetNode's.
type NodeView struct {
	Ref      graph.NodeRef
	Edges    []graph.Edge
	Evidence map[uuid.UUID][]graph.Evidence
}

// GraphRepo is the compliance graph's RLS-scoped read path — the
// tier-filtered counterpart to GraphStore's owner-side writes (graph.go).
// Every read runs inside its own SET LOCAL ROLE transaction, exactly like
// Corpus.GetDocument and Search (corpus_read.go, search.go).
type GraphRepo struct {
	pool *pgxpool.Pool
}

// NewGraphRepo returns a GraphRepo backed by pool.
func NewGraphRepo(pool *pgxpool.Pool) *GraphRepo {
	return &GraphRepo{pool: pool}
}

// relationEdgeSelectCols is every graph.relation_edge column GetNode needs
// to populate a graph.Edge (scanNodeEdges below).
const relationEdgeSelectCols = `id, from_corpus_id, from_document_id, from_section_id, to_ref_id, to_corpus_id,
	edge_type, direction, promoted, access_tier, created_at`

// relationEvidenceSelectCols is every graph.relation_evidence column GetNode
// needs to populate a graph.Evidence (scanEdgeEvidence below).
const relationEvidenceSelectCols = `id, edge_id, evidence_kind, confidence, grounding_score, rationale,
	quoted_from_span, quoted_to_span, run_id, model, prompt_hash, created_by, promoted_by, promoted_at, created_at`

// GetNode reads ref's outgoing graph.relation_edge rows, and each visible
// edge's graph.relation_evidence rows, inside one SET LOCAL ROLE transaction
// scoped to role — so only rows role's RLS policies
// (migrations/010_graph_rls.sql) admit are ever visible. role must come from
// the caller's resolved access tier (resolveRole validates it against
// mise_public/mise_group/mise_local, defaulting "" to mise_public —
// search.go).
//
// ref.SectionID nil matches every edge from ref's document, section-scoped
// or not; non-nil narrows to exactly that section's edges (from_section_id =
// *ref.SectionID) — document-level edges (from_section_id IS NULL) are
// excluded in that case.
//
// A node with zero visible edges — whether it truly has none, or every edge
// exists but sits above role's tier — returns ErrNodeNotFound: the two cases
// are deliberately indistinguishable (see ErrNodeNotFound). Any
// permission-denied cause (SQLSTATE 42501) is preserved in the error chain
// via double-%w, exactly like corpus_read.go's scanDocument.
func (r *GraphRepo) GetNode(ctx context.Context, role string, ref graph.NodeRef) (NodeView, error) {
	validRole, err := resolveRole(role)
	if err != nil {
		return NodeView{}, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return NodeView{}, fmt.Errorf("beginning GetNode read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// SET LOCAL ROLE can't take a query parameter for the role name; role is
	// validated against the fixed set in resolveRole, but it's quoted via
	// pgx.Identifier as defense in depth (mirrors corpus_read.go/search.go).
	roleQ := `SET LOCAL ROLE ` + pgx.Identifier{validRole}.Sanitize()
	if _, err := tx.Exec(ctx, roleQ); err != nil {
		return NodeView{}, fmt.Errorf("setting local role %q: %w", validRole, err)
	}

	edges, err := scanNodeEdges(ctx, tx, ref)
	switch {
	case isNotFound(err):
		return NodeView{}, fmt.Errorf("getting graph node (corpus %s, document %s): %w (%w)",
			ref.CorpusID, ref.DocumentID, ErrNodeNotFound, err)
	case err != nil:
		return NodeView{}, err
	case len(edges) == 0:
		return NodeView{}, fmt.Errorf("getting graph node (corpus %s, document %s): %w",
			ref.CorpusID, ref.DocumentID, ErrNodeNotFound)
	}

	evidence := make(map[uuid.UUID][]graph.Evidence, len(edges))
	for _, e := range edges {
		ev, err := scanEdgeEvidence(ctx, tx, e.ID)
		if err != nil {
			return NodeView{}, err
		}
		evidence[e.ID] = ev
	}

	if err := tx.Commit(ctx); err != nil {
		return NodeView{}, fmt.Errorf("committing GetNode read: %w", err)
	}
	return NodeView{Ref: ref, Edges: edges, Evidence: evidence}, nil
}

// scanNodeEdges reads ref's outgoing graph.relation_edge rows visible to
// tx's current role. from_corpus_id/from_document_id always match ref
// exactly; from_section_id matches ref.SectionID when set, or any section
// (including document-level, from_section_id IS NULL) when ref.SectionID is
// nil — see GetNode.
func scanNodeEdges(ctx context.Context, tx pgx.Tx, ref graph.NodeRef) ([]graph.Edge, error) {
	const q = `
SELECT ` + relationEdgeSelectCols + `
FROM graph.relation_edge
WHERE from_corpus_id = $1 AND from_document_id = $2 AND ($3::uuid IS NULL OR from_section_id = $3::uuid)
ORDER BY created_at, id`

	rows, err := tx.Query(ctx, q, ref.CorpusID, ref.DocumentID, ref.SectionID)
	if err != nil {
		return nil, fmt.Errorf("querying relation_edge for corpus %s document %s: %w", ref.CorpusID, ref.DocumentID, err)
	}
	defer rows.Close()

	var out []graph.Edge
	for rows.Next() {
		var e graph.Edge
		var edgeType, tier string
		err := rows.Scan(&e.ID, &e.From.CorpusID, &e.From.DocumentID, &e.From.SectionID, &e.ToRefID, &e.ToCorpusID,
			&edgeType, &e.Direction, &e.Promoted, &tier, &e.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scanning relation_edge row for corpus %s document %s: %w",
				ref.CorpusID, ref.DocumentID, err)
		}
		e.EdgeType = graph.EdgeType(edgeType)
		e.AccessTier = graph.Tier(tier)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading relation_edge rows for corpus %s document %s: %w",
			ref.CorpusID, ref.DocumentID, err)
	}
	return out, nil
}

// scanEdgeEvidence reads edgeID's graph.relation_evidence rows visible to
// tx's current role, ordered by created_at — an edge carries at most one row
// per evidence_kind (relation_evidence_uq), so up to three overall. Every
// evidence row attached to a visible edge is itself visible: relation_
// evidence's RLS policy is an EXISTS join to the same edge/access_tier pair
// that already let the edge through (migrations/010_graph_rls.sql), so this
// never needs its own not-found fold — an error here is a genuine failure,
// not a permission gate.
func scanEdgeEvidence(ctx context.Context, tx pgx.Tx, edgeID uuid.UUID) ([]graph.Evidence, error) {
	const q = `
SELECT ` + relationEvidenceSelectCols + `
FROM graph.relation_evidence
WHERE edge_id = $1
ORDER BY created_at`

	rows, err := tx.Query(ctx, q, edgeID)
	if err != nil {
		return nil, fmt.Errorf("querying relation_evidence for edge %s: %w", edgeID, err)
	}
	defer rows.Close()

	var out []graph.Evidence
	for rows.Next() {
		var ev graph.Evidence
		var kind string
		err := rows.Scan(&ev.ID, &ev.EdgeID, &kind, &ev.Confidence, &ev.GroundingScore, &ev.Rationale,
			&ev.QuotedFromSpan, &ev.QuotedToSpan, &ev.RunID, &ev.Model, &ev.PromptHash, &ev.CreatedBy,
			&ev.PromotedBy, &ev.PromotedAt, &ev.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scanning relation_evidence row for edge %s: %w", edgeID, err)
		}
		ev.EvidenceKind = graph.EvidenceKind(kind)
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading relation_evidence rows for edge %s: %w", edgeID, err)
	}
	return out, nil
}
