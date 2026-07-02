package eval

import (
	"context"
	"errors"
	"testing"

	"danny.vn/mise/pkg/store"
)

// hit is a tiny constructor for a store.Hit with the fields the metrics
// read, defaulting ValidityStatus to "in_force" (InForcePrecision has its
// own literals where validity varies).
func hit(docNumber, citationPath, headingPath string) store.Hit {
	return store.Hit{
		DocNumber: docNumber, CitationPath: citationPath, HeadingPath: headingPath,
		ValidityStatus: "in_force",
	}
}

func TestMatches(t *testing.T) {
	tests := []struct {
		name string
		ec   ExpectedCitation
		h    store.Hit
		want bool
	}{
		{
			name: "doc-only expectation matched case-insensitively",
			ec:   ExpectedCitation{DocNumber: "ACT 758"},
			h:    hit("Act 758", "part-vi/section-133", "Part VI — Secrecy"),
			want: true,
		},
		{
			name: "vn segment matched when the hit's citation path carries it",
			ec:   ExpectedCitation{DocNumber: "50/2024/tt-nhnn", Segments: []string{"dieu-7"}},
			h:    hit("50/2024/TT-NHNN", "chuong-i/dieu-7", "Chương I > Điều 7"),
			want: true,
		},
		{
			name: "vn segment missed when no hit names that dieu",
			ec:   ExpectedCitation{DocNumber: "50/2024/tt-nhnn", Segments: []string{"dieu-9"}},
			h:    hit("50/2024/TT-NHNN", "chuong-i/dieu-7", "Chương I > Điều 7"),
			want: false,
		},
		{
			name: "my multi-segment: section and subsection both required",
			ec:   ExpectedCitation{DocNumber: "Act 758", Segments: []string{"section-11", "subsection-2"}},
			h:    hit("Act 758", "part-ii/section-11/subsection-2", "Part II > Section 11 > (2)"),
			want: true,
		},
		{
			name: "my multi-segment: one segment missing fails the whole match",
			ec:   ExpectedCitation{DocNumber: "Act 758", Segments: []string{"section-11", "subsection-2"}},
			h:    hit("Act 758", "part-ii/section-11/subsection-3", "Part II > Section 11 > (3)"),
			want: false,
		},
		{
			name: "segment may appear in heading path rather than citation path",
			ec:   ExpectedCitation{DocNumber: "Act 758", Segments: []string{"secrecy"}},
			h:    hit("Act 758", "part-vi/section-133", "Part VI — Secrecy"),
			want: true,
		},
		{
			name: "wrong document misses even when the segment coincidentally matches",
			ec:   ExpectedCitation{DocNumber: "Act 758", Segments: []string{"section-133"}},
			h:    hit("Act 759", "part-vi/section-133", "Part VI — Secrecy"),
			want: false,
		},
		{
			name: "doc number may appear anywhere in doc/citation/heading text",
			ec:   ExpectedCitation{DocNumber: "758"},
			h:    hit("", "act-758/section-1", ""),
			want: true,
		},
		{
			name: "diacritics are preserved, not folded away",
			ec:   ExpectedCitation{DocNumber: "50/2024/tt-nhnn", Segments: []string{"doi tuong ap dung"}},
			h:    hit("50/2024/TT-NHNN", "dieu-2", "Điều 2 — Đối tượng áp dụng"),
			want: false, // "doi tuong" (no diacritics) does not match "Đối tượng" (with diacritics)
		},
		{
			name: "matching diacritics on both sides still match, case-insensitively",
			ec:   ExpectedCitation{DocNumber: "50/2024/tt-nhnn", Segments: []string{"ĐỐI TƯỢNG áp dụng"}},
			h:    hit("50/2024/TT-NHNN", "dieu-2", "Điều 2 — Đối tượng áp dụng"),
			want: true,
		},
		{
			name: "no expected citations segments means doc-level match suffices",
			ec:   ExpectedCitation{DocNumber: "BNM/pd-rmit-nov25"},
			h:    hit("BNM/pd-rmit-nov25", "fulltext", "Risk Management in Technology (RMiT)"),
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Matches(tt.ec, tt.h); got != tt.want {
				t.Errorf("Matches(%+v, %+v) = %v, want %v", tt.ec, tt.h, got, tt.want)
			}
		})
	}
}

// TestFoldNFCNormalizes proves fold canonicalizes Unicode composition (a
// combining-mark diacritic and its precomposed form fold equal) rather than
// only lowercasing — the reason Matches uses fold instead of a plain
// strings.ToLower/Contains.
func TestFoldNFCNormalizes(t *testing.T) {
	precomposed := "è" // è, single rune
	decomposed := "è" // e + combining grave accent, two runes
	if precomposed == decomposed {
		t.Fatal("test fixture invariant broken: the two forms must differ byte-for-byte")
	}
	if fold(precomposed) != fold(decomposed) {
		t.Errorf("fold(%q) = %q, fold(%q) = %q, want equal after NFC normalization",
			precomposed, fold(precomposed), decomposed, fold(decomposed))
	}
}

func TestRecall(t *testing.T) {
	tests := []struct {
		name      string
		expected  []ExpectedCitation
		hits      []store.Hit
		wantFrac  float64
		wantFound int
		wantWant  int
	}{
		{
			name:     "no expected citations (out of scope) has no denominator",
			expected: nil,
			hits:     []store.Hit{hit("50/2024/tt-nhnn", "dieu-7", "")},
			wantFrac: 0, wantFound: 0, wantWant: 0,
		},
		{
			name:     "doc-only expectation matched",
			expected: []ExpectedCitation{{DocNumber: "50/2024/TT-NHNN"}},
			hits:     []store.Hit{hit("50/2024/tt-nhnn", "dieu-7/khoan-2", "")},
			wantFrac: 1, wantFound: 1, wantWant: 1,
		},
		{
			name:     "segment expectation matched when a hit names it",
			expected: []ExpectedCitation{{DocNumber: "09/2020/tt-nhnn", Segments: []string{"dieu-4"}}},
			hits:     []store.Hit{hit("09/2020/tt-nhnn", "dieu-4", "")},
			wantFrac: 1, wantFound: 1, wantWant: 1,
		},
		{
			name:     "segment expectation missed when no hit names it",
			expected: []ExpectedCitation{{DocNumber: "09/2020/tt-nhnn", Segments: []string{"dieu-4"}}},
			hits:     []store.Hit{hit("09/2020/tt-nhnn", "dieu-9", "")},
			wantFrac: 0, wantFound: 0, wantWant: 1,
		},
		{
			name: "two expected, one found -> 0.5",
			expected: []ExpectedCitation{
				{DocNumber: "50/2024/tt-nhnn"},
				{DocNumber: "Act 758"},
			},
			hits:     []store.Hit{hit("50/2024/tt-nhnn", "dieu-7", "")},
			wantFrac: 0.5, wantFound: 1, wantWant: 2,
		},
		{
			name:     "wrong document misses",
			expected: []ExpectedCitation{{DocNumber: "50/2024/tt-nhnn"}},
			hits:     []store.Hit{hit("09/2020/tt-nhnn", "dieu-4", "")},
			wantFrac: 0, wantFound: 0, wantWant: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Case{Expected: tt.expected}
			frac, found, want := Recall(c, tt.hits)
			if frac != tt.wantFrac || found != tt.wantFound || want != tt.wantWant {
				t.Errorf("Recall = (%v, %d, %d), want (%v, %d, %d)",
					frac, found, want, tt.wantFrac, tt.wantFound, tt.wantWant)
			}
		})
	}
}

func TestReciprocalRank(t *testing.T) {
	tests := []struct {
		name     string
		expected []ExpectedCitation
		hits     []store.Hit
		wantRR   float64
		wantRank int
	}{
		{
			name:     "no expected citations has no denominator",
			expected: nil,
			hits:     []store.Hit{hit("50/2024/tt-nhnn", "dieu-7", "")},
			wantRR:   0, wantRank: 0,
		},
		{
			name:     "first hit",
			expected: []ExpectedCitation{{DocNumber: "50/2024/tt-nhnn"}},
			hits:     []store.Hit{hit("50/2024/tt-nhnn", "dieu-7", "")},
			wantRR:   1, wantRank: 1,
		},
		{
			name:     "third hit",
			expected: []ExpectedCitation{{DocNumber: "50/2024/tt-nhnn"}},
			hits: []store.Hit{
				hit("09/2020/tt-nhnn", "dieu-4", ""),
				hit("17/2024/tt-nhnn", "dieu-1", ""),
				hit("50/2024/tt-nhnn", "dieu-7", ""),
			},
			wantRR: 1.0 / 3.0, wantRank: 3,
		},
		{
			name:     "missing expected citation",
			expected: []ExpectedCitation{{DocNumber: "50/2024/tt-nhnn"}},
			hits:     []store.Hit{hit("09/2020/tt-nhnn", "dieu-4", "")},
			wantRR:   0, wantRank: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRR, gotRank := ReciprocalRank(Case{Expected: tt.expected}, tt.hits)
			if gotRR != tt.wantRR || gotRank != tt.wantRank {
				t.Errorf("ReciprocalRank = (%v, %d), want (%v, %d)", gotRR, gotRank, tt.wantRR, tt.wantRank)
			}
		})
	}
}

func TestInForcePrecision(t *testing.T) {
	hits := []store.Hit{
		{DocNumber: "50/2024/tt-nhnn", ValidityStatus: "in_force"},
		{DocNumber: "13/2023/nd-cp", ValidityStatus: "repealed"}, // leak candidate
		{DocNumber: "91/2025/qh15", ValidityStatus: "amended"},
	}

	t.Run("all in force -> 1.0", func(t *testing.T) {
		frac, ok, total := InForcePrecision(hits[:1])
		if frac != 1 || ok != 1 || total != 1 {
			t.Errorf("got (%v, %d, %d), want (1, 1, 1)", frac, ok, total)
		}
	})

	t.Run("repealed leak ABOVE current law counts against precision", func(t *testing.T) {
		// The non-current hit sits between current hits, so it cannot be the
		// trailing badged pass — it is a real leak and counts.
		frac, ok, total := InForcePrecision(hits)
		want := 2.0 / 3.0
		if frac != want || ok != 2 || total != 3 {
			t.Errorf("got (%v, %d, %d), want (%v, 2, 3)", frac, ok, total, want)
		}
	})

	t.Run("trailing non-current run is excluded", func(t *testing.T) {
		trailing := []store.Hit{hits[0], hits[1]} // in_force then repealed, in that order
		frac, ok, total := InForcePrecision(trailing)
		if frac != 1 || ok != 1 || total != 1 {
			t.Errorf("got (%v, %d, %d), want (1, 1, 1)", frac, ok, total)
		}
	})

	t.Run("nothing current at all -> scored over everything, 0", func(t *testing.T) {
		allRepealed := []store.Hit{
			{ValidityStatus: "repealed"}, {ValidityStatus: "superseded"}, {ValidityStatus: "not_yet_effective"},
		}
		frac, ok, total := InForcePrecision(allRepealed)
		if frac != 0 || ok != 0 || total != 3 {
			t.Errorf("got (%v, %d, %d), want (0, 0, 3)", frac, ok, total)
		}
	})

	t.Run("no hits -> no denominator", func(t *testing.T) {
		frac, ok, total := InForcePrecision(nil)
		if frac != 0 || ok != 0 || total != 0 {
			t.Errorf("got (%v, %d, %d), want (0, 0, 0)", frac, ok, total)
		}
	})
}

func TestAbstainCorrect(t *testing.T) {
	tests := []struct {
		name          string
		expectAbstain bool
		abstained     bool
		want          bool
	}{
		{"in-scope answered correctly", false, false, true},
		{"in-scope wrongly abstained", false, true, false},
		{"out-of-scope correctly abstained", true, true, true},
		{"out-of-scope wrongly answered", true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Case{ExpectAbstain: tt.expectAbstain}
			if got := AbstainCorrect(c, tt.abstained); got != tt.want {
				t.Errorf("AbstainCorrect = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCitationPrecision(t *testing.T) {
	tests := []struct {
		name         string
		expected     []ExpectedCitation
		hits         []store.Hit
		wantFrac     float64
		wantGrounded int
		wantMade     int
	}{
		{
			name:     "no hits has no denominator",
			expected: []ExpectedCitation{{DocNumber: "Act 758"}},
			hits:     nil,
			wantFrac: 0, wantGrounded: 0, wantMade: 0,
		},
		{
			name:     "every hit matches -> 1.0",
			expected: []ExpectedCitation{{DocNumber: "Act 758", Segments: []string{"section-133"}}},
			hits: []store.Hit{
				hit("Act 758", "part-vi/section-133", ""),
				hit("Act 758", "part-vi/section-133", ""),
			},
			wantFrac: 1, wantGrounded: 2, wantMade: 2,
		},
		{
			name:     "one of two hits matches -> 0.5",
			expected: []ExpectedCitation{{DocNumber: "Act 758", Segments: []string{"section-133"}}},
			hits: []store.Hit{
				hit("Act 758", "part-vi/section-133", ""),
				hit("Act 731", "section-6", ""),
			},
			wantFrac: 0.5, wantGrounded: 1, wantMade: 2,
		},
		{
			name:     "out-of-scope case with leaked hits scores 0, not excluded",
			expected: nil,
			hits:     []store.Hit{hit("Act 758", "part-vi/section-133", "")},
			wantFrac: 0, wantGrounded: 0, wantMade: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Case{Expected: tt.expected}
			frac, grounded, made := CitationPrecision(c, tt.hits)
			if frac != tt.wantFrac || grounded != tt.wantGrounded || made != tt.wantMade {
				t.Errorf("CitationPrecision = (%v, %d, %d), want (%v, %d, %d)",
					frac, grounded, made, tt.wantFrac, tt.wantGrounded, tt.wantMade)
			}
		})
	}
}

// TestScore checks that Score wires every metric together for a realistic
// in-scope case with a partial recall match and one leaked repealed hit.
func TestScore(t *testing.T) {
	c := Case{
		ID:       "q-test",
		Question: "What are the secrecy obligations under the Financial Services Act 2013?",
		Expected: []ExpectedCitation{
			{DocNumber: "Act 758", Segments: []string{"section-133"}},
			{DocNumber: "missing-act"},
		},
	}
	hits := []store.Hit{
		{DocNumber: "Act 758", CitationPath: "part-vi/section-133", ValidityStatus: "in_force"},
		{DocNumber: "Act 731", CitationPath: "section-6", ValidityStatus: "repealed"}, // leak above current law
		{DocNumber: "Act 709", CitationPath: "section-9", ValidityStatus: "amended"},
	}

	r := Score(c, hits, false)

	if r.RecallHits != 1 || r.RecallWant != 2 || r.RecallAtK != 0.5 {
		t.Errorf("recall = %d/%d (%v), want 1/2 (0.5)", r.RecallHits, r.RecallWant, r.RecallAtK)
	}
	if r.Rank != 1 || r.MRRAtK != 1 {
		t.Errorf("mrr = rank %d rr %v, want rank 1 rr 1", r.Rank, r.MRRAtK)
	}
	if r.CitationsGrounded != 1 || r.CitationsMade != 3 {
		t.Errorf("citation = %d/%d, want 1/3", r.CitationsGrounded, r.CitationsMade)
	}
	if r.HitsInForce != 2 || r.HitsTotal != 3 || r.InForcePrecision != 2.0/3.0 {
		t.Errorf("in-force = %d/%d (%v), want 2/3", r.HitsInForce, r.HitsTotal, r.InForcePrecision)
	}
	if !r.AbstainCorrect {
		t.Error("AbstainCorrect = false, want true (in-scope, answered)")
	}
}

func TestShouldAbstain(t *testing.T) {
	tests := []struct {
		name     string
		hits     []store.Hit
		minScore float64
		want     bool
	}{
		{"no hits always abstains", nil, 0, true},
		{"hits present, floor disabled -> answered", []store.Hit{{Score: 0.01}}, 0, false},
		{"hits present, top score below floor -> abstains", []store.Hit{{Score: 0.2}}, 0.5, true},
		{"hits present, top score at or above floor -> answered", []store.Hit{{Score: 0.5}}, 0.5, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldAbstain(tt.hits, tt.minScore); got != tt.want {
				t.Errorf("shouldAbstain(%v, %v) = %v, want %v", tt.hits, tt.minScore, got, tt.want)
			}
		})
	}
}

// fakeSearcher is an in-memory Searcher keyed by question text, for Run
// tests — no DB, no network. capturedOpts records the store.SearchOpts each
// question was searched with, so tests can assert RunOpts was forwarded.
type fakeSearcher struct {
	hits         map[string][]store.Hit
	err          error
	capturedOpts map[string]store.SearchOpts
}

func (f *fakeSearcher) Search(_ context.Context, query string, opts store.SearchOpts) ([]store.Hit, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.capturedOpts != nil {
		f.capturedOpts[query] = opts
	}
	return f.hits[query], nil
}

func TestRun(t *testing.T) {
	cases := []Case{
		{ID: "found", Question: "q1", Expected: []ExpectedCitation{{DocNumber: "Act 758"}}},
		{ID: "abstain", Question: "q2", ExpectAbstain: true},
	}
	s := &fakeSearcher{
		hits: map[string][]store.Hit{
			"q1": {hit("Act 758", "section-133", "")},
			"q2": nil, // no hits -> Run treats this as an abstain
		},
		capturedOpts: map[string]store.SearchOpts{},
	}

	report, err := Run(context.Background(), s, cases, RunOpts{TopK: 5, InForceOnly: true, Role: "mise_public"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(report.Results) != 2 {
		t.Fatalf("Results = %d, want 2", len(report.Results))
	}
	if report.Results[0].Abstained {
		t.Error("case 'found' Abstained = true, want false (it had hits)")
	}
	if !report.Results[1].Abstained || !report.Results[1].AbstainCorrect {
		t.Errorf("case 'abstain' = %+v, want Abstained and AbstainCorrect", report.Results[1])
	}
	if report.Aggregate.Cases != 2 {
		t.Errorf("Aggregate.Cases = %d, want 2", report.Aggregate.Cases)
	}

	opts := s.capturedOpts["q1"]
	if opts.TopK != 5 || !opts.InForceOnly || opts.Role != "mise_public" {
		t.Errorf("Search() opts = %+v, want RunOpts forwarded", opts)
	}
}

// TestRunAppliesAbstainMinScore is the regression test for IMPORTANT C: Run
// must thread RunOpts.AbstainMinScore into shouldAbstain, not the old
// hardcoded 0 (else -min-abstain is structurally unreachable).
func TestRunAppliesAbstainMinScore(t *testing.T) {
	cases := []Case{
		{ID: "below-floor", Question: "q1", Expected: []ExpectedCitation{{DocNumber: "Act 758"}}},
		{ID: "at-floor", Question: "q2", Expected: []ExpectedCitation{{DocNumber: "Act 758"}}},
	}
	s := &fakeSearcher{
		hits: map[string][]store.Hit{
			"q1": {{DocNumber: "Act 758", Score: 0.2}}, // below the 0.5 floor
			"q2": {{DocNumber: "Act 758", Score: 0.5}}, // at the floor
		},
	}

	report, err := Run(context.Background(), s, cases, RunOpts{AbstainMinScore: 0.5})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !report.Results[0].Abstained {
		t.Errorf("case %q Abstained = false, want true (top score 0.2 below floor 0.5)", cases[0].ID)
	}
	if report.Results[1].Abstained {
		t.Errorf("case %q Abstained = true, want false (top score 0.5 at floor 0.5)", cases[1].ID)
	}
}

func TestRunPropagatesSearchError(t *testing.T) {
	wantErr := errors.New("boom")
	s := &fakeSearcher{err: wantErr}
	cases := []Case{{ID: "c1", Question: "q1", ExpectAbstain: true}}

	_, err := Run(context.Background(), s, cases, RunOpts{})
	if err == nil || !errors.Is(err, wantErr) {
		t.Errorf("Run() error = %v, want wrapped %v", err, wantErr)
	}
}
