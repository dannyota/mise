package eval

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Aggregate is the corpus-level roll-up of a Report's per-case CaseResults.
// Each rate is a micro-average (sum of numerators / sum of denominators
// across cases) so larger cases are not over- or under-weighted relative to
// a mean-of-means. A case with no denominator for a metric (e.g. recall on
// an out-of-scope question) is excluded from that metric's average, not
// counted as zero.
type Aggregate struct {
	Cases int // number of scored cases

	RecallAtK   float64 // Σ found / Σ want, over cases that expected citations
	RecallCases int     // cases that contributed to recall (had expected citations)

	MRRAtK   float64 // mean reciprocal rank over cases that expected citations
	MRRCases int     // cases that contributed to MRR (had expected citations)

	CitationCorrectness float64 // Σ grounded / Σ made, over cases that returned hits
	CitationCases       int     // cases that contributed (returned >=1 hit)

	InForcePrecision float64 // Σ current-law hits / Σ hits, over cases that returned hits
	InForceCases     int     // cases that contributed (returned >=1 hit)

	AbstainAccuracy float64 // fraction of cases whose abstention matched expectation
}

// Summarize folds per-case results into corpus metrics. It micro-averages
// each rate over the cases that have a denominator for it, so an empty input
// is well defined (every rate and count 0).
func Summarize(results []CaseResult) Aggregate {
	var agg Aggregate
	agg.Cases = len(results)

	var recallFound, recallWant int
	var citGrounded, citMade int
	var inForceOK, inForceTotal int
	var abstainOK int

	for _, r := range results {
		if r.RecallWant > 0 {
			recallFound += r.RecallHits
			recallWant += r.RecallWant
			agg.MRRAtK += r.MRRAtK
			agg.RecallCases++
			agg.MRRCases++
		}
		if r.CitationsMade > 0 {
			citGrounded += r.CitationsGrounded
			citMade += r.CitationsMade
			agg.CitationCases++
		}
		if r.HitsTotal > 0 {
			inForceOK += r.HitsInForce
			inForceTotal += r.HitsTotal
			agg.InForceCases++
		}
		if r.AbstainCorrect {
			abstainOK++
		}
	}

	agg.RecallAtK = ratio(recallFound, recallWant)
	if agg.MRRCases > 0 {
		agg.MRRAtK /= float64(agg.MRRCases)
	}
	agg.CitationCorrectness = ratio(citGrounded, citMade)
	agg.InForcePrecision = ratio(inForceOK, inForceTotal)
	agg.AbstainAccuracy = ratio(abstainOK, len(results))
	return agg
}

// ratio is num/den as a float, or 0 when den is 0 (no data for that metric).
func ratio(num, den int) float64 {
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}

// LoadGolden reads and validates a golden Q&A set from path.
func LoadGolden(path string) ([]Case, error) {
	// path is an operator-supplied CLI flag (cmd/eval's -golden), never
	// request input, so this is not attacker-controlled file inclusion.
	//nolint:gosec
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read golden set %s: %w", path, err)
	}
	return parseGolden(b, path)
}

// parseGolden decodes and validates golden JSON; split from LoadGolden so
// tests can validate in-memory bytes without a file. It rejects an empty
// set, a case missing an id or question, a duplicate case id, an in-scope
// case (expect_abstain false) with no expected citations — those would
// silently never test recall — and any expected citation with no
// doc_number. An out-of-scope case (expect_abstain true) is allowed to have
// no expected citations. Unknown JSON fields are rejected so a stale key
// (e.g. a leftover "notes") fails loudly instead of silently vanishing.
func parseGolden(b []byte, src string) ([]Case, error) {
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	var cases []Case
	if err := dec.Decode(&cases); err != nil {
		return nil, fmt.Errorf("parse golden set %s: %w", src, err)
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("golden set %s is empty", src)
	}

	seen := make(map[string]bool, len(cases))
	for i, c := range cases {
		if err := validateCase(src, i, c, seen); err != nil {
			return nil, err
		}
		seen[c.ID] = true
	}
	return cases, nil
}

// validateCase applies parseGolden's per-case rules to case i of src.
func validateCase(src string, i int, c Case, seen map[string]bool) error {
	switch {
	case c.ID == "":
		return fmt.Errorf("golden set %s: case %d has no id", src, i)
	case c.Question == "":
		return fmt.Errorf("golden set %s: case %q has no question", src, c.ID)
	case seen[c.ID]:
		return fmt.Errorf("golden set %s: duplicate case id %q", src, c.ID)
	case !c.ExpectAbstain && len(c.Expected) == 0:
		return fmt.Errorf(
			"golden set %s: in-scope case %q has no expected citations (set expect_abstain or add expected)", src, c.ID)
	}
	for j, ec := range c.Expected {
		if ec.DocNumber == "" {
			return fmt.Errorf("golden set %s: case %q expected citation %d has no doc_number", src, c.ID, j)
		}
	}
	return nil
}

// Thresholds are the minimum aggregate metrics a Report must meet. A zero
// field imposes no floor for that metric, so cmd/eval can gate on a subset.
type Thresholds struct {
	MinRecall   float64
	MinMRR      float64
	MinCitation float64
	MinInForce  float64
	MinAbstain  float64
}

// Check returns one human-readable message per threshold r's Aggregate did
// not meet. A metric with no contributing cases is skipped (it cannot pass
// or fail without data), except abstention, which every case contributes to.
// An empty result means every configured threshold passed.
func (t Thresholds) Check(r Report) []string {
	agg := r.Aggregate
	var fails []string
	add := func(metric string, got, want float64, hasData bool) {
		if want > 0 && hasData && got < want {
			fails = append(fails, fmt.Sprintf("%s: got %.3f, want >= %.3f", metric, got, want))
		}
	}
	add("recall@k", agg.RecallAtK, t.MinRecall, agg.RecallCases > 0)
	add("mrr@k", agg.MRRAtK, t.MinMRR, agg.MRRCases > 0)
	add("citation-correctness", agg.CitationCorrectness, t.MinCitation, agg.CitationCases > 0)
	add("current-law-precision", agg.InForcePrecision, t.MinInForce, agg.InForceCases > 0)
	add("abstention-accuracy", agg.AbstainAccuracy, t.MinAbstain, agg.Cases > 0)
	return fails
}

// WriteReport renders a human-readable per-case table plus the aggregate
// summary to w. It is deterministic (cases in input order) so the output
// diffs cleanly in CI logs.
func WriteReport(w io.Writer, r Report) {
	_, _ = fmt.Fprintln(w, "ID                    ABSTAIN  RECALL@K   RANK  CITES(grnd)  CURRENT   OK")
	_, _ = fmt.Fprintln(w, "--------------------  -------  ---------  ----  -----------  --------  --")
	for _, cr := range r.Results {
		_, _ = fmt.Fprintf(w, "%-20s  %-7s  %4d/%-4d  %-4s  %4d/%-6d  %5.0f%%   %s\n",
			truncate(cr.Case.ID, 20),
			boolMark(cr.Abstained),
			cr.RecallHits, cr.RecallWant,
			rankMark(cr.Rank),
			cr.CitationsGrounded, cr.CitationsMade,
			cr.InForcePrecision*100,
			passFail(cr.AbstainCorrect),
		)
	}
	agg := r.Aggregate
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "Cases: %d\n", agg.Cases)
	_, _ = fmt.Fprintf(w, "recall@k:              %s\n", pct(agg.RecallAtK, agg.RecallCases))
	_, _ = fmt.Fprintf(w, "mrr@k:                 %s\n", pct(agg.MRRAtK, agg.MRRCases))
	_, _ = fmt.Fprintf(w, "citation-correctness:  %s\n", pct(agg.CitationCorrectness, agg.CitationCases))
	_, _ = fmt.Fprintf(w, "current-law-precision: %s\n", pct(agg.InForcePrecision, agg.InForceCases))
	_, _ = fmt.Fprintf(w, "abstention-accuracy:   %s\n", pct(agg.AbstainAccuracy, agg.Cases))
}

// pct formats a rate as a percentage, or "n/a (0 cases)" when no case fed the
// metric, so a missing-data zero is never mistaken for a real 0%.
func pct(v float64, cases int) string {
	if cases == 0 {
		return "n/a (0 cases)"
	}
	return fmt.Sprintf("%.1f%% (%d cases)", v*100, cases)
}

// boolMark renders a yes/no for the abstain column.
func boolMark(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// passFail renders the per-case abstention-correctness check.
func passFail(b bool) string {
	if b {
		return "OK"
	}
	return "XX"
}

// rankMark renders MRR rank, or "-" when no expected citation was found.
func rankMark(rank int) string {
	if rank <= 0 {
		return "-"
	}
	return strconv.Itoa(rank)
}

// truncate caps s at n runes so the table column stays aligned.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
