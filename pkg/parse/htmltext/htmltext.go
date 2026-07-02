// Package htmltext extracts plain text from HTML deterministically and
// locally — no model call. It exists for sources that serve the document body
// as server-rendered HTML (vbpl-style detail pages), where the downstream
// legal-structure parsers need a stable line discipline: one line per block
// element, no blank lines, entities decoded.
package htmltext

import (
	"bytes"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

// blockTags are elements whose content renders as its own line: a newline is
// emitted before and after the element. The set covers the block elements the
// line-by-line legal parsers rely on (p, div, li, tr, h1–h6) plus the usual
// HTML flow containers so unexpected markup still breaks cleanly.
var blockTags = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true,
	"caption": true, "dd": true, "div": true, "dl": true, "dt": true,
	"fieldset": true, "figcaption": true, "figure": true, "footer": true,
	"form": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true,
	"h6": true, "header": true, "li": true, "main": true, "nav": true,
	"ol": true, "p": true, "pre": true, "section": true, "table": true,
	"tbody": true, "tfoot": true, "thead": true, "tr": true, "ul": true,
}

// voidBreakTags are void elements that force a line break in place.
var voidBreakTags = map[string]bool{"br": true, "hr": true}

// skipTags are subtrees that contribute no document text.
var skipTags = map[string]bool{
	"head": true, "iframe": true, "noscript": true, "object": true,
	"script": true, "style": true, "template": true,
}

// cellTags are table cells: adjacent cells on the same row are separated by a
// space so their text does not run together.
var cellTags = map[string]bool{"td": true, "th": true}

// Text extracts plain text from HTML. Each block element (p, div, li, tr,
// h1–h6, …) and each <br> becomes its own line; script/style/head subtrees are
// dropped; entities are decoded; whitespace (including NBSP) is collapsed
// within lines and blank lines are removed. The parser is the tolerant HTML5
// x/net/html parser, so malformed markup still yields text.
func Text(b []byte) string {
	root, err := html.Parse(bytes.NewReader(b))
	if err != nil {
		// html.Parse only fails when the reader fails; a bytes.Reader cannot.
		return ""
	}
	var sb strings.Builder
	walk(root, &sb)
	return normalize(sb.String())
}

// walk appends n's text to sb, inserting structural newlines around block
// elements. Text-node whitespace is flattened to plain spaces first so the
// only newlines in sb are structural ones.
func walk(n *html.Node, sb *strings.Builder) {
	if n.Type == html.TextNode {
		sb.WriteString(flattenSpace(n.Data))
		return
	}
	if n.Type == html.ElementNode {
		if skipTags[n.Data] {
			return
		}
		if voidBreakTags[n.Data] {
			sb.WriteByte('\n')
			return
		}
	}
	block := n.Type == html.ElementNode && blockTags[n.Data]
	if block {
		sb.WriteByte('\n')
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, sb)
	}
	if block {
		sb.WriteByte('\n')
	}
	if n.Type == html.ElementNode && cellTags[n.Data] {
		sb.WriteByte(' ')
	}
}

// flattenSpace maps every whitespace rune (tab, newline, NBSP, …) to a plain
// space so source-formatting newlines cannot masquerade as line breaks.
func flattenSpace(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, s)
}

// normalize collapses whitespace runs within each line, trims the line, and
// drops blank lines, yielding newline-joined non-empty lines.
func normalize(raw string) string {
	var lines []string
	for line := range strings.SplitSeq(raw, "\n") {
		if fields := strings.Fields(line); len(fields) > 0 {
			lines = append(lines, strings.Join(fields, " "))
		}
	}
	return strings.Join(lines, "\n")
}
