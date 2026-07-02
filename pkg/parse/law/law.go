// Package law defines the shared, jurisdiction-agnostic legal-document tree
// and citation-flattening logic. Every jurisdiction's structure parser
// (vnlaw today; others later) produces this tree; downstream chunking/embed
// code consumes it through Flatten alone, never a parser-specific type.
package law

import "strings"

// Node is one node in a parsed legal-document hierarchy, e.g. Vietnam's
// Phần > Chương > Mục > Điều > Khoản > Điểm (+ Phụ lục appendices).
//
// Kind is a jurisdiction-specific lowercase tag (vnlaw emits "phan", "chuong",
// "muc", "dieu", "khoan", "diem", "phuluc"). Label is the human-facing
// citation label ("Điều 7", "Khoản 2", "Điểm a"). Heading is the title text
// that follows the label on the same line, when the node carries one.
// Content is the node's own body text, excluding its children's text.
// CitationPath is the stable dedup/citation key ("dieu-7/khoan-2").
type Node struct {
	Kind, Label, Heading, Content, CitationPath string
	Ordinal                                     int
	Children                                    []*Node
}

// FlatSection is one citable unit of text: a single Node's own Content,
// addressed by its CitationPath, with the full ancestor heading trail spelled
// out in HeadingPath for humans (search results, citations, prompts).
type FlatSection struct {
	CitationPath, HeadingPath, Body string
}

// Flatten walks nodes depth-first and returns one FlatSection per node whose
// Content is non-empty (nodes that only group children, such as a Chương with
// no body text of its own, contribute no FlatSection but their descendants
// still do). HeadingPath joins every ancestor's, and the node's own,
// "Label — Heading" — or bare Label when Heading is empty — with " > ",
// root first.
func Flatten(nodes []*Node) []FlatSection {
	var out []FlatSection
	flattenInto(&out, nodes, "")
	return out
}

// flattenInto appends the flattened sections for nodes (and their
// descendants) to out, given the joined heading path of their parent.
func flattenInto(out *[]FlatSection, nodes []*Node, ancestry string) {
	for _, n := range nodes {
		path := headingLabel(n)
		if ancestry != "" {
			path = ancestry + " > " + path
		}
		if strings.TrimSpace(n.Content) != "" {
			*out = append(*out, FlatSection{
				CitationPath: n.CitationPath,
				HeadingPath:  path,
				Body:         n.Content,
			})
		}
		flattenInto(out, n.Children, path)
	}
}

// headingLabel renders a single node's own heading-path segment.
func headingLabel(n *Node) string {
	if n.Heading == "" {
		return n.Label
	}
	return n.Label + " — " + n.Heading
}
