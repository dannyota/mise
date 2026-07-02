// Package vnlaw parses Vietnamese legal-document text into the shared
// law.Node hierarchy (Phần > Chương > Mục > Điều > Khoản > Điểm, plus Phụ lục
// appendices). It offers two entry points over the two shapes VN law sources
// deliver: Parse over extracted Markdown text (a deterministic line-by-line
// state machine — patterns.go/classify.go/tree.go/fallback.go), and ParseTree
// over a VBPL provision_tree_json payload (provisiontree.go). Both are pure,
// local, and free of AI/network calls; see danny.vn/mise/pkg/parse/quality
// for the companion extraction-quality gate.
package vnlaw

import (
	"slices"
	"strings"

	"danny.vn/mise/pkg/parse/law"
)

// Parse parses Vietnamese legal Markdown text (as emitted by the DOCX/PDF
// extractors) into a tree of law.Nodes. It is a deterministic line-by-line
// state machine — no AI, no I/O, never fails: unparseable input degrades to a
// whole-document fallback node rather than an error.
//
// The parser is engine-uniform: it classifies every line on its own merits and
// does NOT depend on the extractor inserting blank lines before headings. This
// matters because clauses and points are written as "1." and "a)" (not the
// words "Khoản"/"Điểm"), so they never get a preceding blank line — a
// block-splitting parser silently drops all of them. Khoản/Điểm are only
// recognised inside an open Điều, so numbered lists in a preamble or a table do
// not masquerade as clauses.
func Parse(markdown string) ([]*law.Node, error) {
	// Normalise no-break spaces to plain spaces so heading patterns match: real
	// DOCX writes e.g. "Chương II" with a no-break space, which \s rejects.
	markdown = strings.ReplaceAll(markdown, "\u00a0", " ") // NBSP
	markdown = strings.ReplaceAll(markdown, "\u202f", " ") // narrow NBSP
	roots := buildTree(markdown)
	if len(roots) == 0 && !supplementOnlyText(markdown) {
		if outline := buildNumberedOutlineTree(markdown); len(outline) > 0 {
			return outline, nil
		}
		return buildWholeDocumentFallback(markdown), nil
	}
	return roots, nil
}

// frame holds the construction context for one open node on the parse stack.
// node is the heap-allocated law.Node already appended to its parent's
// Children; because Children holds *law.Node (not law.Node by value), this
// pointer stays valid for the lifetime of the parse regardless of any later
// slice growth/reallocation on any ancestor's Children.
type frame struct {
	kind          blockKind
	citationPath  string
	legacyOutline bool
	explicitDieu  bool
	node          *law.Node // nil for the root sentinel
}

// treeBuilder holds the mutable state threaded through buildTree's line scan.
type treeBuilder struct {
	roots         []*law.Node
	stack         []frame
	counters      map[blockKind]int
	pathCounts    map[string]int
	pendingTitle  bool // top structural node is awaiting its title line
	inQuotedBlock bool
}

// buildTree assembles the Node tree by scanning markdown line by line. A
// heading of level L pops all open frames of level >= L, then pushes itself.
// Text lines attach to the current node (or become a structural node's title
// when one is awaiting it). Blank lines are separators and are ignored.
func buildTree(markdown string) []*law.Node {
	b := &treeBuilder{
		stack:      []frame{{kind: bkText}}, // sentinel
		counters:   make(map[blockKind]int),
		pathCounts: make(map[string]int),
	}
	for raw := range strings.SplitSeq(markdown, "\n") {
		b.consumeLine(raw)
	}
	return b.roots
}

// consumeLine classifies one raw input line and either attaches it as text to
// the current node or opens a new structural node for it.
func (b *treeBuilder) consumeLine(raw string) {
	rawLine := strings.TrimSpace(raw)
	if rawLine == "" {
		return
	}
	line := stripMDEmphasis(rawLine) // unwrap MarkItDown's "**heading**" before use
	t := b.classify(rawLine, line)
	if t.kind == bkText {
		b.appendText(line, rawLine)
		return
	}
	b.appendNode(t)
	b.inQuotedBlock = updateQuotedBlock(b.inQuotedBlock, rawLine)
}

// classify determines the token a line represents, honouring quoted-amendment
// suppression and the "lost article heading" promotion.
func (b *treeBuilder) classify(rawLine, line string) token {
	if b.inQuotedBlock || startsWithOpeningQuote(rawLine) {
		return token{kind: bkText, body: line}
	}
	if t, ok := numericArticleToken(rawLine); ok && shouldPromoteLostArticle(rawLine, b.stack) {
		return t
	}
	return classifyLine(rawLine, b.canStartClause(), activeExplicitArticle(b.stack))
}

// canStartClause reports whether an open Điều/Khoản (or legacy outline
// section) is on the stack, so a following "1."/"a)" line reads as a clause
// rather than as body text or a numbered-outline reference.
func (b *treeBuilder) canStartClause() bool {
	for i := range b.stack {
		if b.stack[i].kind == bkDieu || b.stack[i].kind == bkKhoan || b.stack[i].legacyOutline {
			return true
		}
	}
	return false
}

// appendText attaches a text-classified line to the current node: as its
// pending title, its first content line, or a continuation of its content.
// Text above the first heading (the preamble) is dropped.
func (b *treeBuilder) appendText(line, rawLine string) {
	top := b.stack[len(b.stack)-1]
	if top.node == nil {
		b.inQuotedBlock = updateQuotedBlock(b.inQuotedBlock, rawLine)
		return // stray text above the first heading (preamble) — drop
	}
	switch {
	case b.pendingTitle:
		top.node.Heading = line
		b.pendingTitle = false
	case top.node.Content == "":
		top.node.Content = line
	default:
		top.node.Content += "\n" + line
	}
	b.inQuotedBlock = updateQuotedBlock(b.inQuotedBlock, rawLine)
}

// appendNode closes out any frames the new structural token supersedes, then
// opens a node for it as a child of whatever remains on top of the stack.
func (b *treeBuilder) appendNode(t token) {
	b.pendingTitle = false
	b.popTo(t.kind)

	parent := b.stack[len(b.stack)-1]
	ord, label, citPath := b.identify(t, parent)

	ch := b.childrenOf(parent)
	node := &law.Node{
		Kind: kindName(t.kind), Ordinal: ord, Label: label,
		Heading: t.heading, Content: t.body, CitationPath: citPath,
	}
	*ch = append(*ch, node)
	b.stack = append(b.stack, frame{
		kind: t.kind, citationPath: citPath,
		legacyOutline: t.legacyOutline, explicitDieu: t.explicitDieu,
		node: node,
	})

	if isStructural(t.kind) && t.heading == "" {
		b.pendingTitle = true
	}
}

// popTo pops every open frame at or deeper than kind's level, and forgets the
// sibling ordinal counters for anything deeper than kind — a new heading at
// level L starts a fresh count for levels below L.
func (b *treeBuilder) popTo(kind blockKind) {
	for len(b.stack) > 1 && levelOf(b.stack[len(b.stack)-1].kind) >= levelOf(kind) {
		b.stack = b.stack[:len(b.stack)-1]
	}
	for k := range b.counters {
		if levelOf(k) > levelOf(kind) {
			delete(b.counters, k)
		}
	}
}

// identify assigns the sibling ordinal, label, and (parent-qualified, unique)
// citation path for a new node.
func (b *treeBuilder) identify(t token, parent frame) (ord int, label, citPath string) {
	ord = t.ordinal
	if ord == 0 {
		b.counters[t.kind]++
		ord = b.counters[t.kind]
	}
	label = t.label
	if label == "" {
		label = defaultLabel(t.kind, ord)
	}
	citPath = t.pathSeg
	if citPath == "" {
		citPath = defaultPathSegment(t.kind, ord)
	}
	if parent.citationPath != "" {
		citPath = parent.citationPath + "/" + citPath
	}
	citPath = uniqueCitationPath(citPath, b.pathCounts)
	return ord, label, citPath
}

// childrenOf returns the slot the new node should be appended to: the tree
// roots for the sentinel frame, or the frame's own node's Children.
func (b *treeBuilder) childrenOf(f frame) *[]*law.Node {
	if f.node == nil {
		return &b.roots
	}
	return &f.node.Children
}

// ---- stack queries ------------------------------------------------------------

func activeLegacyOutline(stack []frame) bool {
	for i := range stack {
		if stack[i].legacyOutline {
			return true
		}
	}
	return false
}

func activeExplicitArticle(stack []frame) bool {
	for i := range stack {
		if stack[i].kind == bkDieu && stack[i].explicitDieu {
			return true
		}
	}
	return false
}

func currentArticle(stack []frame) *law.Node {
	for _, f := range slices.Backward(stack) {
		if f.kind == bkDieu {
			return f.node
		}
	}
	return nil
}

func hasArticleBodyOrChildren(s *law.Node) bool {
	return s != nil && (strings.TrimSpace(s.Content) != "" || len(s.Children) > 0)
}

func hasArticleContainer(stack []frame) bool {
	for i := range stack {
		if stack[i].kind == bkChuong || stack[i].kind == bkMuc {
			return true
		}
	}
	return false
}

// looksLikeLostArticleHeading reports whether a "N. heading" line reads like a
// promoted article title rather than an ordinary numbered clause: either it
// matches one of the fixed statutory heading phrases, or it is short and
// carries no clause-ending punctuation.
func looksLikeLostArticleHeading(clean string) bool {
	m := numericHeadingRe.FindStringSubmatch(clean)
	if m == nil {
		return false
	}
	heading := strings.TrimSpace(m[2])
	if isArticleLikeNumericHeading(clean) {
		return true
	}
	if len([]rune(heading)) > 120 {
		return false
	}
	if strings.ContainsAny(heading, ":;") || strings.HasSuffix(heading, ".") {
		return false
	}
	return true
}

// shouldPromoteLostArticle reports whether a "1. heading" line should be
// promoted to a new Điều rather than read as clause 1 of the current article:
// only ever for the first ("1.") clause, never inside a quoted amendment or a
// legacy outline section, and only once the current article (if any) already
// has a body or a container (Chương/Mục) is open to receive a new sibling.
func shouldPromoteLostArticle(line string, stack []frame) bool {
	clean := stripMDStructuralMarkers(line)
	m := numericHeadingRe.FindStringSubmatch(clean)
	if m == nil || m[1] != "1" || activeLegacyOutline(stack) || startsWithOpeningQuote(line) {
		return false
	}
	if !looksLikeLostArticleHeading(clean) {
		return false
	}

	article := currentArticle(stack)
	if article != nil {
		return hasArticleContainer(stack) && hasArticleBodyOrChildren(article)
	}

	top := stack[len(stack)-1]
	return top.kind == bkChuong || top.kind == bkMuc
}
