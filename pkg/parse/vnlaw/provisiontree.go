package vnlaw

import (
	"encoding/json"
	"errors"
	"html"
	"regexp"
	"strconv"
	"strings"

	"danny.vn/mise/pkg/parse/law"
)

// ErrInvalidProvisionTree reports that a ParseTree payload is neither a bare
// VBPL provision-tree JSON array nor a {"data": [...]} envelope around one.
var ErrInvalidProvisionTree = errors.New("vnlaw: payload is not a VBPL provision tree")

// ErrEmptyProvisionTree reports that a ParseTree payload decoded cleanly but
// held zero nodes.
var ErrEmptyProvisionTree = errors.New("vnlaw: VBPL provision tree is empty")

// vbplProvisionNode mirrors the VBPL provision_tree_json payload shape: a
// recursive Phần/Chương/Mục/Điều/Khoản/Điểm tree keyed by a source-native
// node id/key and a numeric provision-type code.
type vbplProvisionNode struct {
	ID         string               `json:"id"`
	Key        string               `json:"key"`
	Title      string               `json:"title"`
	PType      int16                `json:"ptype"`
	Level      string               `json:"level"`
	OrderIndex int                  `json:"orderIndex"`
	Content    vbplProvisionContent `json:"content"`
	Children   []vbplProvisionNode  `json:"children"`
}

// vbplProvisionContent is a VBPL provision node's own title/content pair.
type vbplProvisionContent struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

// ParseTree parses a VBPL provision_tree_json payload — either a bare JSON
// array of nodes, or an envelope object such as {"success":true,"data":[...]}
// — into the same law.Node hierarchy Parse produces from Markdown.
//
// Unlike Parse (which degrades to a whole-document fallback rather than ever
// failing), ParseTree returns an error when the payload does not decode to
// either shape (ErrInvalidProvisionTree) or decodes to zero nodes
// (ErrEmptyProvisionTree): both mean the source never actually delivered a
// provision tree, so callers should fall back to Parse over the document's
// plain text instead.
//
// A tree whose nodes decode but carry no text content returns (tree, nil) —
// NOT an error. Callers must additionally check len(law.Flatten(roots)) == 0
// and fall back to Parse in that case, or the document's content is silently
// dropped (banhmi signalled this same condition as ok=false).
func ParseTree(payload string) ([]*law.Node, error) {
	nodes, ok := decodeVBPLProvisionTree(payload)
	if !ok {
		return nil, ErrInvalidProvisionTree
	}
	if len(nodes) == 0 {
		return nil, ErrEmptyProvisionTree
	}
	counts := make(map[string]int)
	return buildVBPLTree(nodes, "", counts), nil
}

// decodeVBPLProvisionTree accepts either shape VBPL has been observed to send
// a provision tree in: a bare array, or {"data": [...]}.
func decodeVBPLProvisionTree(payload string) ([]vbplProvisionNode, bool) {
	var nodes []vbplProvisionNode
	if err := json.Unmarshal([]byte(payload), &nodes); err == nil {
		return nodes, true
	}

	var envelope struct {
		Data []vbplProvisionNode `json:"data"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err == nil {
		return envelope.Data, true
	}
	return nil, false
}

// buildVBPLTree converts decoded VBPL nodes into law.Nodes. A node whose
// level/ptype does not map to a known kind is dropped, but its children are
// kept and promoted to the current level (rather than losing that whole
// branch of the tree).
func buildVBPLTree(nodes []vbplProvisionNode, parentPath string, counts map[string]int) []*law.Node {
	out := make([]*law.Node, 0, len(nodes))
	for i := range nodes {
		node := nodes[i]
		kind := vbplKind(node.Level, node.PType)
		if kind == "" {
			out = append(out, buildVBPLTree(node.Children, parentPath, counts)...)
			continue
		}

		label, heading, segment, ordinal, labelVariants := vbplTitleParts(kind, node, i+1)
		path := segment
		if parentPath != "" {
			path = parentPath + "/" + segment
		}
		path = uniqueCitationPath(path, counts)
		n := &law.Node{
			Kind: kind, Ordinal: ordinal, Label: label, Heading: heading,
			Content: vbplOwnContent(node, labelVariants), CitationPath: path,
		}
		n.Children = buildVBPLTree(node.Children, path, counts)
		out = append(out, n)
	}
	return out
}

// vbplKind maps a VBPL node's level (preferred) or numeric ptype to a
// law.Node.Kind string.
func vbplKind(level string, ptype int16) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "part":
		return "phan"
	case "chapter":
		return "chuong"
	case "section":
		return "muc"
	case "article":
		return "dieu"
	case "clause":
		return "khoan"
	case "point":
		return "diem"
	}
	switch ptype {
	case 1:
		return "phan"
	case 2:
		return "chuong"
	case 3, 4:
		return "muc"
	case 5:
		return "dieu"
	case 6:
		return "khoan"
	case 7:
		return "diem"
	default:
		return ""
	}
}

// vbplTitleParts extracts the label/heading/citation-segment/ordinal for a
// node from its own title text, falling back to its sibling position when the
// title does not match the expected "<Prefix> <N>[. heading]" shape.
// variants collects every string form of the label VBPL might have prefixed
// onto the node's own content, for vbplOwnContent to strip.
func vbplTitleParts(
	kind string, node vbplProvisionNode, siblingOrdinal int,
) (label, heading, segment string, ordinal int, variants []string) {
	title := normalizeTreeText(firstNonEmpty(node.Title, node.Content.Title))
	ordinal = siblingOrdinal

	switch kind {
	case "phan":
		label, heading, segment, ordinal = parseVBPLRomanTitle(title, "Phần", "phan", ordinal)
	case "chuong":
		label, heading, segment, ordinal = parseVBPLRomanTitle(title, "Chương", "chuong", ordinal)
	case "muc":
		label, heading, segment, ordinal = parseVBPLRomanTitle(title, "Mục", "muc", ordinal)
	case "dieu":
		label, heading, segment, ordinal = parseVBPLArticleTitle(title, ordinal)
	case "khoan":
		label, heading, segment, ordinal = parseVBPLClauseTitle(title, ordinal)
	case "diem":
		label, heading, segment, ordinal = parseVBPLPointTitle(title, ordinal)
	}

	if label == "" {
		label = defaultVBPLLabel(kind, ordinal)
	}
	if segment == "" {
		segment = kind + "-" + strconv.Itoa(ordinal)
	}
	variants = append(variants, title, label)
	switch kind {
	case "khoan":
		variants = append(variants, strings.TrimSuffix(label, "."), label)
	case "diem":
		variants = append(variants, "Điểm "+strings.TrimSuffix(label, ")"), label)
	case "dieu", "phan", "chuong", "muc":
		variants = append(variants, label+".")
	}
	return label, strings.Trim(heading, " .\t\r\n"), segment, ordinal, variants
}

var (
	vbplArticleTitleRe = regexp.MustCompile(`^Điều\s+(\d+[A-Za-zĐđ]?)(?:[.\s]+(.*))?$`)
	vbplClauseTitleRe  = regexp.MustCompile(`^(?:Khoản\s+)?(\d+)(?:[.\s]+(.*))?$`)
	vbplPointTitleRe   = regexp.MustCompile(`(?i)^(?:Điểm\s+)?([a-zđ])(?:[\)\.\s]+(.*))?$`)
)

func parseVBPLRomanTitle(
	title, prefix, pathPrefix string, fallbackOrdinal int,
) (label, heading, segment string, ordinal int) {
	ordinal = fallbackOrdinal
	fields := strings.Fields(title)
	if len(fields) >= 2 && strings.EqualFold(fields[0], prefix) {
		raw := strings.Trim(fields[1], ".:")
		label = prefix + " " + raw
		heading = strings.TrimSpace(strings.TrimPrefix(title, label))
		heading = strings.TrimLeft(heading, ".: ")
		if n := romanOrArabicOrdinal(raw); n > 0 {
			ordinal = n
			segment = pathPrefix + "-" + strconv.Itoa(n)
		}
	}
	return label, heading, segment, ordinal
}

func parseVBPLArticleTitle(title string, fallbackOrdinal int) (label, heading, segment string, ordinal int) {
	ordinal = fallbackOrdinal
	if m := vbplArticleTitleRe.FindStringSubmatch(title); m != nil {
		raw := m[1]
		label = "Điều " + raw
		heading = strings.TrimSpace(m[2])
		if n := atoiLeading(raw); n > 0 {
			ordinal = n
		}
		segment = "dieu-" + strings.ToLower(raw)
	}
	return label, heading, segment, ordinal
}

func parseVBPLClauseTitle(title string, fallbackOrdinal int) (label, heading, segment string, ordinal int) {
	ordinal = fallbackOrdinal
	if m := vbplClauseTitleRe.FindStringSubmatch(title); m != nil {
		raw := m[1]
		label = raw + "."
		heading = strings.TrimSpace(m[2])
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			ordinal = n
		}
		segment = "khoan-" + raw
	}
	return label, heading, segment, ordinal
}

func parseVBPLPointTitle(title string, fallbackOrdinal int) (label, heading, segment string, ordinal int) {
	ordinal = fallbackOrdinal
	if m := vbplPointTitleRe.FindStringSubmatch(title); m != nil {
		raw := strings.ToLower(m[1])
		label = raw + ")"
		heading = strings.TrimSpace(m[2])
		if n := diemLetterOrdinal(raw); n > 0 {
			ordinal = n
		}
		segment = "diem-" + raw
	}
	return label, heading, segment, ordinal
}

func romanOrArabicOrdinal(raw string) int {
	raw = strings.ToUpper(strings.TrimSpace(raw))
	if raw == "" {
		return 0
	}
	if n, err := strconv.Atoi(raw); err == nil {
		return n
	}
	return romanToInt(raw)
}

func defaultVBPLLabel(kind string, ordinal int) string {
	switch kind {
	case "phan":
		return "Phần " + strconv.Itoa(ordinal)
	case "chuong":
		return "Chương " + strconv.Itoa(ordinal)
	case "muc":
		return "Mục " + strconv.Itoa(ordinal)
	case "dieu":
		return "Điều " + strconv.Itoa(ordinal)
	case "khoan":
		return strconv.Itoa(ordinal) + "."
	case "diem":
		return "Điểm " + strconv.Itoa(ordinal)
	default:
		return strconv.Itoa(ordinal)
	}
}

// vbplOwnContent returns a node's own body text: its raw HTML content with
// every child's own (recursively rendered) text removed, then with the
// leading label ("Điều 1", "1.", "a)", ...) stripped.
func vbplOwnContent(node vbplProvisionNode, labelVariants []string) string {
	text := normalizeTreeText(htmlToTreeText(node.Content.Content))
	for _, child := range node.Children {
		childText := normalizeTreeText(htmlToTreeText(child.Content.Content))
		if childText == "" {
			continue
		}
		text = normalizeTreeText(strings.Replace(text, childText, "", 1))
	}
	return stripTreePrefixes(text, labelVariants)
}

var (
	vbplBreakRe = regexp.MustCompile(`(?i)<\s*(br|/p|/div|/li)\s*/?>`)
	vbplTagRe   = regexp.MustCompile(`<[^>]+>`)
)

// htmlToTreeText renders a VBPL content-HTML fragment down to plain text:
// line breaks become newlines, remaining tags become spaces, and HTML
// entities are unescaped.
func htmlToTreeText(s string) string {
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = strings.ReplaceAll(s, "\u202f", " ")
	s = vbplBreakRe.ReplaceAllString(s, "\n")
	s = vbplTagRe.ReplaceAllString(s, " ")
	return html.UnescapeString(s)
}

// stripTreePrefixes removes whichever label variant text begins with (VBPL's
// own content field repeats the node's label/title before its actual body).
func stripTreePrefixes(text string, variants []string) string {
	text = normalizeTreeText(text)
	for _, variant := range variants {
		v := normalizeTreeText(htmlToTreeText(variant))
		if v == "" || !strings.HasPrefix(text, v) {
			continue
		}
		text = strings.TrimSpace(strings.TrimPrefix(text, v))
		text = strings.TrimLeft(text, ".:) ")
		return normalizeTreeText(text)
	}
	return normalizeTreeText(text)
}

// normalizeTreeText collapses all whitespace runs to single spaces and trims
// the ends.
func normalizeTreeText(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v := strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
