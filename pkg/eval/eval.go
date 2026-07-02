// Package eval is mise's retrieval-quality eval harness. It scores
// pkg/store.Search against a golden Q&A set with deterministic metrics —
// recall@k, MRR@k, citation correctness, current-law precision, and
// abstention correctness — so changes to chunking, ranking, or the RRF fusion
// weights can be gated before they lock in defaults (TESTING.md §5). mise's
// serving layer is evidence-only; there is no answer model to score, so every
// metric here is computed directly from the ranked []store.Hit a search
// returns.
//
// This file is a generalized port of banhmi's pkg/eval/eval.go: Recall,
// ReciprocalRank, InForcePrecision, and AbstainCorrect keep banhmi's exact
// semantics. Citation matching generalizes banhmi's VN-only Điều/Khoản
// keyword scan (citationHasNumber) into Matches, a jurisdiction-agnostic
// substring test over mise's CitationPath/HeadingPath. CitationPrecision
// reinterprets banhmi's CitationCorrectness (in banhmi: "of the citations the
// answer made, the fraction grounded in the retrieved hits") for a
// retrieval-only harness — see its doc comment.
//
// Every function here is pure (no database, no network) so it is unit-tested
// with synthetic []store.Hit; Run wires the live Searcher, runs every case,
// and aggregates the per-case CaseResults into a Report (report.go).
package eval

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/text/unicode/norm"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/store"
)

// ExpectedCitation is one legal reference a golden Case expects retrieval to
// surface. DocNumber is the document's citation number — vn-reg's số ký hiệu
// or my-reg's Act/BNM/SC reference (store.Hit.DocNumber). Segments are
// optional citation-path tokens in mise's CitationPath format (e.g.
// "dieu-7", "khoan-2", "section-133", pkg/parse/{vnlaw,mylaw}) a matching hit
// must also carry; a nil/empty Segments means DocNumber alone is enough (a
// document-level citation).
type ExpectedCitation struct {
	DocNumber string   `json:"doc_number"`
	Segments  []string `json:"segments,omitempty"`
}

// Case is one golden question with its expectations. Expected lists the
// citations a good retrieval should surface (empty only when ExpectAbstain is
// true — an out-of-scope question). ExpectAbstain is true when the correct
// behavior is to abstain (out of scope / not in the corpus).
type Case struct {
	ID            string             `json:"id"`
	Question      string             `json:"question"`
	Expected      []ExpectedCitation `json:"expected"`
	ExpectAbstain bool               `json:"expect_abstain"`
}

// CaseResult is the scored outcome of one Case against a live search's hits.
// The metric fields are per-case; report.go's Summarize aggregates them.
// Counts (denominators) are kept alongside each rate so aggregation is a true
// micro-average rather than a mean-of-means.
type CaseResult struct {
	Case      Case
	Abstained bool // the search abstained (Run: zero hits)

	// RecallAtK: fraction of Expected citations found among the retrieved
	// hits. RecallHits / RecallWant are the numerator / denominator.
	RecallAtK  float64
	RecallHits int
	RecallWant int

	// MRRAtK: reciprocal rank of the first expected citation found in the hit
	// list. Rank is 1-based; 0 means no expected citation was retrieved.
	MRRAtK float64
	Rank   int

	// CitationCorrectness: of the hits retrieval returned, the fraction that
	// match some Expected citation. CitationsMade / CitationsGrounded are the
	// denominator / numerator — see CitationPrecision.
	CitationCorrectness float64
	CitationsMade       int
	CitationsGrounded   int

	// InForcePrecision: fraction of returned hits that are current law
	// (ValidityStatus in_force/amended). With InForceOnly:true search this
	// should be 1.0; a value below surfaces a leak. HitsInForce / HitsTotal
	// are the numerator / denominator.
	InForcePrecision float64
	HitsInForce      int
	HitsTotal        int

	// AbstainCorrect: the run's Abstained matched the case's ExpectAbstain.
	AbstainCorrect bool
}

// Searcher is the retrieval seam Run scores — pkg/store.Search's shape minus
// the *pgxpool.Pool and embed.Embedder parameters, which cmd/eval closes over
// so pkg/eval itself never imports pgx or touches a live connection.
type Searcher interface {
	Search(ctx context.Context, query string, opts store.SearchOpts) ([]store.Hit, error)
}

// RunOpts controls how Run queries Searcher for every Case: the corpus/role
// scope and retrieval depth, forwarded into a store.SearchOpts per case.
type RunOpts struct {
	Corpora     []corpus.ID
	TopK        int
	InForceOnly bool
	Role        string
}

// Report is Run's output: every case's scored CaseResult plus the
// micro-averaged Aggregate roll-up (report.go's Summarize). Thresholds.Check
// reads Aggregate; cmd/eval prints Results as a per-case table via
// WriteReport.
type Report struct {
	Results   []CaseResult
	Aggregate Aggregate
}

// Run executes every case in cases against s and returns the scored Report —
// per-case CaseResults plus their micro-averaged Aggregate. A case's search
// returning zero hits counts as an abstain (shouldAbstain); Run stops and
// returns the first search error, wrapped with the failing case's id.
func Run(ctx context.Context, s Searcher, cases []Case, opts RunOpts) (Report, error) {
	searchOpts := store.SearchOpts{
		Corpora:     opts.Corpora,
		TopK:        opts.TopK,
		InForceOnly: opts.InForceOnly,
		Role:        opts.Role,
	}

	results := make([]CaseResult, 0, len(cases))
	for _, c := range cases {
		hits, err := s.Search(ctx, c.Question, searchOpts)
		if err != nil {
			return Report{}, fmt.Errorf("eval: search case %q: %w", c.ID, err)
		}
		results = append(results, Score(c, hits, shouldAbstain(hits, 0)))
	}
	return Report{Results: results, Aggregate: Summarize(results)}, nil
}

// shouldAbstain ports banhmi's cmd/eval retrievalShouldAbstain rule: an empty
// hit list always abstains; a positive minScore additionally abstains when
// the top hit's score falls below it. Run always calls this with minScore 0
// (score-floor check disabled), matching banhmi's own default wiring
// (-abstain-min-score defaults to 0) — mise's Run/RunOpts has no flag to set
// a nonzero floor today, so only the zero-hits branch is currently reachable.
func shouldAbstain(hits []store.Hit, minScore float64) bool {
	if len(hits) == 0 {
		return true
	}
	if minScore <= 0 {
		return false
	}
	return hits[0].Score < minScore
}

// Score runs every retrieval metric for one case and returns the combined
// CaseResult. hits is the retrieved evidence; abstained is whether the run
// decided to abstain (Run: zero hits).
func Score(c Case, hits []store.Hit, abstained bool) CaseResult {
	r := CaseResult{Case: c, Abstained: abstained}
	r.RecallAtK, r.RecallHits, r.RecallWant = Recall(c, hits)
	r.MRRAtK, r.Rank = ReciprocalRank(c, hits)
	r.CitationCorrectness, r.CitationsGrounded, r.CitationsMade = CitationPrecision(c, hits)
	r.InForcePrecision, r.HitsInForce, r.HitsTotal = InForcePrecision(hits)
	r.AbstainCorrect = AbstainCorrect(c, abstained)
	return r
}

// Recall computes recall@k for one case: the fraction of Expected citations
// that some retrieved hit Matches. An out-of-scope case (no Expected
// citations) has no recall denominator and returns (0, 0, 0).
func Recall(c Case, hits []store.Hit) (frac float64, found, want int) {
	want = len(c.Expected)
	if want == 0 {
		return 0, 0, 0
	}
	for _, ec := range c.Expected {
		if expectedInHits(ec, hits) {
			found++
		}
	}
	return float64(found) / float64(want), found, want
}

// expectedInHits reports whether some retrieved hit Matches ec.
func expectedInHits(ec ExpectedCitation, hits []store.Hit) bool {
	for _, h := range hits {
		if Matches(ec, h) {
			return true
		}
	}
	return false
}

// ReciprocalRank computes reciprocal rank for one case: 1/rank of the first
// retrieved hit Matching any Expected citation. Rank is 1-based. A missing
// expected citation contributes 0. Out-of-scope cases have no denominator and
// return (0, 0).
func ReciprocalRank(c Case, hits []store.Hit) (rr float64, rank int) {
	if len(c.Expected) == 0 {
		return 0, 0
	}
	for i, h := range hits {
		for _, ec := range c.Expected {
			if Matches(ec, h) {
				return 1.0 / float64(i+1), i + 1
			}
		}
	}
	return 0, 0
}

// CitationPrecision computes citation correctness for one case: of the hits
// retrieval actually returned, the fraction that Match some Expected
// citation. In banhmi, CitationCorrectness scored an LLM answer's citations
// against the grounding evidence ("of the citations the answer made, the
// fraction grounded in the retrieved hit set") — mise's serving layer has no
// answer model at eval time, so this generalizes the same made/grounded
// shape directly onto the hits Search returned: each hit IS a citation the
// system "makes" by presenting it as evidence, and Matches (the same
// predicate Recall/MRR use) decides whether that citation is grounded
// (relevant to the case) rather than noise. This makes the metric a
// precision counterpart to Recall, and — unlike banhmi's dead field, which
// nothing ever populated once its answer mode was removed — a live signal:
// an out-of-scope case that leaks hits scores 0 here even though it has no
// Expected citations to leak against. Cases with zero hits have no
// denominator and return (0, 0, 0), so a correct abstain is excluded rather
// than scored 0.
func CitationPrecision(c Case, hits []store.Hit) (frac float64, grounded, made int) {
	made = len(hits)
	if made == 0 {
		return 0, 0, 0
	}
	for _, h := range hits {
		if anyExpectedMatches(c.Expected, h) {
			grounded++
		}
	}
	return float64(grounded) / float64(made), grounded, made
}

// anyExpectedMatches reports whether h Matches any of expected.
func anyExpectedMatches(expected []ExpectedCitation, h store.Hit) bool {
	for _, ec := range expected {
		if Matches(ec, h) {
			return true
		}
	}
	return false
}

// inForceStatuses mirrors pkg/store/search.go's validityPredicate: the exact
// store.Hit.ValidityStatus values Search's InForceOnly:true filter treats as
// current law.
var inForceStatuses = map[string]bool{
	"in_force": true,
	"amended":  true,
}

// isInForce reports whether h's own ValidityStatus is current law.
func isInForce(h store.Hit) bool {
	return inForceStatuses[h.ValidityStatus]
}

// InForcePrecision computes the fraction of returned hits that are current
// law. Search's default (InForceOnly:false) may deliberately return a
// trailing pass of non-current law after the current results as disclosed
// evidence rather than a leak, so the trailing run of non-current hits is
// trimmed before scoring — ported byte-for-byte from banhmi's InForcePrecision.
// Any non-current hit ABOVE the last current hit still counts against
// precision (a real leak: stale validity or a filter failure). With
// InForceOnly:true this should be 1.0. No hits returns (0, 0, 0).
func InForcePrecision(hits []store.Hit) (frac float64, ok, total int) {
	end := len(hits)
	for end > 0 && !isInForce(hits[end-1]) {
		end--
	}
	if end == 0 && len(hits) > 0 {
		// Nothing current at all: score over everything rather than
		// vacuously passing.
		end = len(hits)
	}
	scored := hits[:end]
	total = len(scored)
	if total == 0 {
		return 0, 0, 0
	}
	for _, h := range scored {
		if isInForce(h) {
			ok++
		}
	}
	return float64(ok) / float64(total), ok, total
}

// AbstainCorrect reports whether the run's abstention matched the case's
// expectation: an out-of-scope case should abstain, an in-scope one should
// not.
func AbstainCorrect(c Case, abstained bool) bool {
	return abstained == c.ExpectAbstain
}

// Matches reports whether hit h satisfies expected citation ec: fold(h)
// folds a string to lowercase, NFC-normalized Unicode (diacritics preserved —
// vn-reg citation numbers and headings carry meaningful diacritics; only case
// and Unicode composition are folded). ec.DocNumber must fold-appear
// somewhere in h's own document/citation/heading text, AND every one of
// ec.Segments must fold-appear in h's citation-path/heading text. An
// ExpectedCitation with no Segments matches any hit from the right document
// (a document-level citation).
func Matches(ec ExpectedCitation, h store.Hit) bool {
	docHaystack := fold(h.DocNumber + " " + h.CitationPath + " " + h.HeadingPath)
	if !strings.Contains(docHaystack, fold(ec.DocNumber)) {
		return false
	}
	segHaystack := fold(h.CitationPath + " " + h.HeadingPath)
	for _, seg := range ec.Segments {
		if !strings.Contains(segHaystack, fold(seg)) {
			return false
		}
	}
	return true
}

// fold normalizes s for citation matching: NFC-normalize, then lowercase.
// Diacritics are preserved deliberately (see Matches).
func fold(s string) string {
	return strings.ToLower(norm.NFC.String(s))
}
