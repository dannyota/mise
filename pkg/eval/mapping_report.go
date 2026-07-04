package eval

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// MappingAggregate is the roll-up of a MappingReport's per-case results.
type MappingAggregate struct {
	Cases     int
	Precision float64 // true positives / all proposed
	Recall    float64 // true positives / all golden positives
	TP        int     // true positives (proposed AND expected)
	FP        int     // false positives (proposed but NOT expected)
	FN        int     // false negatives (expected but NOT proposed)
	TN        int     // true negatives (not proposed AND not expected)
}

// SummarizeMapping folds per-case mapping results into aggregate metrics.
func SummarizeMapping(results []MappingResult) MappingAggregate {
	var agg MappingAggregate
	agg.Cases = len(results)
	for _, r := range results {
		switch {
		case r.Proposed && r.Case.Expected:
			agg.TP++
		case r.Proposed && !r.Case.Expected:
			agg.FP++
		case !r.Proposed && r.Case.Expected:
			agg.FN++
		default:
			agg.TN++
		}
	}
	agg.Precision = ratio(agg.TP, agg.TP+agg.FP)
	agg.Recall = ratio(agg.TP, agg.TP+agg.FN)
	return agg
}

// MappingThresholds are the minimum mapping metrics a MappingReport must
// meet. A zero field imposes no floor.
type MappingThresholds struct {
	MinPrecision float64
	MinRecall    float64
}

// Check returns one human-readable message per threshold the aggregate
// did not meet.
func (t MappingThresholds) Check(r MappingReport) []string {
	agg := r.Aggregate
	var fails []string
	proposed := agg.TP + agg.FP
	positives := agg.TP + agg.FN
	if t.MinPrecision > 0 && proposed > 0 && agg.Precision < t.MinPrecision {
		fails = append(fails, fmt.Sprintf(
			"mapping-precision: got %.3f, want >= %.3f", agg.Precision, t.MinPrecision))
	}
	if t.MinRecall > 0 && positives > 0 && agg.Recall < t.MinRecall {
		fails = append(fails, fmt.Sprintf(
			"mapping-recall: got %.3f, want >= %.3f", agg.Recall, t.MinRecall))
	}
	return fails
}

// WriteMappingReport renders a human-readable per-case table plus the
// aggregate summary to w.
func WriteMappingReport(w io.Writer, r MappingReport) {
	_, _ = fmt.Fprintln(w, "ID                    PROPOSED  EXPECTED  CORRECT")
	_, _ = fmt.Fprintln(w, "--------------------  --------  --------  -------")
	for _, mr := range r.Results {
		_, _ = fmt.Fprintf(w, "%-20s  %-8s  %-8s  %s\n",
			truncate(mr.Case.ID, 20),
			boolMark(mr.Proposed),
			boolMark(mr.Case.Expected),
			passFail(mr.Correct),
		)
	}
	agg := r.Aggregate
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "Cases: %d  (TP=%d FP=%d FN=%d TN=%d)\n",
		agg.Cases, agg.TP, agg.FP, agg.FN, agg.TN)
	_, _ = fmt.Fprintf(w, "mapping-precision: %s\n", mappingPct(agg.Precision, agg.TP+agg.FP))
	_, _ = fmt.Fprintf(w, "mapping-recall:    %s\n", mappingPct(agg.Recall, agg.TP+agg.FN))
}

// mappingPct formats a rate as a percentage, or "n/a" when the
// denominator is zero.
func mappingPct(v float64, denom int) string {
	if denom == 0 {
		return "n/a (0 cases)"
	}
	return fmt.Sprintf("%.1f%%", v*100)
}

// LoadMappingGolden reads and validates a mapping golden set from path.
func LoadMappingGolden(path string) ([]MappingCase, error) {
	//nolint:gosec // path is an operator-supplied CLI flag, not request input
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read mapping golden set %s: %w", path, err)
	}
	return parseMappingGolden(b, path)
}

// parseMappingGolden decodes and validates mapping golden JSON; split
// from LoadMappingGolden so tests can validate in-memory bytes. It
// rejects empty sets, missing required fields, duplicate IDs, and sets
// without at least one positive and one negative example.
func parseMappingGolden(b []byte, src string) ([]MappingCase, error) {
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	var cases []MappingCase
	if err := dec.Decode(&cases); err != nil {
		return nil, fmt.Errorf("parse mapping golden set %s: %w", src, err)
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("mapping golden set %s is empty", src)
	}

	seen := make(map[string]bool, len(cases))
	var hasPositive, hasNegative bool
	for i, c := range cases {
		if err := validateMappingCase(src, i, c, seen); err != nil {
			return nil, err
		}
		seen[c.ID] = true
		if c.Expected {
			hasPositive = true
		} else {
			hasNegative = true
		}
	}
	if !hasPositive {
		return nil, fmt.Errorf("mapping golden set %s has no positive examples", src)
	}
	if !hasNegative {
		return nil, fmt.Errorf("mapping golden set %s has no negative examples", src)
	}
	return cases, nil
}

// validateMappingCase applies per-case validation rules.
func validateMappingCase(src string, i int, c MappingCase, seen map[string]bool) error {
	switch {
	case c.ID == "":
		return fmt.Errorf("mapping golden set %s: case %d has no id", src, i)
	case seen[c.ID]:
		return fmt.Errorf("mapping golden set %s: duplicate case id %q", src, c.ID)
	case c.FromCorpus == "":
		return fmt.Errorf("mapping golden set %s: case %q has no from_corpus", src, c.ID)
	case c.FromDoc == "":
		return fmt.Errorf("mapping golden set %s: case %q has no from_doc", src, c.ID)
	case c.ToCorpus == "":
		return fmt.Errorf("mapping golden set %s: case %q has no to_corpus", src, c.ID)
	case c.ToDoc == "":
		return fmt.Errorf("mapping golden set %s: case %q has no to_doc", src, c.ID)
	case c.EdgeType == "":
		return fmt.Errorf("mapping golden set %s: case %q has no edge_type", src, c.ID)
	}
	return nil
}
