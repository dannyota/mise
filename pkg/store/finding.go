package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"danny.vn/mise/pkg/graph"
)

// Finding is one graph.finding row: a cross-corpus detector result (gap,
// conflict, or staleness) linking zero or more graph nodes (NodeRefs) with
// supporting evidence. AccessTier is trigger-computed by
// graph.set_finding_access_tier (migrations/012_findings.sql) from the
// strictest corpus tier across all NodeRefs — the app never supplies it.
type Finding struct {
	ID         uuid.UUID
	Kind       string
	Severity   string
	Status     string
	NodeRefs   []NodeRefJSON
	Evidence   json.RawMessage
	AccessTier string
	DetectedAt time.Time
	DedupKey   string
}

// NodeRefJSON is the jsonb-serialisable shape of a graph.NodeRef stored in
// graph.finding.node_refs[]. The trigger
// (graph.set_finding_access_tier) parses corpus_id from each element to
// compute the finding's access_tier; FindingsByNode queries the array with
// jsonb containment (@>).
type NodeRefJSON struct {
	CorpusID   string     `json:"corpus_id"`
	DocumentID uuid.UUID  `json:"document_id"`
	SectionID  *uuid.UUID `json:"section_id,omitempty"`
}

// ToNodeRef converts a NodeRefJSON to a graph.NodeRef.
func (n NodeRefJSON) ToNodeRef() graph.NodeRef {
	return graph.NodeRef{
		CorpusID:   n.CorpusID,
		DocumentID: n.DocumentID,
		SectionID:  n.SectionID,
	}
}

// NodeRefJSONFrom builds a NodeRefJSON from a graph.NodeRef.
func NodeRefJSONFrom(ref graph.NodeRef) NodeRefJSON {
	return NodeRefJSON{
		CorpusID:   ref.CorpusID,
		DocumentID: ref.DocumentID,
		SectionID:  ref.SectionID,
	}
}

// Resolution is one graph.finding_resolution row: a disposition and owner
// assignment for a Finding. Status tracks its own lifecycle independently
// of the parent Finding's status.
type Resolution struct {
	ID        uuid.UUID
	FindingID uuid.UUID
	Disposition,
	OwnerDept,
	OwnerRole,
	Status,
	Rationale string
	DueDate *time.Time
}

// FindingOpts controls FindingsByNode's scope and RLS role.
type FindingOpts struct {
	// Role is the RLS role the query runs as — one of mise_public/
	// mise_group/mise_local; empty defaults to mise_public.
	Role string
}

// FindingStore is the findings write and RLS-scoped read path. Writes run
// owner-side (no SET LOCAL ROLE) — RLS is a read concern. Reads run inside
// a SET LOCAL ROLE transaction scoped to the caller's resolved tier.
type FindingStore struct {
	pool *pgxpool.Pool
}

// NewFindingStore returns a FindingStore backed by pool.
func NewFindingStore(pool *pgxpool.Pool) *FindingStore {
	return &FindingStore{pool: pool}
}

// CreateFinding inserts a graph.finding row. Dedup is by dedup_key: ON
// CONFLICT DO NOTHING means a repeat call with the same DedupKey is a
// no-op and returns the existing row's id (looked up in a fallback query).
// AccessTier is trigger-computed — any value in f.AccessTier is ignored.
func (s *FindingStore) CreateFinding(ctx context.Context, f Finding) (uuid.UUID, error) {
	refs, err := json.Marshal(f.NodeRefs)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("marshalling finding node_refs: %w", err)
	}
	evidence := f.Evidence
	if evidence == nil {
		evidence = json.RawMessage(`{}`)
	}

	const q = `
INSERT INTO graph.finding (kind, severity, status, node_refs, evidence, detected_at, dedup_key)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (dedup_key) DO NOTHING
RETURNING id`

	var id uuid.UUID
	err = s.pool.QueryRow(ctx, q,
		f.Kind, f.Severity, f.Status, refs, evidence, f.DetectedAt, f.DedupKey,
	).Scan(&id)

	switch {
	case err == nil:
		return id, nil
	case errors.Is(err, pgx.ErrNoRows):
		return s.findFindingByDedupKey(ctx, f.DedupKey)
	default:
		return uuid.UUID{}, fmt.Errorf("creating finding (dedup_key %s): %w", f.DedupKey, err)
	}
}

// findFindingByDedupKey looks up an existing finding by its unique
// dedup_key — CreateFinding's fallback after ON CONFLICT DO NOTHING
// confirms the row already exists.
func (s *FindingStore) findFindingByDedupKey(ctx context.Context, dedupKey string) (uuid.UUID, error) {
	const q = `SELECT id FROM graph.finding WHERE dedup_key = $1`
	var id uuid.UUID
	if err := s.pool.QueryRow(ctx, q, dedupKey).Scan(&id); err != nil {
		return uuid.UUID{}, fmt.Errorf("finding existing finding by dedup_key %s: %w", dedupKey, err)
	}
	return id, nil
}

// FindingsByNode returns findings whose node_refs jsonb array contains a
// matching node, scoped by RLS to the caller's role. The query uses jsonb
// containment (@>) to find findings referencing the given corpus_id and
// document_id.
func (s *FindingStore) FindingsByNode(
	ctx context.Context, role string, ref graph.NodeRef, opts FindingOpts,
) ([]Finding, error) {
	effectiveRole := opts.Role
	if effectiveRole == "" {
		effectiveRole = role
	}
	validRole, err := resolveRole(effectiveRole)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning FindingsByNode read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	roleQ := `SET LOCAL ROLE ` + pgx.Identifier{validRole}.Sanitize()
	if _, err := tx.Exec(ctx, roleQ); err != nil {
		return nil, fmt.Errorf("setting local role %q: %w", validRole, err)
	}

	// Build the containment match: a jsonb array containing one object with
	// the target corpus_id and document_id.
	match := NodeRefJSON{CorpusID: ref.CorpusID, DocumentID: ref.DocumentID, SectionID: ref.SectionID}
	matchJSON, err := json.Marshal([]NodeRefJSON{match})
	if err != nil {
		return nil, fmt.Errorf("marshalling FindingsByNode match: %w", err)
	}

	const q = `
SELECT id, kind, severity, status, node_refs, evidence, access_tier, detected_at, dedup_key
FROM graph.finding
WHERE node_refs @> $1::jsonb
ORDER BY detected_at DESC, id`

	rows, err := tx.Query(ctx, q, matchJSON)
	if err != nil {
		return nil, fmt.Errorf("querying findings by node: %w", err)
	}
	defer rows.Close()

	var out []Finding
	for rows.Next() {
		var f Finding
		var refsRaw []byte
		err := rows.Scan(&f.ID, &f.Kind, &f.Severity, &f.Status,
			&refsRaw, &f.Evidence, &f.AccessTier, &f.DetectedAt, &f.DedupKey)
		if err != nil {
			return nil, fmt.Errorf("scanning finding row: %w", err)
		}
		if err := json.Unmarshal(refsRaw, &f.NodeRefs); err != nil {
			return nil, fmt.Errorf("unmarshalling finding node_refs: %w", err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading finding rows: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing FindingsByNode read: %w", err)
	}
	return out, nil
}

// CreateResolution inserts a graph.finding_resolution row for findingID and
// returns the new row's id.
func (s *FindingStore) CreateResolution(
	ctx context.Context, findingID uuid.UUID, res Resolution,
) (uuid.UUID, error) {
	const q = `
INSERT INTO graph.finding_resolution
	(finding_id, disposition, owner_department, owner_role, status, rationale, due_date)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id`

	var id uuid.UUID
	err := s.pool.QueryRow(ctx, q,
		findingID, res.Disposition, res.OwnerDept, res.OwnerRole,
		res.Status, res.Rationale, res.DueDate,
	).Scan(&id)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("creating resolution for finding %s: %w", findingID, err)
	}
	return id, nil
}
