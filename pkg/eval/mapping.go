package eval

import (
	"context"
	"fmt"
)

// MappingCase is one golden mapping example: a cross-corpus edge assertion
// between a standard/control and a local regulatory provision. Expected is
// true for valid mappings, false for pairs that should NOT be mapped.
type MappingCase struct {
	ID         string `json:"id"`
	FromCorpus string `json:"from_corpus"`
	FromDoc    string `json:"from_doc"`
	FromText   string `json:"from_text"`
	ToCorpus   string `json:"to_corpus"`
	ToDoc      string `json:"to_doc"`
	ToText     string `json:"to_text"`
	EdgeType   string `json:"edge_type"`
	Expected   bool   `json:"expected"`
	Notes      string `json:"notes"`
}

// MappingResult is the scored outcome of one MappingCase.
type MappingResult struct {
	Case     MappingCase
	Proposed bool // the system proposed this edge
	Correct  bool // Proposed == Expected
}

// MappingRef identifies one side of a mapping edge.
type MappingRef struct {
	Corpus string
	Doc    string
	Text   string
}

// MappingSearcher checks whether the system would propose a mapping edge.
type MappingSearcher interface {
	ProposesEdge(ctx context.Context, from, to MappingRef) (bool, error)
}

// MappingReport is RunMapping's output: every case's scored MappingResult
// plus the aggregate roll-up.
type MappingReport struct {
	Results   []MappingResult
	Aggregate MappingAggregate
}

// MappingScore evaluates one mapping case against a searcher's proposal.
func MappingScore(c MappingCase, proposed bool) MappingResult {
	return MappingResult{
		Case:     c,
		Proposed: proposed,
		Correct:  proposed == c.Expected,
	}
}

// MappingPrecision computes the fraction of proposed satisfies edges that
// are in the golden set (true positives / all proposed). Proposed edges
// that the golden set marks as negative are false positives.
func MappingPrecision(results []MappingResult) (frac float64, tp, proposed int) {
	for _, r := range results {
		if r.Proposed {
			proposed++
			if r.Case.Expected {
				tp++
			}
		}
	}
	if proposed == 0 {
		return 0, 0, 0
	}
	return float64(tp) / float64(proposed), tp, proposed
}

// MappingRecall computes the fraction of golden-set positive edges that
// were proposed by the system (true positives / all positive golden
// entries).
func MappingRecall(results []MappingResult) (frac float64, tp, positives int) {
	for _, r := range results {
		if r.Case.Expected {
			positives++
			if r.Proposed {
				tp++
			}
		}
	}
	if positives == 0 {
		return 0, 0, 0
	}
	return float64(tp) / float64(positives), tp, positives
}

// RunMapping executes every mapping case against s and returns the scored
// MappingReport. It stops and returns the first ProposesEdge error,
// wrapped with the failing case's id.
func RunMapping(ctx context.Context, s MappingSearcher, cases []MappingCase) (MappingReport, error) {
	results := make([]MappingResult, 0, len(cases))
	for _, c := range cases {
		from := MappingRef{Corpus: c.FromCorpus, Doc: c.FromDoc, Text: c.FromText}
		to := MappingRef{Corpus: c.ToCorpus, Doc: c.ToDoc, Text: c.ToText}
		proposed, err := s.ProposesEdge(ctx, from, to)
		if err != nil {
			return MappingReport{}, fmt.Errorf("eval: mapping case %q: %w", c.ID, err)
		}
		results = append(results, MappingScore(c, proposed))
	}
	return MappingReport{
		Results:   results,
		Aggregate: SummarizeMapping(results),
	}, nil
}
