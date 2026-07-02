package vnlaw_test

import (
	"strings"
	"testing"

	"danny.vn/mise/pkg/parse/law"
)

func TestParse_numberedOutlineFallback(t *testing.T) {
	md := `
NGÂN HÀNG NHÀ NƯỚC

THÔNG TƯ

Ngày 25-11-1993, Chính phủ đã ban hành Nghị định.

1. Số liệu ở tài khoản được cung cấp trong các trường hợp sau đây:

1.1. Theo yêu cầu của chủ tài khoản.

1.2. Theo quy định của cơ quan có thẩm quyền.

2. Thông tư này có hiệu lực thi hành kể từ ngày ký.
`

	roots := mustParse(t, md)
	if len(roots) != 2 {
		t.Fatalf("roots = %d, want 2 numbered outline roots", len(roots))
	}
	if roots[0].Kind != "dieu" || roots[0].Label != "1." {
		t.Fatalf("first root = %s/%s, want dieu/1.", roots[0].Kind, roots[0].Label)
	}
	if len(roots[0].Children) != 2 {
		t.Fatalf("first root children = %d, want 2", len(roots[0].Children))
	}
	if roots[0].Children[1].CitationPath != "dieu-outline-1/khoan-outline-1-2" {
		t.Fatalf("second child path = %q", roots[0].Children[1].CitationPath)
	}
}

func TestParse_supplementOnlyDoesNotUseNumberedFallback(t *testing.T) {
	md := `
**BÁO CÁO TÌNH HÌNH THỰC HIỆN CƠ CẤU LẠI THỜI HẠN TRẢ NỢ**

1. Tình hình thực hiện cơ cấu lại thời hạn trả nợ.

2. Tổng dư nợ không bị chuyển sang nhóm nợ xấu.
`

	if roots := mustParse(t, md); len(roots) != 0 {
		t.Fatalf("roots = %#v, want no synthetic legal sections for supplement text", roots)
	}
}

func TestParse_wholeDocumentFallback(t *testing.T) {
	md := `
NGÂN HÀNG NHÀ NƯỚC VIỆT NAM

THÔNG TƯ LIÊN BỘ

Về việc hướng dẫn một số nội dung nghiệp vụ ngân hàng trong thời kỳ chuyển tiếp.

Căn cứ chức năng, nhiệm vụ của các cơ quan quản lý nhà nước, văn bản này hướng dẫn
các ngân hàng thương mại, tổ chức tín dụng và các đơn vị có liên quan thực hiện
thống nhất việc mở tài khoản, hạch toán, thanh toán và báo cáo định kỳ.

Các đơn vị phải tổ chức thực hiện nghiêm túc, kịp thời phản ánh khó khăn vướng mắc
về Ngân hàng Nhà nước Việt Nam để tổng hợp, xem xét và xử lý theo thẩm quyền.
`

	roots := mustParse(t, md)
	if len(roots) != 1 {
		t.Fatalf("roots = %d, want one whole-document fallback", len(roots))
	}
	if roots[0].Kind != "dieu" || roots[0].Label != "Toàn văn" || roots[0].CitationPath != "toan-van" {
		t.Fatalf("fallback root = %s/%s/%s", roots[0].Kind, roots[0].Label, roots[0].CitationPath)
	}
	if !strings.Contains(roots[0].Content, "hạch toán, thanh toán") {
		t.Fatalf("fallback content missing body: %q", roots[0].Content)
	}
}

// TestParse_realFailureModes pins the failure modes found on real SBV
// circulars (09/2020/TT-NHNN). Each case is a regression guard for the
// line-level, engine-uniform parser. Counts are checked exactly (default 0).
func TestParse_realFailureModes(t *testing.T) {
	tests := []struct {
		name                      string
		md                        string
		chuong, dieu, khoan, diem int
		check                     func(t *testing.T, roots []*law.Node)
	}{
		{
			// The headline bug: clauses are written "1."/"2." (not "Khoản 1") and
			// points "a)"/"b)" (not "Điểm a") with no blank line — these were all
			// swallowed by the old block parser.
			name: "clauses_as_N_dot_and_points_as_letter",
			md:   "Điều 5. Phân loại\n1. Nhóm một.\n2. Nhóm hai.\na) Điểm a.\nb) Điểm b.\n",
			dieu: 1, khoan: 2, diem: 2,
		},
		{
			// PDF extraction emits no boundary blank lines at all.
			name: "no_blank_lines_pdf_style",
			md:   "Điều 1. A\nNội dung.\nĐiều 2. B\n1. x\n2. y\nĐiều 3. C\n",
			dieu: 3, khoan: 2,
		},
		{
			// Real DOCX writes "Chương<NBSP>II" with a no-break space.
			name:   "nbsp_in_chuong_heading",
			md:     "Chương\u00a0II\nQUY ĐỊNH\nĐiều 1. A\n",
			chuong: 1, dieu: 1,
		},
		{
			// "Chương trình" (a programme) must not be parsed as a Chương.
			name: "chuong_trinh_is_not_a_chuong",
			md:   "Điều 1. Phê duyệt\nChương trình được triển khai theo kế hoạch.\n",
			dieu: 1,
		},
		{
			// A mid-sentence cross-reference must not create an Điều node.
			name: "cross_reference_not_a_heading",
			md:   "Điều 1. Sửa đổi\nbãi bỏ khoản 3 Điều 12 của Nghị định.\n",
			dieu: 1,
		},
		{
			// An inserted article keeps its letter suffix in the citation path.
			name: "dieu_letter_suffix",
			md:   "Điều 21b. Bổ sung\n1. Nội dung mới.\n",
			dieu: 1, khoan: 1,
			check: func(t *testing.T, roots []*law.Node) {
				t.Helper()
				if roots[0].CitationPath != "dieu-21b" {
					t.Errorf("CitationPath = %q, want dieu-21b", roots[0].CitationPath)
				}
			},
		},
		{
			// A "1." outside any Điều (preamble) is body text, not a Khoản.
			name: "numbered_line_outside_dieu_is_not_khoan",
			md:   "Số: 09/2020/TT-NHNN\n1. Căn cứ Luật Ngân hàng Nhà nước.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roots := mustParse(t, tt.md)
			m := map[string]int{}
			countAllKinds(roots, m)
			if m["chuong"] != tt.chuong {
				t.Errorf("chuong = %d, want %d", m["chuong"], tt.chuong)
			}
			if m["dieu"] != tt.dieu {
				t.Errorf("dieu = %d, want %d", m["dieu"], tt.dieu)
			}
			if m["khoan"] != tt.khoan {
				t.Errorf("khoan = %d, want %d", m["khoan"], tt.khoan)
			}
			if m["diem"] != tt.diem {
				t.Errorf("diem = %d, want %d", m["diem"], tt.diem)
			}
			if tt.check != nil {
				tt.check(t, roots)
			}
		})
	}
}
