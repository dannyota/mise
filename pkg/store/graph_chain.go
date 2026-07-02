package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"danny.vn/mise/pkg/graph"
)

// MaxChainDepth is the hard cap on a single Chain walk's hop count. Chain
// clamps any requested maxDepth into (0, MaxChainDepth] (clampDepth) so a
// walk can never be coaxed into running unbounded, regardless of caller
// input or how the underlying graph is shaped (including a malformed or
// cyclic one — see walkChain's visited guard).
const MaxChainDepth = 8

// directionUp is graph.relation_edge.direction's control-chain value — the
// authority direction Chain walks (SOP -> Policy -> Group -> law). It is the
// column's own default (migrations/009_graph_tables.sql) and the only value
// WriteExtractedEdge ever writes today (graph.go), but Chain still filters
// on it explicitly rather than assuming every edge returned by GetNode
// qualifies.
const directionUp = "up"

// Hop is one step of a Chain walk: the edge_type that led here, the node it
// arrives at (Ref, CorpusID), a human-readable Citation for that node, and
// the strength of the edge's backing evidence (Promoted, Confidence,
// GroundingScore — see bestEvidence). Text is deliberately left blank; see
// Chain's doc comment for why.
type Hop struct {
	Ref            graph.NodeRef
	EdgeType       string
	CorpusID       string
	Citation       string
	Text           string
	Promoted       bool
	Confidence     float64
	GroundingScore float64
}

// docRefTarget is what resolveDocRef reads off one graph.doc_ref row: just
// enough to build the next hop's NodeRef (CorpusID/DocID/SecID) and its
// Citation (Label, falling back to RefKey) — not the full graph.DocRef shape
// (timestamps, SrcRef) the chain walk never needs. DocID nil means the
// doc_ref is still an unresolved stub (doc_ref_unresolved_idx,
// migrations/009_graph_tables.sql): the citation hasn't been matched to an
// ingested document yet, so there is no NodeRef to walk to.
type docRefTarget struct {
	CorpusID string
	DocID    *uuid.UUID
	SecID    *uuid.UUID
	Label    string
	RefKey   string
}

// chainSource is the per-hop read seam Chain walks over: GetNode's
// tier-filtered edge read, plus the doc_ref lookup an edge's ToRefID needs
// to become the next NodeRef. *GraphRepo satisfies it with real, RLS-scoped
// Postgres reads (GetNode, graph_read.go; resolveDocRef, below);
// graph_chain_test.go's fakeChainSource satisfies it with an in-memory map
// — so walkChain's bound/cycle-guard logic, the whole point of this task,
// is unit-tested with no database at all.
type chainSource interface {
	GetNode(ctx context.Context, role string, ref graph.NodeRef) (NodeView, error)
	resolveDocRef(ctx context.Context, role string, refID uuid.UUID) (docRefTarget, error)
}

// Chain walks graph.relation_edge's "up" direction from start — the
// control-chain's authority direction, e.g. SOP -> Policy -> Group -> law —
// tier-filtering every hop through role's RLS policies exactly like GetNode:
// a hop role can't see (or that simply doesn't exist) ends the walk cleanly,
// not an error. The walk is bounded (maxDepth, clamped into (0,
// MaxChainDepth] by clampDepth) and cycle-guarded (a node already visited on
// this walk stops it rather than re-appending it), so a malformed or cyclic
// graph can never run away — see walkChain for the mechanics.
//
// Citation/Text: Citation comes from the target's own graph.doc_ref row —
// Label when set, else the stable RefKey — since Chain already reads that
// row to resolve the next hop. Text (the target's verbatim quoted body) is
// deliberately left empty: resolving it needs a schema-specific document/
// section lookup (corpus.Descriptor.SchemaName differs per corpus — vn_reg,
// my_reg, group_std, ...), which is heavier than this bounded per-hop walk
// should take on. The API layer already calls Corpus.GetDocument per corpus
// and is better placed to fill Text in for whichever hops a caller actually
// renders.
func (r *GraphRepo) Chain(ctx context.Context, role string, start graph.NodeRef, maxDepth int) ([]Hop, error) {
	validRole, err := resolveRole(role)
	if err != nil {
		return nil, err
	}
	return walkChain(ctx, r, validRole, start, maxDepth)
}

// walkChain is Chain's testable core: an iterative, bounded, cycle-guarded
// walk over src, parameterized on the chainSource seam so unit tests
// (graph_chain_test.go) can exercise it against an in-memory fake with no
// database. Hops are ordered: start's first outgoing up-edge, then that
// edge's target's own first outgoing up-edge, and so on, until a hop role
// can't see, a node with no further up-edge, an unresolved doc_ref stub, or
// a revisited node (cycle) ends it — or maxDepth hops have been appended.
func walkChain(ctx context.Context, src chainSource, role string, start graph.NodeRef, maxDepth int) ([]Hop, error) {
	maxDepth = clampDepth(maxDepth)
	visited := map[string]bool{nodeKey(start): true}
	hops := make([]Hop, 0, maxDepth)

	current := start
	for len(hops) < maxDepth {
		hop, next, ok, err := stepChain(ctx, src, role, current, visited)
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		hops = append(hops, hop)
		visited[nodeKey(next)] = true
		current = next
	}
	return hops, nil
}

// stepChain runs one hop of walkChain's loop: reads current's tier-filtered
// node, picks its first up-edge, and resolves that edge's target. ok is
// false whenever the walk should stop cleanly — current invisible to role,
// no up-edge, an unresolved doc_ref stub, or next already in visited (a
// cycle) — none of those are errors; err is reserved for genuine failures
// (a real GetNode/resolveDocRef error beyond ErrNodeNotFound).
func stepChain(
	ctx context.Context, src chainSource, role string, current graph.NodeRef, visited map[string]bool,
) (hop Hop, next graph.NodeRef, ok bool, err error) {
	view, err := src.GetNode(ctx, role, current)
	if errors.Is(err, ErrNodeNotFound) {
		return Hop{}, graph.NodeRef{}, false, nil
	}
	if err != nil {
		return Hop{}, graph.NodeRef{}, false, fmt.Errorf("walking chain: reading node (corpus %s, document %s): %w",
			current.CorpusID, current.DocumentID, err)
	}

	edge, ok := firstUpEdge(view.Edges)
	if !ok {
		return Hop{}, graph.NodeRef{}, false, nil
	}

	target, err := src.resolveDocRef(ctx, role, edge.ToRefID)
	if errors.Is(err, ErrNodeNotFound) {
		return Hop{}, graph.NodeRef{}, false, nil
	}
	if err != nil {
		return Hop{}, graph.NodeRef{}, false, fmt.Errorf("walking chain: resolving edge %s target: %w", edge.ID, err)
	}
	if target.DocID == nil {
		return Hop{}, graph.NodeRef{}, false, nil
	}

	next = graph.NodeRef{CorpusID: target.CorpusID, DocumentID: *target.DocID, SectionID: target.SecID}
	if visited[nodeKey(next)] {
		return Hop{}, graph.NodeRef{}, false, nil // cycle: next was already visited on this walk
	}
	return buildHop(edge, target, next, view.Evidence[edge.ID]), next, true, nil
}

// firstUpEdge returns the first Direction==directionUp edge in edges —
// GetNode's own query already orders edges by (created_at, id)
// (scanNodeEdges, graph_read.go), so "first" here is deterministic: the
// earliest-created up-edge. A node with more than one up-edge (e.g. a
// policy that both satisfies a law and implements a group standard) still
// yields a single, deterministic Chain path by following only this one; ok
// is false when edges has no up-edge at all (end of chain).
//
// edges is already role's own tier-filtered view (GetNode's RLS read), so
// this "earliest-created" pick is evaluated per caller-role — two roles can
// legitimately walk different single paths when a node's up-edges span more
// than one access tier; that's expected per-role divergence, not a leak.
func firstUpEdge(edges []graph.Edge) (edge graph.Edge, ok bool) {
	for _, e := range edges {
		if e.Direction == directionUp {
			return e, true
		}
	}
	return graph.Edge{}, false
}

// bestEvidence returns ev's highest-Confidence row — a Hop's
// Confidence/GroundingScore, per the task brief, come from "the edge's best
// evidence." The zero value (Confidence 0, GroundingScore 0) results only
// when ev itself is empty: an edge can be written with no evidence row at
// all. Seeding best from ev[0] (rather than a zero graph.Evidence) matters
// when a real row's own Confidence is exactly 0 — a strict ">" comparison
// against a zero-value seed would otherwise never select it, silently
// dropping that row's GroundingScore.
func bestEvidence(ev []graph.Evidence) graph.Evidence {
	if len(ev) == 0 {
		return graph.Evidence{}
	}
	best := ev[0]
	for _, e := range ev[1:] {
		if e.Confidence > best.Confidence {
			best = e
		}
	}
	return best
}

// buildHop assembles one Hop from the edge that was followed, the doc_ref
// target it resolved to, that target's own NodeRef (next), and the edge's
// evidence rows (bestEvidence picks the one Confidence/GroundingScore come
// from). Citation prefers the doc_ref's Label, falling back to its RefKey
// when no label was captured at extraction time.
func buildHop(edge graph.Edge, target docRefTarget, next graph.NodeRef, ev []graph.Evidence) Hop {
	citation := target.Label
	if citation == "" {
		citation = target.RefKey
	}
	best := bestEvidence(ev)
	return Hop{
		Ref:            next,
		EdgeType:       string(edge.EdgeType),
		CorpusID:       target.CorpusID,
		Citation:       citation,
		Promoted:       edge.Promoted,
		Confidence:     best.Confidence,
		GroundingScore: best.GroundingScore,
	}
}

// nodeKey is the chain walk's visited-set key: corpus_id, document_id, and
// section_id (empty string when nil), NUL-joined so the three fields can
// never ambiguously concatenate (mirrors search.go's queryEmbedCacheKey) —
// the key shape the task specifies for the cycle guard.
func nodeKey(ref graph.NodeRef) string {
	sec := ""
	if ref.SectionID != nil {
		sec = ref.SectionID.String()
	}
	return ref.CorpusID + "\x00" + ref.DocumentID.String() + "\x00" + sec
}

// clampDepth folds maxDepth into (0, MaxChainDepth]: non-positive values
// default to the cap (mirrors search.go's defaultTopK pattern — "unset"
// means "use the standard bound", not "walk zero hops"), and any value above
// the cap is pulled back down to it. A caller can shrink a Chain walk but
// never grow it past MaxChainDepth.
func clampDepth(maxDepth int) int {
	if maxDepth <= 0 || maxDepth > MaxChainDepth {
		return MaxChainDepth
	}
	return maxDepth
}

// docRefSelectCols is every graph.doc_ref column resolveDocRef needs to
// populate a docRefTarget.
const docRefSelectCols = `corpus_id, document_id, section_id, label, ref_key`

// resolveDocRef reads refID's graph.doc_ref row under role's RLS policies
// (migrations/010_graph_rls.sql) — the same tier gate GetNode's edges pass
// through. In practice a role that can see an edge can always see the
// doc_ref its to_ref_id names too: access_tier is the STRICTER of the
// edge's two corpora (graph.stricter_tier, migrations/009_graph_tables.sql),
// so its rank is never below the target corpus's own rank alone — the one
// doc_ref's own policy checks. resolveDocRef still runs its own SET LOCAL
// ROLE read rather than trusting that invariant, so a hidden or
// (defensively) nonexistent row folds to ErrNodeNotFound exactly like
// GetNode, and stepChain treats both the same way: end the walk cleanly, not
// an error. Unlike GraphStore.ResolveDocRefs (graph.go — the owner-side,
// no-RLS write path that flips a stub's document_id), this is a read-only,
// RLS-scoped lookup of one already-written row.
func (r *GraphRepo) resolveDocRef(ctx context.Context, role string, refID uuid.UUID) (docRefTarget, error) {
	validRole, err := resolveRole(role)
	if err != nil {
		return docRefTarget{}, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return docRefTarget{}, fmt.Errorf("beginning doc_ref read for %s: %w", refID, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// SET LOCAL ROLE can't take a query parameter for the role name; role is
	// validated against the fixed set in resolveRole, but it's quoted via
	// pgx.Identifier as defense in depth (mirrors GetNode, graph_read.go).
	roleQ := `SET LOCAL ROLE ` + pgx.Identifier{validRole}.Sanitize()
	if _, err := tx.Exec(ctx, roleQ); err != nil {
		return docRefTarget{}, fmt.Errorf("setting local role %q: %w", validRole, err)
	}

	q := `SELECT ` + docRefSelectCols + ` FROM graph.doc_ref WHERE id = $1`
	var t docRefTarget
	err = tx.QueryRow(ctx, q, refID).Scan(&t.CorpusID, &t.DocID, &t.SecID, &t.Label, &t.RefKey)
	switch {
	case isNotFound(err):
		return docRefTarget{}, fmt.Errorf("resolving doc_ref %s: %w (%w)", refID, ErrNodeNotFound, err)
	case err != nil:
		return docRefTarget{}, fmt.Errorf("resolving doc_ref %s: %w", refID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return docRefTarget{}, fmt.Errorf("committing doc_ref read for %s: %w", refID, err)
	}
	return t, nil
}
