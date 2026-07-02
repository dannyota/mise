package mylaw_test

import (
	"testing"

	"danny.vn/mise/pkg/parse/law"
	"danny.vn/mise/pkg/parse/mylaw"
)

// testAct is a compact Act that exercises the proven recipe: a front
// "Arrangement of Sections" TOC (must be skipped), the enacting clause, two
// Parts, sections with inline + standalone subsections and paragraphs,
// page-header noise, and a Schedule whose own 1./2. numbering must NOT be
// read as sections.
const testAct = `LAWS OF MALAYSIA

ARRANGEMENT OF SECTIONS

PART I
1. Short title
2. Interpretation
PART II
3. Powers

ENACTED by the Parliament of Malaysia as follows:

PART I
PRELIMINARY
Short title and commencement
1. (1) This Act may be cited as the Test Act 2026.
(2) This Act comes into operation on a date appointed by the Minister.
Interpretation
2. (1) In this Act, unless the context otherwise requires—
(a) the first defined term;
(b) the second defined term.
24
Laws of Malaysia
ACT 999
PART II
REGULATORY OBJECTIVES AND POWERS
3. The Bank shall regulate the matters set out in this Act.

FIRST SCHEDULE
1. This is a schedule paragraph, not section one.
2. This is another schedule paragraph.`

func collect(nodes []*law.Node, kind string, out *[]string) {
	for _, n := range nodes {
		if n.Kind == kind {
			*out = append(*out, n.Label)
		}
		collect(n.Children, kind, out)
	}
}

func findByPath(nodes []*law.Node, path string) *law.Node {
	for _, n := range nodes {
		if n.CitationPath == path {
			return n
		}
		if got := findByPath(n.Children, path); got != nil {
			return got
		}
	}
	return nil
}

func TestParse_structure(t *testing.T) {
	roots, err := mylaw.Parse(testAct)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// Top level: Part I, Part II, and the Schedule (TOC before ENACTED is skipped).
	var parts []string
	collect(roots, "part", &parts)
	if len(parts) != 2 || parts[0] != "Part I" || parts[1] != "Part II" {
		t.Fatalf("parts = %v, want [Part I, Part II]", parts)
	}

	// Sections form the monotonic run 1..3 — the Schedule's 1./2. are NOT sections.
	var secs []string
	collect(roots, "section", &secs)
	if len(secs) != 3 || secs[0] != "Section 1" || secs[1] != "Section 2" || secs[2] != "Section 3" {
		t.Fatalf("sections = %v, want [Section 1, Section 2, Section 3]", secs)
	}

	// Schedule is captured as its own node.
	var scheds []string
	collect(roots, "schedule", &scheds)
	if len(scheds) != 1 {
		t.Fatalf("schedules = %v, want one", scheds)
	}

	// Nesting + citation paths.
	if s := findByPath(roots, "part-i/section-1"); s == nil {
		t.Fatal("missing part-i/section-1")
	}
	if s := findByPath(roots, "part-ii/section-3"); s == nil {
		t.Fatal("section 3 not under Part II (path part-ii/section-3)")
	}
	// Section 1 has two subsections (inline (1) + standalone (2)).
	s1 := findByPath(roots, "part-i/section-1")
	var subs []string
	collect(s1.Children, "subsection", &subs)
	if len(subs) != 2 || subs[0] != "(1)" || subs[1] != "(2)" {
		t.Fatalf("section 1 subsections = %v, want [(1) (2)]", subs)
	}
	// Section 2 → subsection (1) → paragraphs (a),(b).
	para := findByPath(roots, "part-i/section-2/subsection-1/paragraph-a")
	if para == nil || para.Kind != "paragraph" {
		t.Fatal("missing paragraph at part-i/section-2/subsection-1/paragraph-a")
	}
}

// Older AGC PDFs render the enacting clause in small caps that flatten to
// mixed case ("enActeD by"); the TOC must still be cut so the body sections
// parse and body subsections do not pile onto the last TOC section.
func TestParse_smallCapsEnactingCutsTOC(t *testing.T) {
	const act = `Laws of Malaysia
ARRANGEMENT OF SECTIONS
1. Short title
2. Powers
enActeD by the Parliament of Malaysia as follows:
1. (1) This Act may be cited as the Test Act.
(2) It binds the Government.
2. The powers clause.`
	roots, err := mylaw.Parse(act)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	s1 := findByPath(roots, "section-1")
	if s1 == nil {
		t.Fatal("no section-1 (TOC not cut?)")
	}
	var subs []string
	collect(s1.Children, "subsection", &subs)
	if len(subs) != 2 {
		t.Fatalf("section 1 subsections = %v, want [(1) (2)] — body parsed, TOC cut", subs)
	}
}

// Roman (i)/(ii) are subparagraphs nested under their alphabetic paragraph,
// not sibling paragraphs; a 4-digit year parenthetical is not a subsection.
func TestParse_subparagraphsAndYearGuard(t *testing.T) {
	const act = `enActeD by the Parliament as follows:
1. (1) A person may—
(a) do the first thing;
(b) do the second thing, namely—
(i) the first way; and
(ii) the second way.
(2) The second subsection.
(1965) A stray year reference, not a subsection.`
	roots, err := mylaw.Parse(act)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// (i),(ii) nest under paragraph (b).
	if findByPath(roots, "section-1/subsection-1/paragraph-b/subparagraph-i") == nil {
		t.Fatal("roman (i) should nest under paragraph (b)")
	}
	if findByPath(roots, "section-1/subsection-1/paragraph-b/subparagraph-ii") == nil {
		t.Fatal("roman (ii) should nest under paragraph (b)")
	}
	// subsection (1) has exactly two direct paragraphs (a),(b) — romans are nested.
	sub := findByPath(roots, "section-1/subsection-1")
	direct := 0
	for _, c := range sub.Children {
		if c.Kind == "paragraph" {
			direct++
		}
	}
	if direct != 2 {
		t.Fatalf("subsection-1 direct paragraphs = %d, want 2 (romans nested under (b))", direct)
	}
	// Year guard: no subsection labelled (1965).
	var subs []string
	collect(roots, "subsection", &subs)
	for _, s := range subs {
		if s == "(1965)" {
			t.Fatal("(1965) was misparsed as a subsection")
		}
	}
}

// Binding MY text with no Part/Section structure still yields one chunkable
// section via the fallback (so it is not silently dropped from the index);
// blank text yields no junk section.
func TestParse_fullTextFallback(t *testing.T) {
	roots, err := mylaw.Parse("Organisation Structure\n\nThe Commission consists of the following divisions and units.")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(roots) != 1 || roots[0].Kind != "section" || roots[0].CitationPath != "fulltext" {
		t.Fatalf("fallback roots = %+v, want one full-text section", roots)
	}

	roots, err = mylaw.Parse("   ")
	if err != nil || roots != nil {
		t.Fatalf("Parse(whitespace) = %v, %v, want nil, nil", roots, err)
	}
}

// uniqueSeg guarantees distinct citation paths even if a source repeats a label.
func TestParse_uniquePaths(t *testing.T) {
	roots, err := mylaw.Parse(testAct)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	seen := map[string]bool{}
	var walk func([]*law.Node)
	walk = func(nodes []*law.Node) {
		for _, n := range nodes {
			if seen[n.CitationPath] {
				t.Fatalf("duplicate citation path: %s", n.CitationPath)
			}
			seen[n.CitationPath] = true
			walk(n.Children)
		}
	}
	walk(roots)
}

// TestParse_citationPathShape pins the citation-path shape the brief calls
// out for a nested Act: part/section/subsection segments joined by "/", each
// lowercase-slug labelled.
func TestParse_citationPathShape(t *testing.T) {
	const act = `enActeD by the Parliament as follows:
PART I
PRELIMINARY
1. (1) This Act may be cited as the Test Act 2026.`
	roots, err := mylaw.Parse(act)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	sub := findByPath(roots, "part-i/section-1/subsection-1")
	if sub == nil {
		t.Fatal("missing part-i/section-1/subsection-1")
	}
	if sub.Kind != "subsection" || sub.Label != "(1)" {
		t.Fatalf("part-i/section-1/subsection-1 = %+v, want kind=subsection label=(1)", sub)
	}
	// Ancestors carry the expected prefix segments in order.
	sec := findByPath(roots, "part-i/section-1")
	if sec == nil || sec.Kind != "section" {
		t.Fatal("missing part-i/section-1 ancestor")
	}
	part := findByPath(roots, "part-i")
	if part == nil || part.Kind != "part" {
		t.Fatal("missing part-i ancestor")
	}
}
