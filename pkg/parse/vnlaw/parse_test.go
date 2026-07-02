// Package vnlaw_test ports the acceptance tests for the Vietnamese legal
// structure parser from banhmi's pkg/pipeline/legalparse_test.go. The
// regression cases (real failure modes pinned from actual SBV circulars) and
// fixtures are kept byte-faithful; only the type (law.Node vs. banhmi's
// Section) and function name (Parse vs. ParseSections) changed.
package vnlaw_test

import (
	"strings"
	"testing"

	"danny.vn/mise/pkg/parse/law"
	"danny.vn/mise/pkg/parse/vnlaw"
)

// viSnippet is a realistic Vietnamese legal Markdown text covering the main
// structural levels (Chương, Điều, Khoản, Điểm) with body text in each.
// Blank lines before headings mirror what the DOCX extractor emits.
const viSnippet = `

Chương I
QUY ĐỊNH CHUNG

Điều 1. Phạm vi điều chỉnh
Thông tư này quy định về hoạt động cho vay.

Điều 2. Đối tượng áp dụng
Thông tư này áp dụng với các tổ chức tín dụng.

1. Ngân hàng thương mại nhà nước.

2. Ngân hàng thương mại cổ phần.

a) Ngân hàng trong nước.

b) Ngân hàng nước ngoài.

Chương II
QUY ĐỊNH CỤ THỂ

Điều 3. Nguyên tắc cho vay
Tổ chức tín dụng thực hiện theo nguyên tắc sau.

1. Nguyên tắc một: sử dụng vốn đúng mục đích.

2. Nguyên tắc hai: hoàn trả đầy đủ gốc và lãi.
`

func mustParse(t *testing.T, md string) []*law.Node {
	t.Helper()
	roots, err := vnlaw.Parse(md)
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	return roots
}

func TestParse_markdownEmphasis(t *testing.T) {
	// MarkItDown wraps headings in bold ("**Điều 1. ...**") and renders GFM tables.
	// The parser must still recognise the Chương/Điều/Khoản structure, store clean
	// heading text (no ** markers), and not let table rows leak as sections.
	md := "\n**Chương I**\n\n**QUY ĐỊNH CHUNG**\n\n" +
		"**Điều 1. Phạm vi điều chỉnh**\n\nThông tư này quy định về hoạt động cho vay.\n\n" +
		"| **STT** | **Tên tổ chức** |\n| --- | --- |\n| 1 | Ngân hàng A |\n\n" +
		"**Điều 2. Đối tượng áp dụng**\n\n1. Ngân hàng thương mại nhà nước.\n\n2. Ngân hàng thương mại cổ phần.\n"

	roots := mustParse(t, md)
	if len(roots) != 1 || roots[0].Kind != "chuong" {
		t.Fatalf("expected 1 Chương root, got %d: %v", len(roots), rootKinds(roots))
	}
	ch := roots[0]
	if ch.Ordinal != 1 || ch.Heading != "QUY ĐỊNH CHUNG" {
		t.Errorf("Chương: ordinal=%d heading=%q, want 1 / 'QUY ĐỊNH CHUNG'", ch.Ordinal, ch.Heading)
	}
	if len(ch.Children) != 2 {
		t.Fatalf("expected 2 Điều under Chương I, got %d", len(ch.Children))
	}
	d1 := ch.Children[0]
	if d1.Kind != "dieu" || d1.Ordinal != 1 {
		t.Errorf("Điều 1: kind=%q ordinal=%d, want dieu / 1", d1.Kind, d1.Ordinal)
	}
	if d1.Heading != "Phạm vi điều chỉnh" {
		t.Errorf("Điều 1 heading = %q, want 'Phạm vi điều chỉnh' (markdown ** must be stripped)", d1.Heading)
	}
	if d2 := ch.Children[1]; d2.Ordinal != 2 || len(d2.Children) != 2 {
		t.Errorf("Điều 2: ordinal=%d khoản=%d, want 2 / 2", d2.Ordinal, len(d2.Children))
	}
}

func TestParse_inlineMarkdownEmphasis(t *testing.T) {
	md := `
**Điều 1.** Ban hành kèm theo Quyết định này Quy chế thanh toán.

***Điều 2.*** Quyết định này có hiệu lực thi hành.
`

	roots := mustParse(t, md)
	if len(roots) != 2 {
		t.Fatalf("roots = %d, want 2", len(roots))
	}
	if roots[0].CitationPath != "dieu-1" {
		t.Errorf("roots[0].CitationPath = %q, want dieu-1", roots[0].CitationPath)
	}
	if roots[0].Heading != "Ban hành kèm theo Quyết định này Quy chế thanh toán." {
		t.Errorf("roots[0].Heading = %q", roots[0].Heading)
	}
	if roots[1].CitationPath != "dieu-2" {
		t.Errorf("roots[1].CitationPath = %q, want dieu-2", roots[1].CitationPath)
	}
}

func TestParseWhitespaceInsensitiveArticleLabel(t *testing.T) {
	md := `
Điều  1.  Sửa đổi, bổ sung một số điều

1. Sửa đổi tên văn bản.
`

	roots := mustParse(t, md)
	if len(roots) != 1 {
		t.Fatalf("roots = %d, want 1", len(roots))
	}
	if roots[0].Heading != "Sửa đổi, bổ sung một số điều" {
		t.Fatalf("heading = %q, want stripped article label", roots[0].Heading)
	}
	if strings.Contains(roots[0].Heading, "Điều") {
		t.Fatalf("heading still contains article label: %q", roots[0].Heading)
	}
	if len(roots[0].Children) != 1 || roots[0].Children[0].Content != "Sửa đổi tên văn bản." {
		t.Fatalf("children = %#v, want one clean clause", roots[0].Children)
	}
}

func TestParseBoldNumberedLinesInsideArticleAreClauses(t *testing.T) {
	md := `
**Điều 1.** Phân công công tác

**1. Thống đốc.**

Chỉ đạo chung.

**2. Phó Thống đốc.**

Giúp Thống đốc.
`

	roots := mustParse(t, md)
	if len(roots) != 1 {
		t.Fatalf("roots = %d, want 1", len(roots))
	}
	if len(roots[0].Children) != 2 {
		t.Fatalf("children = %d, want 2 clauses: %#v", len(roots[0].Children), roots[0].Children)
	}
	for i, child := range roots[0].Children {
		if child.Kind != "khoan" {
			t.Fatalf("child %d kind = %q, want khoan", i, child.Kind)
		}
	}
}

func TestParseKeepsQuotedAmendmentInsideSourceClause(t *testing.T) {
	md := `
Điều 1. Sửa đổi, bổ sung một số điều

1. Bổ sung Điều 4a vào sau Điều 4 như sau:

“Điều 4a. Áp dụng biện pháp

1. Việc áp dụng biện pháp thực hiện theo quy định.

a) Trường hợp thứ nhất;

b) Trường hợp thứ hai.”.

2. Sửa đổi khoản 1 Điều 5.
`

	roots := mustParse(t, md)
	if len(roots) != 1 {
		t.Fatalf("roots = %d, want source document article only", len(roots))
	}
	if roots[0].Heading != "Sửa đổi, bổ sung một số điều" {
		t.Fatalf("heading = %q", roots[0].Heading)
	}
	if len(roots[0].Children) != 2 {
		t.Fatalf("source clauses = %d, want 2: %#v", len(roots[0].Children), roots[0].Children)
	}
	first := roots[0].Children[0]
	if first.Kind != "khoan" || first.Ordinal != 1 {
		t.Fatalf("first child = %#v, want source clause 1", first)
	}
	for _, want := range []string{"Điều 4a. Áp dụng biện pháp", "1. Việc áp dụng", "a) Trường hợp thứ nhất"} {
		if !strings.Contains(first.Content, want) {
			t.Fatalf("quoted amendment content missing %q in %q", want, first.Content)
		}
	}
}

func TestParseDoesNotPromoteWrappedArticleReferences(t *testing.T) {
	md := `
Điều 3. Bãi bỏ một số điều

Bãi bỏ Điều 31; Điều 32;

Điều 33; khoản 1 Điều 34; các khoản 1 và 2 Điều 35;

Điều 103 Nghị định số 15/2020/NĐ-CP.
`

	roots := mustParse(t, md)
	if len(roots) != 1 {
		t.Fatalf("roots = %d, want only source article: %#v", len(roots), roots)
	}
	if roots[0].Label != "Điều 3" {
		t.Fatalf("label = %q, want Điều 3", roots[0].Label)
	}
	for _, want := range []string{"Điều 33; khoản 1", "Điều 103 Nghị định"} {
		if !strings.Contains(roots[0].Content, want) {
			t.Fatalf("article reference missing %q in %q", want, roots[0].Content)
		}
	}
}

func TestParse_atxMarkdownHeadings(t *testing.T) {
	md := `
# Chương I QUY ĐỊNH CHUNG

### Điều 1. Phạm vi điều chỉnh

1. Khoản một.
`

	roots := mustParse(t, md)
	if len(roots) != 1 || roots[0].CitationPath != "chuong-I" {
		t.Fatalf("roots = %#v, want one chuong-I", roots)
	}
	if len(roots[0].Children) != 1 {
		t.Fatalf("children = %d, want 1", len(roots[0].Children))
	}
	dieu := roots[0].Children[0]
	if dieu.CitationPath != "chuong-I/dieu-1" {
		t.Errorf("dieu path = %q, want chuong-I/dieu-1", dieu.CitationPath)
	}
	if len(dieu.Children) != 1 || dieu.Children[0].Kind != "khoan" {
		t.Fatalf("dieu children = %#v, want one khoan", dieu.Children)
	}
}

func TestParse_numericBoldHeadingsAsArticles(t *testing.T) {
	md := `
**Chương I**

**QUY ĐỊNH CHUNG**

1. **Phạm vi điều chỉnh**

1. Thông tư này quy định một nội dung.

a) Một điểm.

1. **Đối tượng áp dụng**

1. Tổ chức tín dụng.
`

	roots := mustParse(t, md)
	if len(roots) != 1 {
		t.Fatalf("roots = %d, want 1", len(roots))
	}
	if len(roots[0].Children) != 2 {
		t.Fatalf("articles = %d, want 2", len(roots[0].Children))
	}
	d1 := roots[0].Children[0]
	if d1.CitationPath != "chuong-I/dieu-1" || d1.Heading != "Phạm vi điều chỉnh" {
		t.Fatalf("first article path/heading = %q/%q", d1.CitationPath, d1.Heading)
	}
	if len(d1.Children) != 1 || d1.Children[0].Kind != "khoan" {
		t.Fatalf("first article children = %#v, want one khoan", d1.Children)
	}
	d2 := roots[0].Children[1]
	if d2.CitationPath != "chuong-I/dieu-2" || d2.Heading != "Đối tượng áp dụng" {
		t.Fatalf("second article path/heading = %q/%q", d2.CitationPath, d2.Heading)
	}
}

func TestParse_numericPlainArticleHeadings(t *testing.T) {
	md := `
**Chương I**

**NHỮNG QUY ĐỊNH CHUNG**

1. Phạm vi điều chỉnh

Nghị định này quy định về giao dịch điện tử.

1. Đối tượng áp dụng

Nghị định này áp dụng đối với cơ quan, tổ chức, cá nhân.

1. Giải thích từ ngữ

Trong Nghị định này, các từ ngữ dưới đây được hiểu như sau:

1. Quản trị nội bộ trên môi trường điện tử là việc xử lý công việc nội bộ.
`

	roots := mustParse(t, md)
	if len(roots) != 1 {
		t.Fatalf("roots = %d, want 1", len(roots))
	}
	if len(roots[0].Children) != 3 {
		t.Fatalf("articles = %d, want 3", len(roots[0].Children))
	}
	if roots[0].Children[0].Heading != "Phạm vi điều chỉnh" {
		t.Errorf("first heading = %q", roots[0].Children[0].Heading)
	}
	if roots[0].Children[2].Heading != "Giải thích từ ngữ" {
		t.Errorf("third heading = %q", roots[0].Children[2].Heading)
	}
	if len(roots[0].Children[2].Children) != 1 || roots[0].Children[2].Children[0].Kind != "khoan" {
		t.Fatalf("definition clause = %#v, want one khoan", roots[0].Children[2].Children)
	}
}

func TestParse_legacyOutlineHeadings(t *testing.T) {
	md := `
**I - VẬN DỤNG CÁC TIÊU CHUẨN ĐỂ PHÂN LOẠI XÍ NGHIỆP**

**A. Việc tổ chức thực hiện thanh toán và cho vay**

1. Các cơ quan ngân hàng thực hiện nghiệp vụ theo đúng chế độ.

a) Tiêu chuẩn 1.

b/ Tiêu chuẩn 2.
`

	roots := mustParse(t, md)
	if len(roots) != 1 || roots[0].Kind != "chuong" {
		t.Fatalf("roots = %#v, want one legacy chuong", roots)
	}
	if len(roots[0].Children) != 1 || roots[0].Children[0].Kind != "muc" {
		t.Fatalf("legacy children = %#v, want one muc", roots[0].Children)
	}
	muc := roots[0].Children[0]
	if len(muc.Children) != 1 || muc.Children[0].Kind != "khoan" {
		t.Fatalf("muc children = %#v, want one khoan", muc.Children)
	}
	if len(muc.Children[0].Children) != 2 {
		t.Fatalf("points = %d, want 2", len(muc.Children[0].Children))
	}
	if muc.Children[0].Children[1].CitationPath != "chuong-I/muc-A/khoan-1/diem-b" {
		t.Errorf("second point path = %q", muc.Children[0].Children[1].CitationPath)
	}
}

func TestParse_fullyWrappedNumericHeadings(t *testing.T) {
	md := `
**1. Về mở và sử dụng tài khoản (điều 34):**

- Các xí nghiệp quốc doanh có quyền lựa chọn ngân hàng.

**2. Về quan hệ tín dụng:**

- Ngân hàng thực hiện theo quy định.
`

	roots := mustParse(t, md)
	if len(roots) != 2 {
		t.Fatalf("roots = %d, want 2", len(roots))
	}
	if roots[0].CitationPath != "dieu-1" || roots[1].CitationPath != "dieu-2" {
		t.Fatalf("paths = %q/%q, want dieu-1/dieu-2", roots[0].CitationPath, roots[1].CitationPath)
	}
}

func TestParse_repeatedNumberedClausesUnderChapterStayClauses(t *testing.T) {
	md := `
Chương I
QUY ĐỊNH CHUNG

Điều 1. Sửa đổi

1. Sửa đổi một nội dung.

2. Sửa đổi nội dung khác.

1. Trong nội dung được thay thế, khoản này xuất hiện lại.

**1. Khoản in đậm vẫn là khoản.**
`

	roots := mustParse(t, md)
	if len(roots) != 1 || len(roots[0].Children) != 1 {
		t.Fatalf("tree = %#v, want one article under chapter", roots)
	}
	dieu := roots[0].Children[0]
	if len(dieu.Children) != 4 {
		t.Fatalf("clauses = %d, want 4", len(dieu.Children))
	}
	for _, child := range dieu.Children {
		if child.Kind != "khoan" {
			t.Fatalf("child kind = %q, want khoan", child.Kind)
		}
	}
	if dieu.Children[2].CitationPath != "chuong-I/dieu-1/khoan-1~2" {
		t.Errorf("third clause path = %q", dieu.Children[2].CitationPath)
	}
	if dieu.Children[3].Heading != "" || dieu.Children[3].Content != "Khoản in đậm vẫn là khoản." {
		t.Errorf("bold clause heading/content = %q/%q", dieu.Children[3].Heading, dieu.Children[3].Content)
	}
}

// ---- helpers ----------------------------------------------------------------

// rootKinds collects each root node's Kind, for failure messages.
func rootKinds(nodes []*law.Node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.Kind
	}
	return out
}

// countAllKinds counts nodes by Kind across the whole tree.
func countAllKinds(nodes []*law.Node, m map[string]int) {
	for _, n := range nodes {
		m[n.Kind]++
		countAllKinds(n.Children, m)
	}
}
