package eval

import "testing"

// TestGoldenVNParses proves deploy/eval/golden-vn.json parses through
// LoadGolden — the same loader cmd/eval uses — and carries the shape the
// harness expects (dup-id/empty-id/in-scope-needs-citation validation
// already ran inside LoadGolden; this only spot-checks content).
func TestGoldenVNParses(t *testing.T) {
	cases, err := LoadGolden("../../deploy/eval/golden-vn.json")
	if err != nil {
		t.Fatalf("LoadGolden(golden-vn.json): %v", err)
	}
	if len(cases) < 15 {
		t.Fatalf("len(cases) = %d, want >= 15", len(cases))
	}

	var abstain int
	byID := make(map[string]Case, len(cases))
	for _, c := range cases {
		byID[c.ID] = c
		if c.ExpectAbstain {
			abstain++
		}
	}
	if abstain != 2 {
		t.Errorf("abstain cases = %d, want 2", abstain)
	}

	// Spot-check a segment-level and a doc-only citation survived conversion.
	segCase, ok := byID["onbank-password-length-50-2024"]
	if !ok {
		t.Fatal(`case "onbank-password-length-50-2024" missing`)
	}
	wantSegs := []string{"dieu-11", "khoan-1", "diem-a"}
	gotSegs := segCase.Expected[0].Segments
	if len(gotSegs) != len(wantSegs) {
		t.Fatalf("segments = %v, want %v", gotSegs, wantSegs)
	}
	for i, want := range wantSegs {
		if gotSegs[i] != want {
			t.Errorf("segments[%d] = %q, want %q", i, gotSegs[i], want)
		}
	}

	docOnly, ok := byID["edge-soky-only-50-2024"]
	if !ok {
		t.Fatal(`case "edge-soky-only-50-2024" missing`)
	}
	ec := docOnly.Expected
	if len(ec) != 1 || ec[0].DocNumber != "50/2024/tt-nhnn" || len(ec[0].Segments) != 0 {
		t.Errorf("edge-soky-only-50-2024 expected = %+v, want doc-only 50/2024/tt-nhnn", ec)
	}
}

// TestGoldenMYParses proves deploy/eval/golden-my.json parses through
// LoadGolden, meets the brief's >= 15 cases floor, and carries exactly the 2
// abstain controls.
func TestGoldenMYParses(t *testing.T) {
	cases, err := LoadGolden("../../deploy/eval/golden-my.json")
	if err != nil {
		t.Fatalf("LoadGolden(golden-my.json): %v", err)
	}
	if len(cases) < 15 {
		t.Fatalf("len(cases) = %d, want >= 15", len(cases))
	}

	var abstain, inScope int
	for _, c := range cases {
		if c.ExpectAbstain {
			abstain++
			continue
		}
		inScope++
		if len(c.Expected) == 0 {
			t.Errorf("in-scope case %q has no expected citations", c.ID)
		}
		for _, ec := range c.Expected {
			if ec.DocNumber == "" {
				t.Errorf("case %q has an expected citation with no doc_number", c.ID)
			}
		}
	}
	if abstain != 2 {
		t.Errorf("abstain cases = %d, want 2", abstain)
	}
	if inScope < 13 {
		t.Errorf("in-scope cases = %d, want >= 13 (>= 15 total with the 2 abstain controls)", inScope)
	}
}
