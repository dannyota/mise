// This file continues parse_test.go: the viSnippet tree-shape assertions and
// citation-path invariants. TestParse_tree is split across three functions
// (chuong I header/articles, its khoan/diem, chuong II) to stay under the
// project's per-function statement limit; each still asserts on the one
// shared parse of viSnippet, just at a different depth.
package vnlaw_test

import (
	"testing"

	"danny.vn/mise/pkg/parse/law"
)

func TestParse_tree_chuongIHeaderAndArticles(t *testing.T) {
	roots := mustParse(t, viSnippet)
	if len(roots) != 2 {
		t.Fatalf("expected 2 root Chương, got %d: %v", len(roots), rootKinds(roots))
	}

	ch1 := roots[0]
	if ch1.Kind != "chuong" {
		t.Errorf("roots[0].Kind = %q, want chuong", ch1.Kind)
	}
	if ch1.Ordinal != 1 {
		t.Errorf("roots[0].Ordinal = %d, want 1", ch1.Ordinal)
	}
	if ch1.CitationPath != "chuong-I" {
		t.Errorf("roots[0].CitationPath = %q, want chuong-I", ch1.CitationPath)
	}
	if ch1.Heading != "QUY ĐỊNH CHUNG" {
		t.Errorf("roots[0].Heading = %q, want QUY ĐỊNH CHUNG", ch1.Heading)
	}
	if len(ch1.Children) != 2 {
		t.Fatalf("Chương I: expected 2 Điều, got %d", len(ch1.Children))
	}

	d1 := ch1.Children[0]
	if d1.Kind != "dieu" {
		t.Errorf("Điều 1.Kind = %q, want dieu", d1.Kind)
	}
	if d1.Ordinal != 1 {
		t.Errorf("Điều 1.Ordinal = %d, want 1", d1.Ordinal)
	}
	if d1.CitationPath != "chuong-I/dieu-1" {
		t.Errorf("Điều 1.CitationPath = %q, want chuong-I/dieu-1", d1.CitationPath)
	}
	if d1.Heading != "Phạm vi điều chỉnh" {
		t.Errorf("Điều 1.Heading = %q, want 'Phạm vi điều chỉnh'", d1.Heading)
	}

	d2 := ch1.Children[1]
	if d2.Kind != "dieu" {
		t.Errorf("Điều 2.Kind = %q, want dieu", d2.Kind)
	}
	if d2.CitationPath != "chuong-I/dieu-2" {
		t.Errorf("Điều 2.CitationPath = %q, want chuong-I/dieu-2", d2.CitationPath)
	}
	if len(d2.Children) != 2 {
		t.Fatalf("Điều 2: expected 2 Khoản, got %d", len(d2.Children))
	}
}

// TestParse_tree_khoanAndDiem continues TestParse_tree_chuongIHeaderAndArticles
// one level deeper: Điều 2's Khoản 1/2, and Khoản 2's Điểm a/b.
func TestParse_tree_khoanAndDiem(t *testing.T) {
	roots := mustParse(t, viSnippet)
	d2 := roots[0].Children[1]

	k1 := d2.Children[0]
	if k1.Kind != "khoan" {
		t.Errorf("Khoản 1.Kind = %q, want khoan", k1.Kind)
	}
	if k1.Ordinal != 1 {
		t.Errorf("Khoản 1.Ordinal = %d, want 1", k1.Ordinal)
	}
	if k1.CitationPath != "chuong-I/dieu-2/khoan-1" {
		t.Errorf("Khoản 1.CitationPath = %q, want chuong-I/dieu-2/khoan-1", k1.CitationPath)
	}

	k2 := d2.Children[1]
	if k2.CitationPath != "chuong-I/dieu-2/khoan-2" {
		t.Errorf("Khoản 2.CitationPath = %q, want chuong-I/dieu-2/khoan-2", k2.CitationPath)
	}
	if len(k2.Children) != 2 {
		t.Fatalf("Khoản 2: expected 2 Điểm, got %d", len(k2.Children))
	}

	da := k2.Children[0]
	if da.Kind != "diem" {
		t.Errorf("Điểm a.Kind = %q, want diem", da.Kind)
	}
	if da.CitationPath != "chuong-I/dieu-2/khoan-2/diem-a" {
		t.Errorf("Điểm a.CitationPath = %q, want chuong-I/dieu-2/khoan-2/diem-a", da.CitationPath)
	}
	db := k2.Children[1]
	if db.CitationPath != "chuong-I/dieu-2/khoan-2/diem-b" {
		t.Errorf("Điểm b.CitationPath = %q, want chuong-I/dieu-2/khoan-2/diem-b", db.CitationPath)
	}
}

func TestParse_tree_chuongII(t *testing.T) {
	roots := mustParse(t, viSnippet)
	if len(roots) != 2 {
		t.Fatalf("expected 2 root Chương, got %d", len(roots))
	}

	ch2 := roots[1]
	if ch2.CitationPath != "chuong-II" {
		t.Errorf("Chương II.CitationPath = %q, want chuong-II", ch2.CitationPath)
	}
	if ch2.Ordinal != 2 {
		t.Errorf("Chương II.Ordinal = %d, want 2", ch2.Ordinal)
	}
	if len(ch2.Children) != 1 {
		t.Fatalf("Chương II: expected 1 Điều, got %d", len(ch2.Children))
	}

	d3 := ch2.Children[0]
	if d3.CitationPath != "chuong-II/dieu-3" {
		t.Errorf("Điều 3.CitationPath = %q, want chuong-II/dieu-3", d3.CitationPath)
	}
	if len(d3.Children) != 2 {
		t.Fatalf("Điều 3: expected 2 Khoản, got %d", len(d3.Children))
	}
}

// TestParse_citationPaths verifies that every node in the tree has a unique
// CitationPath (the chunk dedup key).
func TestParse_citationPaths(t *testing.T) {
	roots := mustParse(t, viSnippet)
	seen := make(map[string]bool)
	var walk func([]*law.Node)
	walk = func(nodes []*law.Node) {
		for _, n := range nodes {
			if seen[n.CitationPath] {
				t.Errorf("duplicate citation_path: %q", n.CitationPath)
			}
			seen[n.CitationPath] = true
			walk(n.Children)
		}
	}
	walk(roots)
}

func TestParse_duplicateCitationPathsAreDisambiguated(t *testing.T) {
	roots := mustParse(t, `
Điều 1. Sửa đổi
1. Sửa đổi một nội dung.
2. Sửa đổi nội dung khác.
1. Trong nội dung được thay thế, khoản này xuất hiện lại.
a) Điểm trong phần được thay thế.
a) Điểm lặp lại trong phần được thay thế.
`)

	if len(roots) != 1 {
		t.Fatalf("roots = %d, want 1", len(roots))
	}
	var paths []string
	var walk func([]*law.Node)
	walk = func(nodes []*law.Node) {
		for _, n := range nodes {
			paths = append(paths, n.CitationPath)
			walk(n.Children)
		}
	}
	walk(roots)

	want := []string{
		"dieu-1",
		"dieu-1/khoan-1",
		"dieu-1/khoan-2",
		"dieu-1/khoan-1~2",
		"dieu-1/khoan-1~2/diem-a",
		"dieu-1/khoan-1~2/diem-a~2",
	}
	if len(paths) != len(want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Errorf("paths[%d] = %q, want %q", i, paths[i], want[i])
		}
	}
}

// TestParse_diemOrdinalVietnameseOrder exercises the Vietnamese điểm-letter
// ordinal mapping (banhmi's diemLetterOrdinal, unexported here too) through
// the public Parse API: đ sits right after d in Vietnamese legal ordering,
// not after z as in Unicode code-point order.
func TestParse_diemOrdinalVietnameseOrder(t *testing.T) {
	roots := mustParse(t, `
Điều 1. Thứ tự điểm
1. Khoản một.
d) Điểm d.
đ) Điểm đ.
e) Điểm e.
g) Điểm g.
`)

	if len(roots) != 1 || len(roots[0].Children) != 1 {
		t.Fatalf("tree = %#v, want one article with one clause", roots)
	}
	points := roots[0].Children[0].Children
	if len(points) != 4 {
		t.Fatalf("points = %d, want 4", len(points))
	}
	want := map[string]int{"d)": 4, "đ)": 5, "e)": 6, "g)": 7}
	for _, p := range points {
		if p.Ordinal != want[p.Label] {
			t.Errorf("point %q ordinal = %d, want %d", p.Label, p.Ordinal, want[p.Label])
		}
	}
}

// TestParse_flatDoc tests a document with no Chương hierarchy — just
// top-level Điều (common in shorter thông tư).
func TestParse_flatDoc(t *testing.T) {
	text := `

Điều 1. Mục đích
Quy định mục đích sử dụng.

Điều 2. Phạm vi
Áp dụng cho toàn quốc.
`
	roots := mustParse(t, text)
	if len(roots) != 2 {
		t.Fatalf("flat doc: expected 2 Điều, got %d", len(roots))
	}
	if roots[0].CitationPath != "dieu-1" {
		t.Errorf("roots[0].CitationPath = %q, want dieu-1", roots[0].CitationPath)
	}
	if roots[1].CitationPath != "dieu-2" {
		t.Errorf("roots[1].CitationPath = %q, want dieu-2", roots[1].CitationPath)
	}
}
