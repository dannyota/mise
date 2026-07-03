// Package detect computes derived graph edges — edges whose existence is
// inferred from the existing promoted edge chain rather than extracted from
// document text or classified by a model. The "covers" edge (local-sop
// transitively covers a law node) is the first such derivation (M3-12).
//
// CONSTRAINT (DEC 7): this package must NEVER import danny.vn/mise/pkg/vertex
// or call any model/judge function. The "covers" relation is computed from
// the promoted edge chain alone — no model classification on the SOP→law path.
// covers_test.go enforces this with an AST-level import assertion.
package detect

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/graph"
)

// Hop mirrors store.Hop — the per-step shape a chain walk returns. Defined
// here so the detect package depends on the graph types package, not the store
// package (which carries pgx and would break the depguard intent).
type Hop struct {
	Ref            graph.NodeRef
	EdgeType       string
	CorpusID       string
	Promoted       bool
	Confidence     float64
	GroundingScore float64
}

// ChainWalker walks the M2 control chain from a start node upward. The
// production implementation delegates to store.GraphRepo.Chain; tests supply a
// fake.
type ChainWalker interface {
	Chain(ctx context.Context, role string, start graph.NodeRef, maxDepth int) ([]Hop, error)
}

// EdgeWriter writes one extracted edge. The production implementation
// delegates to store.GraphStore.WriteExtractedEdge.
type EdgeWriter interface {
	WriteExtractedEdge(ctx context.Context, e graph.ExtractedEdge) (uuid.UUID, error)
}

// CoverResult is one covers edge that ComputeCovers produced.
type CoverResult struct {
	From graph.NodeRef
	To   graph.NodeRef
	ID   uuid.UUID
}

// ComputeCovers walks the promoted control chain from each start node
// (typically every local-sop document) and, when the chain reaches a law
// corpus through ONLY promoted edges, writes a covers edge with
// evidence_kind=extracted (transitive, no model call).
//
// starts is the set of (sopNodeRef, attestationOwner) pairs to evaluate —
// the caller is responsible for enumerating local-sop documents and building
// NodeRefs from them. Separating enumeration from computation keeps this
// function testable with no database.
//
// maxDepth caps each chain walk (passed through to ChainWalker.Chain).
func ComputeCovers(
	ctx context.Context,
	walker ChainWalker,
	writer EdgeWriter,
	role string,
	starts []StartNode,
	maxDepth int,
) ([]CoverResult, error) {
	var results []CoverResult
	for _, s := range starts {
		res, err := computeOne(ctx, walker, writer, role, s, maxDepth)
		if err != nil {
			return nil, fmt.Errorf("computing covers for document %s in corpus %s: %w",
				s.Ref.DocumentID, s.Ref.CorpusID, err)
		}
		if res != nil {
			results = append(results, *res)
		}
	}
	return results, nil
}

// StartNode pairs a graph NodeRef with the attestation owner string for edge
// provenance.
type StartNode struct {
	Ref              graph.NodeRef
	AttestationOwner string
}

// computeOne walks one start node's chain and, if it reaches a law corpus via
// all-promoted edges, writes the covers edge. Returns nil (no error) when the
// chain doesn't qualify.
func computeOne(
	ctx context.Context,
	walker ChainWalker,
	writer EdgeWriter,
	role string,
	start StartNode,
	maxDepth int,
) (*CoverResult, error) {
	hops, err := walker.Chain(ctx, role, start.Ref, maxDepth)
	if err != nil {
		return nil, fmt.Errorf("walking chain: %w", err)
	}
	if len(hops) == 0 {
		return nil, nil
	}

	// Every intermediate edge must be promoted.
	for _, h := range hops {
		if !h.Promoted {
			return nil, nil
		}
	}

	// The terminal hop must land in a law corpus.
	terminal := hops[len(hops)-1]
	if !isLawCorpus(terminal.CorpusID) {
		return nil, nil
	}

	edge := graph.ExtractedEdge{
		From:      start.Ref,
		EdgeType:  string(graph.EdgeCovers),
		Direction: "up",
		CreatedBy: start.AttestationOwner,
		Target: graph.ResolvedRef{
			Target:     terminal.Ref,
			ToCorpusID: terminal.CorpusID,
			IsStub:     false,
			RefKey:     terminal.Ref.DocumentID.String(),
			Label:      "",
		},
	}

	edgeID, err := writer.WriteExtractedEdge(ctx, edge)
	if err != nil {
		return nil, fmt.Errorf("writing covers edge: %w", err)
	}

	return &CoverResult{
		From: start.Ref,
		To:   terminal.Ref,
		ID:   edgeID,
	}, nil
}

// isLawCorpus reports whether corpusID names a law corpus — one whose Kind is
// KindLaw in the corpus registry. Law corpora (vn-reg, my-reg) are pure
// targets (CanTarget=true, no SatisfiesTarget) — they sit at the top of the
// control chain.
func isLawCorpus(corpusID string) bool {
	desc, ok := corpus.Get(corpus.ID(corpusID))
	if !ok {
		return false
	}
	return desc.Kind == corpus.KindLaw
}
