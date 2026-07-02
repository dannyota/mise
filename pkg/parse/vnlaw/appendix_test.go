package vnlaw_test

import (
	"strings"
	"testing"
)

func TestParseAppendixClosesLastArticle(t *testing.T) {
	md := `Điều 1. Phạm vi điều chỉnh
Thông tư này quy định về an toàn hệ thống thông tin.
Điều 2. Hiệu lực thi hành
Thông tư này có hiệu lực từ ngày 01/01/2027.
PHỤ LỤC 01
DANH MỤC HẠN MỨC GIAO DỊCH
Giao dịch loại A: tối đa 100 triệu đồng.
Giao dịch loại B: xác thực sinh trắc học.`

	roots := mustParse(t, md)
	if len(roots) != 3 {
		t.Fatalf("roots = %v, want [dieu dieu phuluc]", rootKinds(roots))
	}
	if roots[2].Kind != "phuluc" || roots[2].Label != "Phụ lục 01" {
		t.Fatalf("appendix root = %q/%q, want phuluc/Phụ lục 01", roots[2].Kind, roots[2].Label)
	}
	if !strings.Contains(roots[2].Content, "100 triệu đồng") {
		t.Fatalf("appendix content = %q, want the threshold rows", roots[2].Content)
	}
	if strings.Contains(roots[1].Content, "100 triệu đồng") {
		t.Fatalf("Điều 2 content %q still contains appendix text", roots[1].Content)
	}
	if roots[2].Heading != "DANH MỤC HẠN MỨC GIAO DỊCH" {
		t.Fatalf("appendix heading = %q", roots[2].Heading)
	}
}

func TestParseAppendixNestsAttachedRegulation(t *testing.T) {
	md := `Điều 1. Ban hành kèm theo Quyết định này Quy chế an toàn thông tin.
Điều 2. Hiệu lực thi hành.
Phụ lục
QUY CHẾ AN TOÀN THÔNG TIN
Chương I
QUY ĐỊNH CHUNG
Điều 1. Phạm vi điều chỉnh
Quy chế này quy định về bảo mật.`

	roots := mustParse(t, md)
	if len(roots) != 3 || roots[2].Kind != "phuluc" {
		t.Fatalf("roots = %v, want [dieu dieu phuluc]", rootKinds(roots))
	}
	pl := roots[2]
	if len(pl.Children) != 1 || pl.Children[0].Kind != "chuong" {
		t.Fatalf("appendix children = %+v, want one chuong", pl.Children)
	}
	ch := pl.Children[0]
	if len(ch.Children) != 1 || ch.Children[0].Kind != "dieu" || ch.Children[0].Label != "Điều 1" {
		t.Fatalf("chuong children = %+v, want nested Điều 1", ch.Children)
	}
}

func TestParseAppendixProseReferenceStaysText(t *testing.T) {
	md := `Điều 1. Hạn mức
Khách hàng thực hiện theo Phụ lục 01 ban hành kèm theo Thông tư này.
Phụ lục 01 quy định chi tiết các hạn mức giao dịch cho từng nhóm.
Điều 2. Hiệu lực thi hành.`

	roots := mustParse(t, md)
	if len(roots) != 2 {
		t.Fatalf("roots = %v, want two dieu only (prose references must not open an appendix)", rootKinds(roots))
	}
	if !strings.Contains(roots[0].Content, "quy định chi tiết các hạn mức") {
		t.Fatalf("Điều 1 content = %q, want the prose reference kept inline", roots[0].Content)
	}
}
