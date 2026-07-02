package vnlaw

import (
	"strings"

	"danny.vn/mise/pkg/parse/law"
)

// buildNumberedOutlineTree handles older narrative circulars whose only
// citable structure is a numbered outline ("1.", "1.1.", "2.", ...) rather
// than statutory Điều/Khoản headings. It only fires when the document has at
// least two such lines — a single stray "1." is not outline structure.
func buildNumberedOutlineTree(markdown string) []*law.Node {
	if countNumberedOutlineLines(markdown) < 2 {
		return nil
	}

	var roots []*law.Node
	pathCounts := make(map[string]int)
	lastTop := -1
	lastChild := -1

	appendContinuation := func(line string) {
		switch {
		case lastTop >= 0 && lastChild >= 0:
			appendNodeContinuation(roots[lastTop].Children[lastChild], line)
		case lastTop >= 0:
			appendNodeContinuation(roots[lastTop], line)
		}
	}

	for raw := range strings.SplitSeq(markdown, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "|") {
			continue
		}
		line = stripMDEmphasis(line)
		clean := stripMDStructuralMarkers(line)
		m := numberedOutlineRe.FindStringSubmatch(clean)
		if m == nil {
			if lastTop >= 0 {
				appendContinuation(line)
			}
			continue
		}

		key, body := m[1], strings.TrimSpace(m[2])
		depth := strings.Count(key, ".") + 1
		if depth == 1 || lastTop < 0 {
			roots = append(roots, newOutlineRoot(key, body, pathCounts))
			lastTop, lastChild = len(roots)-1, -1
			continue
		}

		roots[lastTop].Children = append(roots[lastTop].Children, newOutlineChild(roots[lastTop], key, body, pathCounts))
		lastChild = len(roots[lastTop].Children) - 1
	}

	return roots
}

func appendNodeContinuation(n *law.Node, line string) {
	if n.Content == "" {
		n.Content = line
	} else {
		n.Content += "\n" + line
	}
}

func newOutlineRoot(key, body string, pathCounts map[string]int) *law.Node {
	seg := strings.ReplaceAll(key, ".", "-")
	path := uniqueCitationPath("dieu-outline-"+seg, pathCounts)
	return &law.Node{
		Kind: "dieu", Ordinal: outlineOrdinal(key), Label: key + ".",
		Content: body, CitationPath: path,
	}
}

func newOutlineChild(parent *law.Node, key, body string, pathCounts map[string]int) *law.Node {
	seg := strings.ReplaceAll(key, ".", "-")
	path := uniqueCitationPath(parent.CitationPath+"/khoan-outline-"+seg, pathCounts)
	return &law.Node{
		Kind: "khoan", Ordinal: outlineOrdinal(key), Label: key + ".",
		Content: body, CitationPath: path,
	}
}

// buildWholeDocumentFallback is the last resort for text with no recognisable
// statutory or outline structure at all: it keeps the whole cleaned document
// as one node, provided there is enough of it to be worth keeping (guards
// against turning a short supplement/report fragment into a synthetic
// article).
func buildWholeDocumentFallback(markdown string) []*law.Node {
	content := wholeDocumentFallbackContent(markdown)
	if !shouldUseWholeDocumentFallback(content) {
		return nil
	}
	return []*law.Node{{
		Kind: "dieu", Ordinal: 1, Label: "Toàn văn", Content: content, CitationPath: "toan-van",
	}}
}

// wholeDocumentFallbackContent strips blank lines, table separators, ATX
// heading markers, and Markdown emphasis, leaving plain running text.
func wholeDocumentFallbackContent(markdown string) string {
	lines := make([]string, 0)
	for raw := range strings.SplitSeq(markdown, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || isMarkdownTableSeparator(line) {
			continue
		}
		line = stripMDEmphasis(line)
		for strings.HasPrefix(line, "#") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func isMarkdownTableSeparator(line string) bool {
	line = strings.TrimSpace(line)
	if !strings.Contains(line, "-") {
		return false
	}
	for _, r := range line {
		switch r {
		case '|', '-', ':', ' ':
			continue
		default:
			return false
		}
	}
	return true
}

// shouldUseWholeDocumentFallback gates the whole-document fallback on a
// minimum size: short fragments (report snippets, form labels) are dropped
// rather than becoming a spurious "Toàn văn" article.
func shouldUseWholeDocumentFallback(content string) bool {
	if len([]rune(content)) < 300 {
		return false
	}
	lines := 0
	for raw := range strings.SplitSeq(content, "\n") {
		if strings.TrimSpace(raw) != "" {
			lines++
		}
	}
	return lines >= 4
}

func countNumberedOutlineLines(markdown string) int {
	count := 0
	for raw := range strings.SplitSeq(markdown, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "|") {
			continue
		}
		clean := stripMDStructuralMarkers(stripMDEmphasis(line))
		if numberedOutlineRe.MatchString(clean) {
			count++
		}
	}
	return count
}

func outlineOrdinal(key string) int {
	parts := strings.Split(key, ".")
	if len(parts) == 0 {
		return 0
	}
	return atoiLeading(parts[len(parts)-1])
}

// supplementOnlyText sniffs a document's opening lines to tell a real
// statutory/regulatory text apart from a supplement — a report, form, or
// annex that merely uses numbered lines and must not be parsed as if it were
// legal structure (which would fabricate an Điều/Khoản tree out of a report's
// paragraph numbering).
func supplementOnlyText(text string) bool {
	meaningful := 0
	for raw := range strings.SplitSeq(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "|") {
			continue
		}
		meaningful++
		line = strings.TrimSpace(strings.Trim(line, "*_# "))
		folded := strings.ToLower(line)
		if strings.HasPrefix(folded, "điều ") ||
			strings.HasPrefix(folded, "chương ") ||
			strings.HasPrefix(folded, "thông tư") ||
			strings.HasPrefix(folded, "nghị định") ||
			strings.HasPrefix(folded, "quyết định") {
			return false
		}
		if strings.HasPrefix(folded, "phụ lục") ||
			strings.HasPrefix(folded, "phu luc") ||
			strings.HasPrefix(folded, "mẫu số") ||
			strings.HasPrefix(folded, "mau so") ||
			strings.Contains(folded, "báo cáo tình hình") ||
			strings.Contains(folded, "bao cao tinh hinh") {
			return true
		}
		if meaningful >= 8 {
			return false
		}
	}
	return false
}
