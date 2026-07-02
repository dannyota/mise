// Package mylaw parses Malaysian legal-document text into the shared
// law.Node hierarchy: Part > Chapter/Division > Section > Subsection >
// Paragraph, plus Schedule appendices. Parse is a deterministic
// line-by-line state machine — no AI, no I/O — over extracted PDF text from
// AGC's "Laws of Malaysia".
package mylaw

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"danny.vn/mise/pkg/parse/law"
)

// Parse parses the text of a Malaysian Act (as extracted from a born-digital
// AGC "Laws of Malaysia" PDF) into the shared law.Node hierarchy, with the
// Malaysian provision hierarchy:
//
//	Part > Chapter/Division > Section > Subsection > Paragraph   (+ Schedule)
//
// It is a deterministic line-by-line state machine — no AI, no I/O, never
// fails: text with no recognisable Part/Section structure degrades to a
// single full-text node rather than an error. The recipe was proven on FSA
// 2013 (17/17 Parts, 281/281 sections, 0 gaps): strip page noise, cut the
// front "Arrangement of Sections" at the enacting clause, accept a Section
// number only in monotonic sequence (so the Schedules' own 1./2./3.
// renumbering and inline cross-references do not masquerade as sections),
// and stop section parsing at the first Schedule.
//
// Structure, numbering, nesting, and citation paths are reliable. Section
// marginal-note TITLES are not recovered here — pdfminer/MarkItDown text
// drops the margin geometry, so high-fidelity titles need a separate
// layout-aware (pdfplumber x-coordinate) pass; Heading is left empty until
// then.
func Parse(text string) ([]*law.Node, error) {
	p := &parser{stack: []frame{{level: -1}}}
	for _, line := range bodyLines(text) {
		p.consume(line)
	}
	roots := p.roots
	if len(roots) == 0 {
		roots = fullTextFallback(text)
	}
	trimContent(roots)
	return roots, nil
}

// fullTextFallback wraps text that has no recognisable Part/Section
// structure (e.g. a flat notice or schedule-only page) in a single section,
// so its text is still chunked and searchable rather than silently dropped.
// It fires only when the state machine above finds nothing.
func fullTextFallback(text string) []*law.Node {
	body := strings.TrimSpace(text)
	if body == "" {
		return nil
	}
	return []*law.Node{{
		Kind: "section", Ordinal: 1, Label: "Full text",
		Content: body, CitationPath: "fulltext",
	}}
}

// trimContent trims every node's own Content in place, recursively — a
// safety net beyond the line-level trimming already done while accumulating
// content in appendContent.
func trimContent(nodes []*law.Node) {
	for _, n := range nodes {
		n.Content = strings.TrimSpace(n.Content)
		trimContent(n.Children)
	}
}

// ---- parse stack --------------------------------------------------------

// frame holds the construction context for one open node on the parse
// stack. node is nil for the root sentinel; because law.Node.Children holds
// *law.Node (not law.Node by value), this pointer stays valid for the life
// of the parse regardless of later slice growth on any ancestor's Children.
type frame struct {
	level int
	node  *law.Node
}

// parser is the state machine behind Parse.
type parser struct {
	roots    []*law.Node
	stack    []frame
	lastSec  int    // highest Section number accepted (sections are a 1..N run)
	lastPara string // last alphabetic paragraph label, to disambiguate roman (i)/(v)/(x)
	inSched  bool   // once a Schedule starts, stop section parsing
}

// ---- patterns (anchored at line start) -----------------------------------

var (
	// Patterns are case-insensitive where born-digital AGC PDFs render headings
	// in small caps that pdfminer/MarkItDown flattens to mixed case (e.g.
	// "enActeD by").
	pageNoiseRe = regexp.MustCompile(`(?i)^(laws of malaysia|act\s+\d+[a-z]?)$`)
	enactingRe  = regexp.MustCompile(`(?i)enacted by`)
	partRe      = regexp.MustCompile(`(?i)^PART\s+([IVXLC]+)$`)
	chapterRe   = regexp.MustCompile(`(?i)^(?:Division|Chapter)\s+(\d+)$`)
	sectionRe   = regexp.MustCompile(`^(\d+[A-Z]*)\.(?:\s+(.*))?$`)
	// Subsection numbers are 1–3 digits (+ optional letter, e.g. 2A); a
	// 4-digit parenthetical is a year cross-reference, not a subsection label.
	subsecRe = regexp.MustCompile(`^\((\d{1,3}[A-Z]?)\)\s+(.*)$`)
	paraRe   = regexp.MustCompile(`^\(([a-z]{1,3})\)\s+(.*)$`)
	// scheduleRe recognises an ordinal-word schedule heading ("FIRST SCHEDULE")
	// or a numbered one ("SCHEDULE 2"); split across lines to respect lll.
	scheduleRe = regexp.MustCompile(`(?i)^(?:(?:` +
		`FIRST|SECOND|THIRD|FOURTH|FIFTH|SIXTH|SEVENTH|EIGHTH|NINTH|TENTH|ELEVENTH|TWELFTH` +
		`)\s+SCHEDULE|SCHEDULE\s+\d+)\b`)
)

const (
	levelPart = iota
	levelChapter
	levelSection
	levelSubsection
	levelParagraph
	levelSubparagraph
)

// bodyLines strips per-page noise and cuts the front "Arrangement of
// Sections" table of contents at the enacting clause, returning the body's
// non-empty lines.
func bodyLines(text string) []string {
	text = strings.ReplaceAll(text, "\u00a0", " ") // NBSP
	text = strings.ReplaceAll(text, "\u202f", " ") // narrow NBSP
	raw := strings.Split(text, "\n")
	start := 0
	for i, ln := range raw {
		if enactingRe.MatchString(ln) {
			start = i + 1
			break
		}
	}
	var out []string
	for _, ln := range raw[start:] {
		t := strings.TrimSpace(ln)
		if t == "" || isPageNoise(t) {
			continue
		}
		out = append(out, t)
	}
	return out
}

func isPageNoise(t string) bool {
	if pageNoiseRe.MatchString(t) {
		return true
	}
	if _, err := strconv.Atoi(t); err == nil { // bare page number
		return true
	}
	return false
}

// ---- state machine --------------------------------------------------------

func (p *parser) consume(line string) {
	switch {
	case scheduleRe.MatchString(line):
		p.inSched = true
		p.push("schedule", line, levelPart, slug(line))
		return
	case p.inSched:
		p.appendContent(line) // schedules: keep flat content, don't parse their numbering
		return
	}

	if m := partRe.FindStringSubmatch(line); m != nil {
		p.push("part", "Part "+m[1], levelPart, "part-"+strings.ToLower(m[1]))
		return
	}
	if m := chapterRe.FindStringSubmatch(line); m != nil {
		p.push("chapter", "Chapter "+m[1], levelChapter, "chapter-"+m[1])
		return
	}
	if m := sectionRe.FindStringSubmatch(line); m != nil && p.acceptSection(m[1]) {
		p.lastPara = ""
		p.push("section", "Section "+m[1], levelSection, "section-"+strings.ToLower(m[1]))
		if rest := strings.TrimSpace(m[2]); rest != "" {
			p.consumeInline(rest) // e.g. "7. (1) ..." → subsection (1) ...
		}
		return
	}
	if m := subsecRe.FindStringSubmatch(line); m != nil && p.inSection() {
		p.lastPara = ""
		p.push("subsection", "("+m[1]+")", levelSubsection, "subsection-"+strings.ToLower(m[1]))
		p.appendContent(m[2])
		return
	}
	if m := paraRe.FindStringSubmatch(line); m != nil && p.inSection() {
		tok := m[1]
		// A roman (i)/(ii)/… is a subparagraph nested under its alphabetic
		// paragraph, not a sibling paragraph; alpha (a)/(b)/… is a paragraph.
		if p.isSubparagraph(tok) {
			p.push("paragraph", "("+tok+")", levelSubparagraph, "subparagraph-"+tok)
		} else {
			p.push("paragraph", "("+tok+")", levelParagraph, "paragraph-"+tok)
			p.lastPara = tok
		}
		p.appendContent(m[2])
		return
	}
	p.appendContent(line)
}

// isSubparagraph decides whether a lowercase parenthetical like (i) is a
// roman subparagraph rather than an alphabetic paragraph. Multi-letter
// romans (ii, iv, …) are unambiguous; the single ambiguous letters i/v/x are
// alphabetic paragraphs only when they continue the a,b,c… run
// (…h→(i), …u→(v), …w→(x)).
func (p *parser) isSubparagraph(tok string) bool {
	if !isRomanLower(tok) {
		return false
	}
	if len(tok) > 1 {
		return true
	}
	switch tok {
	case "i":
		return p.lastPara != "h"
	case "v":
		return p.lastPara != "u"
	case "x":
		return p.lastPara != "w"
	}
	return false
}

// consumeInline handles the remainder after a section number on the same line.
func (p *parser) consumeInline(rest string) {
	if m := subsecRe.FindStringSubmatch(rest); m != nil {
		p.push("subsection", "("+m[1]+")", levelSubsection, "subsection-"+strings.ToLower(m[1]))
		p.appendContent(m[2])
		return
	}
	p.appendContent(rest)
}

func (p *parser) acceptSection(num string) bool {
	if p.inSched {
		return false
	}
	base := leadingInt(num)
	hasSuffix := base > 0 && len(strconv.Itoa(base)) < len(num)
	if base == p.lastSec+1 || (hasSuffix && base == p.lastSec) {
		p.lastSec = base
		return true
	}
	return false
}

// push pops the stack to the new node's parent (by level), appends the node,
// and makes it the open frame. CitationPath is the parent path plus this
// node's segment.
func (p *parser) push(kind, label string, level int, seg string) {
	for len(p.stack) > 1 && p.stack[len(p.stack)-1].level >= level {
		p.stack = p.stack[:len(p.stack)-1]
	}
	parent := p.stack[len(p.stack)-1]
	siblings := p.childrenOf(parent)
	ord := 0
	for _, c := range siblings {
		if c.Kind == kind {
			ord++
		}
	}
	seg = uniqueSeg(siblings, seg) // guarantee a unique path even if a label repeats
	path := seg
	if parent.node != nil && parent.node.CitationPath != "" {
		path = parent.node.CitationPath + "/" + seg
	}
	node := &law.Node{Kind: kind, Ordinal: ord + 1, Label: label, CitationPath: path}
	if parent.node == nil {
		p.roots = append(p.roots, node)
	} else {
		parent.node.Children = append(parent.node.Children, node)
	}
	p.stack = append(p.stack, frame{level: level, node: node})
}

// childrenOf returns the slot f's node's children live in: the tree roots
// for the sentinel frame, or the frame's own node's Children.
func (p *parser) childrenOf(f frame) []*law.Node {
	if f.node == nil {
		return p.roots
	}
	return f.node.Children
}

func (p *parser) appendContent(s string) {
	top := p.stack[len(p.stack)-1]
	if top.node == nil {
		return // preamble / stray text before the first heading
	}
	if top.node.Content != "" {
		top.node.Content += "\n"
	}
	top.node.Content += strings.TrimSpace(s)
}

func (p *parser) inSection() bool {
	for _, v := range slices.Backward(p.stack) {
		if v.node != nil && v.node.Kind == "section" {
			return true
		}
	}
	return false
}

// ---- helpers ----------------------------------------------------------------

func leadingInt(s string) int {
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	n, _ := strconv.Atoi(s[:end])
	return n
}

var slugStripRe = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	return strings.Trim(slugStripRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

// romanLowerRe matches a lowercase roman numeral (i..xxxix), used to tell a
// roman subparagraph (i)/(ii)/… from an alphabetic paragraph (a)/(b)/….
var romanLowerRe = regexp.MustCompile(`^(?:x{0,3})(?:ix|iv|v?i{0,3})$`)

func isRomanLower(s string) bool {
	return s != "" && romanLowerRe.MatchString(s)
}

// uniqueSeg returns seg, or seg-2/seg-3/… when a sibling already uses it, so
// every node in siblings has a distinct last path segment (hence a unique
// CitationPath).
func uniqueSeg(siblings []*law.Node, seg string) string {
	taken := func(s string) bool {
		for _, c := range siblings {
			cs := c.CitationPath
			if i := strings.LastIndex(cs, "/"); i >= 0 {
				cs = cs[i+1:]
			}
			if cs == s {
				return true
			}
		}
		return false
	}
	if !taken(seg) {
		return seg
	}
	for n := 2; ; n++ {
		if cand := seg + "-" + strconv.Itoa(n); !taken(cand) {
			return cand
		}
	}
}
