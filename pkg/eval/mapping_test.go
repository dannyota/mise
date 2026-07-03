package eval

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestMappingScore(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		expected    bool
		proposed    bool
		wantCorrect bool
	}{
		{"true positive", true, true, true},
		{"true negative", false, false, true},
		{"false positive", false, true, false},
		{"false negative", true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := MappingCase{ID: "test", Expected: tt.expected}
			r := MappingScore(c, tt.proposed)
			if r.Correct != tt.wantCorrect {
				t.Errorf("MappingScore(expected=%v, proposed=%v).Correct = %v, want %v",
					tt.expected, tt.proposed, r.Correct, tt.wantCorrect)
			}
			if r.Proposed != tt.proposed {
				t.Errorf("Proposed = %v, want %v", r.Proposed, tt.proposed)
			}
		})
	}
}

func TestMappingPrecision(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		results      []MappingResult
		wantFrac     float64
		wantTP       int
		wantProposed int
	}{
		{
			name:     "no proposals -> no denominator",
			results:  []MappingResult{{Case: MappingCase{Expected: true}, Proposed: false}},
			wantFrac: 0, wantTP: 0, wantProposed: 0,
		},
		{
			name: "all proposals correct -> 1.0",
			results: []MappingResult{
				{Case: MappingCase{Expected: true}, Proposed: true},
				{Case: MappingCase{Expected: false}, Proposed: false},
			},
			wantFrac: 1, wantTP: 1, wantProposed: 1,
		},
		{
			name: "one TP one FP -> 0.5",
			results: []MappingResult{
				{Case: MappingCase{Expected: true}, Proposed: true},
				{Case: MappingCase{Expected: false}, Proposed: true},
			},
			wantFrac: 0.5, wantTP: 1, wantProposed: 2,
		},
		{
			name: "only FP -> 0",
			results: []MappingResult{
				{Case: MappingCase{Expected: false}, Proposed: true},
			},
			wantFrac: 0, wantTP: 0, wantProposed: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			frac, tp, proposed := MappingPrecision(tt.results)
			if frac != tt.wantFrac || tp != tt.wantTP || proposed != tt.wantProposed {
				t.Errorf("MappingPrecision = (%v, %d, %d), want (%v, %d, %d)",
					frac, tp, proposed, tt.wantFrac, tt.wantTP, tt.wantProposed)
			}
		})
	}
}

func TestMappingRecall(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		results       []MappingResult
		wantFrac      float64
		wantTP        int
		wantPositives int
	}{
		{
			name:     "no positives -> no denominator",
			results:  []MappingResult{{Case: MappingCase{Expected: false}, Proposed: false}},
			wantFrac: 0, wantTP: 0, wantPositives: 0,
		},
		{
			name: "all positives found -> 1.0",
			results: []MappingResult{
				{Case: MappingCase{Expected: true}, Proposed: true},
				{Case: MappingCase{Expected: true}, Proposed: true},
			},
			wantFrac: 1, wantTP: 2, wantPositives: 2,
		},
		{
			name: "one of two found -> 0.5",
			results: []MappingResult{
				{Case: MappingCase{Expected: true}, Proposed: true},
				{Case: MappingCase{Expected: true}, Proposed: false},
			},
			wantFrac: 0.5, wantTP: 1, wantPositives: 2,
		},
		{
			name: "none found -> 0",
			results: []MappingResult{
				{Case: MappingCase{Expected: true}, Proposed: false},
			},
			wantFrac: 0, wantTP: 0, wantPositives: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			frac, tp, positives := MappingRecall(tt.results)
			if frac != tt.wantFrac || tp != tt.wantTP || positives != tt.wantPositives {
				t.Errorf("MappingRecall = (%v, %d, %d), want (%v, %d, %d)",
					frac, tp, positives, tt.wantFrac, tt.wantTP, tt.wantPositives)
			}
		})
	}
}

func TestSummarizeMapping(t *testing.T) {
	t.Parallel()
	results := []MappingResult{
		{Case: MappingCase{Expected: true}, Proposed: true},   // TP
		{Case: MappingCase{Expected: true}, Proposed: false},  // FN
		{Case: MappingCase{Expected: false}, Proposed: true},  // FP
		{Case: MappingCase{Expected: false}, Proposed: false}, // TN
	}
	agg := SummarizeMapping(results)
	if agg.Cases != 4 {
		t.Errorf("Cases = %d, want 4", agg.Cases)
	}
	if agg.TP != 1 || agg.FP != 1 || agg.FN != 1 || agg.TN != 1 {
		t.Errorf("TP=%d FP=%d FN=%d TN=%d, want 1 1 1 1", agg.TP, agg.FP, agg.FN, agg.TN)
	}
	if !approx(agg.Precision, 0.5) {
		t.Errorf("Precision = %v, want 0.5", agg.Precision)
	}
	if !approx(agg.Recall, 0.5) {
		t.Errorf("Recall = %v, want 0.5", agg.Recall)
	}
}

func TestSummarizeMappingEmpty(t *testing.T) {
	t.Parallel()
	agg := SummarizeMapping(nil)
	if agg.Cases != 0 || agg.Precision != 0 || agg.Recall != 0 {
		t.Errorf("empty SummarizeMapping = %+v, want all-zero", agg)
	}
}

func TestMappingThresholdsCheck(t *testing.T) {
	t.Parallel()
	report := MappingReport{
		Aggregate: MappingAggregate{
			Cases: 4, TP: 2, FP: 1, FN: 1, TN: 0,
			Precision: 2.0 / 3.0, Recall: 2.0 / 3.0,
		},
	}

	t.Run("all pass", func(t *testing.T) {
		t.Parallel()
		fails := MappingThresholds{MinPrecision: 0.5, MinRecall: 0.5}.Check(report)
		if len(fails) != 0 {
			t.Errorf("got failures %+v, want none", fails)
		}
	})

	t.Run("precision below floor", func(t *testing.T) {
		t.Parallel()
		fails := MappingThresholds{MinPrecision: 0.9}.Check(report)
		if len(fails) != 1 || !strings.Contains(fails[0], "mapping-precision") {
			t.Errorf("got %+v, want one mapping-precision failure", fails)
		}
	})

	t.Run("recall below floor", func(t *testing.T) {
		t.Parallel()
		fails := MappingThresholds{MinRecall: 0.9}.Check(report)
		if len(fails) != 1 || !strings.Contains(fails[0], "mapping-recall") {
			t.Errorf("got %+v, want one mapping-recall failure", fails)
		}
	})

	t.Run("zero threshold imposes no floor", func(t *testing.T) {
		t.Parallel()
		fails := MappingThresholds{}.Check(report)
		if len(fails) != 0 {
			t.Errorf("got %+v, want none", fails)
		}
	})
}

// mappingJSON builds a valid mapping-case JSON object with the given
// overrides applied to a baseline template. Missing keys test validation.
func mappingJSON(id string, expected bool, omit string) string {
	parts := map[string]string{
		"id":          `"id":"` + id + `"`,
		"from_corpus": `"from_corpus":"s"`,
		"from_doc":    `"from_doc":"d"`,
		"from_text":   `"from_text":"t"`,
		"to_corpus":   `"to_corpus":"v"`,
		"to_doc":      `"to_doc":"d"`,
		"to_text":     `"to_text":"t"`,
		"edge_type":   `"edge_type":"satisfies"`,
	}
	delete(parts, omit)

	exp := "false"
	if expected {
		exp = "true"
	}

	keys := []string{
		"id", "from_corpus", "from_doc", "from_text",
		"to_corpus", "to_doc", "to_text", "edge_type",
	}
	var sb strings.Builder
	sb.WriteByte('{')
	first := true
	for _, k := range keys {
		if v, ok := parts[k]; ok {
			if !first {
				sb.WriteByte(',')
			}
			sb.WriteString(v)
			first = false
		}
	}
	if !first {
		sb.WriteByte(',')
	}
	sb.WriteString(`"expected":` + exp + `,"notes":""}`)
	return sb.String()
}

func TestLoadMappingGoldenValid(t *testing.T) {
	t.Parallel()
	a := mappingJSON("a", true, "")
	b := mappingJSON("b", false, "")
	in := "[" + a + "," + b + "]"
	cases, err := parseMappingGolden([]byte(in), "test")
	if err != nil {
		t.Fatalf("parseMappingGolden: %v", err)
	}
	if len(cases) != 2 {
		t.Fatalf("len = %d, want 2", len(cases))
	}
	if !cases[0].Expected || cases[1].Expected {
		t.Errorf("expected flags wrong: [0]=%v [1]=%v",
			cases[0].Expected, cases[1].Expected)
	}
}

func TestLoadMappingGoldenRejects(t *testing.T) {
	t.Parallel()
	full := func(id string, exp bool) string {
		return mappingJSON(id, exp, "")
	}
	tests := []struct {
		name string
		in   string
	}{
		{"empty array", `[]`},
		{"missing id", "[" + mappingJSON("", true, "id") + "]"},
		{
			"missing from_corpus",
			"[" + mappingJSON("a", true, "from_corpus") + "]",
		},
		{
			"missing from_doc",
			"[" + mappingJSON("a", true, "from_doc") + "]",
		},
		{
			"missing to_corpus",
			"[" + mappingJSON("a", true, "to_corpus") + "]",
		},
		{
			"missing to_doc",
			"[" + mappingJSON("a", true, "to_doc") + "]",
		},
		{
			"missing edge_type",
			"[" + mappingJSON("a", true, "edge_type") + "]",
		},
		{
			"duplicate id",
			"[" + full("a", true) + "," + full("a", false) + "]",
		},
		{
			"no positive examples",
			"[" + full("a", false) + "," + full("b", false) + "]",
		},
		{
			"no negative examples",
			"[" + full("a", true) + "," + full("b", true) + "]",
		},
		{
			"unknown field",
			`[{"id":"a","from_corpus":"s","from_doc":"d",` +
				`"from_text":"t","to_corpus":"v","to_doc":"d",` +
				`"to_text":"t","edge_type":"satisfies",` +
				`"expected":true,"notes":"","bogus":1}]`,
		},
		{"not json", `not json`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseMappingGolden([]byte(tt.in), "test"); err == nil {
				t.Errorf("parseMappingGolden(%s) = nil error, want rejection", tt.name)
			}
		})
	}
}

func TestLoadMappingGoldenMissingFile(t *testing.T) {
	t.Parallel()
	if _, err := LoadMappingGolden("/nonexistent/golden.json"); err == nil {
		t.Error("LoadMappingGolden(missing file) = nil error, want a read error")
	}
}

func TestWriteMappingReport(t *testing.T) {
	t.Parallel()
	results := []MappingResult{
		{Case: MappingCase{ID: "sat-001", Expected: true}, Proposed: true, Correct: true},
		{Case: MappingCase{ID: "sat-neg-001", Expected: false}, Proposed: false, Correct: true},
	}
	report := MappingReport{Results: results, Aggregate: SummarizeMapping(results)}

	var sb strings.Builder
	WriteMappingReport(&sb, report)
	out := sb.String()

	for _, want := range []string{"sat-001", "sat-neg-001", "mapping-precision", "mapping-recall", "TP=1"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q\n%s", want, out)
		}
	}
}

// TestGoldenSatisfiesVNParses proves the VN mapping golden set parses
// through LoadMappingGolden and meets the brief's minimums.
func TestGoldenSatisfiesVNParses(t *testing.T) {
	t.Parallel()
	cases, err := LoadMappingGolden("../../deploy/eval/golden-satisfies-vn.json")
	if err != nil {
		t.Fatalf("LoadMappingGolden(golden-satisfies-vn.json): %v", err)
	}

	var positive, negative int
	for _, c := range cases {
		if c.Expected {
			positive++
		} else {
			negative++
		}
	}
	if positive < 20 {
		t.Errorf("positive cases = %d, want >= 20", positive)
	}
	if negative < 5 {
		t.Errorf("negative cases = %d, want >= 5", negative)
	}
}

// TestGoldenSatisfiesMYParses proves the MY mapping golden set parses
// through LoadMappingGolden and meets the brief's minimums.
func TestGoldenSatisfiesMYParses(t *testing.T) {
	t.Parallel()
	cases, err := LoadMappingGolden("../../deploy/eval/golden-satisfies-my.json")
	if err != nil {
		t.Fatalf("LoadMappingGolden(golden-satisfies-my.json): %v", err)
	}

	var positive, negative int
	for _, c := range cases {
		if c.Expected {
			positive++
		} else {
			negative++
		}
	}
	if positive < 10 {
		t.Errorf("positive cases = %d, want >= 10", positive)
	}
	if negative < 5 {
		t.Errorf("negative cases = %d, want >= 5", negative)
	}
}

// fakeMappingSearcher is an in-memory MappingSearcher for RunMapping tests.
type fakeMappingSearcher struct {
	proposals map[string]bool // keyed by case ID via from.Doc
	err       error
}

func (f *fakeMappingSearcher) ProposesEdge(_ context.Context, from, _ MappingRef) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.proposals[from.Doc], nil
}

func TestRunMapping(t *testing.T) {
	t.Parallel()
	cases := []MappingCase{
		{
			ID: "pos", FromCorpus: "std", FromDoc: "ISO-A",
			FromText: "t", ToCorpus: "vn", ToDoc: "law",
			ToText: "t", EdgeType: "satisfies", Expected: true,
		},
		{
			ID: "neg", FromCorpus: "std", FromDoc: "ISO-B",
			FromText: "t", ToCorpus: "vn", ToDoc: "law",
			ToText: "t", EdgeType: "satisfies", Expected: false,
		},
	}
	s := &fakeMappingSearcher{
		proposals: map[string]bool{"ISO-A": true, "ISO-B": false},
	}

	report, err := RunMapping(context.Background(), s, cases)
	if err != nil {
		t.Fatalf("RunMapping() error = %v", err)
	}
	if len(report.Results) != 2 {
		t.Fatalf("Results = %d, want 2", len(report.Results))
	}
	if !report.Results[0].Correct || !report.Results[1].Correct {
		t.Errorf("expected both correct: [0]=%v [1]=%v",
			report.Results[0].Correct, report.Results[1].Correct)
	}
	if report.Aggregate.TP != 1 || report.Aggregate.TN != 1 {
		t.Errorf("aggregate TP=%d TN=%d, want 1 1", report.Aggregate.TP, report.Aggregate.TN)
	}
}

func TestRunMappingPropagatesError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("boom")
	s := &fakeMappingSearcher{err: wantErr}
	cases := []MappingCase{
		{ID: "c1", FromCorpus: "s", FromDoc: "d", ToCorpus: "v", ToDoc: "d", EdgeType: "satisfies"},
	}

	_, err := RunMapping(context.Background(), s, cases)
	if err == nil || !errors.Is(err, wantErr) {
		t.Errorf("RunMapping() error = %v, want wrapped %v", err, wantErr)
	}
}
