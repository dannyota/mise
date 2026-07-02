package vnlaw

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// ---- patterns ----------------------------------------------------------------
// All anchored at line start. Structural keywords require a numeral after them,
// which keeps prose like "Chương trình", "Mục lục", or "Điều kiện" from matching.

// phanRe matches "Phần <Roman>".
var phanRe = regexp.MustCompile(`^Phần\s+([IVXLC]+)\b`)

// chuongRe matches "Chương <Roman>".
var chuongRe = regexp.MustCompile(`^Chương\s+([IVXLC]+)\b`)

// mucRe matches "Mục <Roman or Arabic>".
var mucRe = regexp.MustCompile(`^Mục\s+([IVXLC0-9]+)\b`)

// dieuRe matches "Điều <N>" with an optional letter suffix for inserted
// articles (e.g. "Điều 21b" added by an amending decree).
var dieuRe = regexp.MustCompile(`^Điều\s+(\d+[a-zđ]?)\b`)

// khoanRe matches a numbered clause opening a line: "1. ", "12. ".
var khoanRe = regexp.MustCompile(`^(\d+)\.\s`)

// numericHeadingRe matches MarkItDown rows where a PDF visually had "Điều N" but
// text extraction only kept "N. <bold heading>". We only promote these when the
// heading itself is bold in the raw Markdown.
var numericHeadingRe = regexp.MustCompile(`^(\d+)\.\s+(.+)$`)

// numberedOutlineRe matches old narrative circulars that use "1.", "1.1.",
// etc. as their only citable structure.
var numberedOutlineRe = regexp.MustCompile(`^(\d+(?:\.\d+)*)(?:[\.)])\s+(.+)$`)

// romanOutlineHeadingRe and alphaOutlineHeadingRe cover older official texts
// that use outline sections ("I.", "I -", "A.") instead of statutory Điều
// headings.
var romanOutlineHeadingRe = regexp.MustCompile(`^([IVXLC]+)\s*[\.\-]\s+(.+)$`)

var alphaOutlineHeadingRe = regexp.MustCompile(`^([A-ZĐ])\s*[\.\-]\s+(.+)$`)

// phuLucLabelRe matches a line that IS an appendix label and nothing else:
// "Phụ lục", "PHỤ LỤC 01", "Phụ lục số 2:", "PHỤ LỤC IIa." — any case, optional
// "số", optional digit/roman designator with letter suffix, optional trailing
// punctuation. Prose that merely mentions an appendix keeps going after the
// designator and does not match.
var phuLucLabelRe = regexp.MustCompile(`^(?i)phụ\s+lục(?:\s+(?:số\s+)?([0-9]+[a-zđ]?|[ivxlc]+[a-zđ]?))?\s*[.:]?\s*$`)

// phuLucCapsRe matches an all-caps appendix heading that carries its title on
// the same line: "PHỤ LỤC DANH MỤC …". The all-caps keyword keeps sentence-case
// prose references ("theo Phụ lục 01 ban hành kèm theo…") out.
var phuLucCapsRe = regexp.MustCompile(`^PHỤ\s+LỤC\b(?:\s+(?:SỐ\s+)?([0-9]+[A-ZĐ]?|[IVXLC]+[A-ZĐ]?)\b)?\s*[.:–-]?\s*`)

// diemRe matches a lettered point opening a line: "a) ", "a. ", "a/ ", "đ) ".
var diemRe = regexp.MustCompile(`^([a-zđ])[\)\./]\s`)

// romanToInt converts an uppercase Roman numeral string to an integer.
// Supports values sufficient for Vietnamese legal docs.
func romanToInt(s string) int {
	vals := map[byte]int{'I': 1, 'V': 5, 'X': 10, 'L': 50, 'C': 100}
	total := 0
	prev := 0
	for i := len(s) - 1; i >= 0; i-- {
		v := vals[s[i]]
		if v < prev {
			total -= v
		} else {
			total += v
		}
		prev = v
	}
	return total
}

// atoiLeading parses the leading decimal digits of s (e.g. "21b" -> 21).
func atoiLeading(s string) int {
	n := 0
	for i := 0; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		n = n*10 + int(s[i]-'0')
	}
	return n
}

// diemMap converts a Vietnamese điểm letter to legal ordering: a, b, c, d, đ,
// e, g, h, i, k...
var diemMap = func() map[string]int {
	letters := []string{
		"a", "b", "c", "d", "đ", "e", "g", "h", "i", "k", "l", "m", "n",
		"o", "p", "q", "r", "s", "t", "u", "v", "x", "y",
	}
	m := make(map[string]int, len(letters))
	for i, letter := range letters {
		m[letter] = i + 1
	}
	return m
}()

// diemLetterOrdinal converts a Vietnamese điểm letter to its legal ordering,
// or 0 when the letter is not part of the Vietnamese alphabet used in points.
func diemLetterOrdinal(letter string) int {
	if n, ok := diemMap[letter]; ok {
		return n
	}
	return 0
}

// ---- line classification -----------------------------------------------------

type blockKind int

const (
	bkText blockKind = iota
	bkPhan
	bkChuong
	bkMuc
	bkDieu
	bkKhoan
	bkDiem
	bkPhuLuc
)

// levelOf returns the numeric depth of a blockKind for stack pruning.
func levelOf(k blockKind) int {
	switch k {
	case bkPhuLuc:
		return 1
	case bkPhan:
		return 1
	case bkChuong:
		return 2
	case bkMuc:
		return 3
	case bkDieu:
		return 4
	case bkKhoan:
		return 5
	case bkDiem:
		return 6
	case bkText:
	}
	return 0
}

// isStructural reports whether a kind takes a heading/title (and so may have its
// title on the next line). Khoản/Điểm carry their text inline instead.
func isStructural(k blockKind) bool {
	return k == bkPhan || k == bkChuong || k == bkMuc || k == bkDieu || k == bkPhuLuc
}

type token struct {
	kind          blockKind
	ordinal       int
	letter        string // for điểm
	label         string // full heading label, e.g. "Điều 7"
	heading       string // structural title on the same line (may be "")
	body          string // inline body for khoan/diem; raw text for bkText
	pathSeg       string // citation_path segment, e.g. "dieu-21b", "khoan-2", "diem-a"
	legacyOutline bool   // older outline-only sections like "I."/"A."
	explicitDieu  bool   // true when the article came from an explicit "Điều N" label
}

func afterLabel(line, label string) string {
	rest := strings.TrimSpace(line)
	for field := range strings.FieldsSeq(label) {
		if !strings.HasPrefix(rest, field) {
			return strings.TrimSpace(strings.TrimPrefix(line, label))
		}
		rest = strings.TrimSpace(rest[len(field):])
	}
	return strings.TrimLeft(strings.TrimSpace(rest), ".: ")
}

func startsWithOpeningQuote(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "“") || strings.HasPrefix(line, "\"")
}

func updateQuotedBlock(inQuote bool, line string) bool {
	delta := strings.Count(line, "“") - strings.Count(line, "”")
	switch {
	case delta > 0:
		return true
	case delta < 0:
		return false
	default:
		return inQuote
	}
}

// stripMDEmphasis removes one layer of wrapping Markdown emphasis from a line so
// "**Điều 1. ...**" is matched as "Điều 1. ...". The MarkItDown path can
// wrap headings in bold; plain text has no markers, so this is a
// no-op there. Only fully-wrapped lines are unwrapped — inline emphasis and table
// rows (which start with '|') are left intact.
func stripMDEmphasis(s string) string {
	s = strings.TrimSpace(s)
	for _, m := range []string{"**", "__", "*", "_"} {
		if len(s) >= 2*len(m) && strings.HasPrefix(s, m) && strings.HasSuffix(s, m) {
			return strings.TrimSpace(s[len(m) : len(s)-len(m)])
		}
	}
	return s
}

// stripMDStructuralMarkers removes emphasis markers that MarkItDown often puts
// around only the structural label: "**Điều 1.** Nội dung". Classification should
// see "Điều 1. Nội dung"; the stored heading should not carry Markdown markers.
func stripMDStructuralMarkers(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimLeft(s, "“\"'")
	s = strings.TrimSpace(s)
	for strings.HasPrefix(s, "#") {
		s = strings.TrimSpace(strings.TrimPrefix(s, "#"))
	}
	for {
		changed := false
		for _, m := range []string{"***", "**", "__", "*", "_"} {
			if rest, ok := strings.CutPrefix(s, m); ok {
				s = strings.TrimSpace(rest)
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	for _, m := range []string{"***", "**", "__", "*", "_"} {
		s = strings.ReplaceAll(s, m, "")
	}
	return strings.TrimSpace(s)
}

func hasBoldNumericHeading(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if inner := stripMDEmphasis(trimmed); inner != trimmed {
		return numericHeadingRe.MatchString(inner)
	}
	m := numericHeadingRe.FindStringSubmatch(trimmed)
	if m == nil {
		return false
	}
	heading := strings.TrimSpace(m[2])
	return strings.HasPrefix(heading, "**") ||
		strings.HasPrefix(heading, "***") ||
		strings.HasPrefix(heading, "__")
}

func isArticleLikeNumericHeading(clean string) bool {
	m := numericHeadingRe.FindStringSubmatch(clean)
	if m == nil {
		return false
	}
	heading := strings.ToLower(strings.TrimSpace(strings.Trim(m[2], ".:;")))
	for _, prefix := range []string{
		"phạm vi điều chỉnh",
		"đối tượng áp dụng",
		"giải thích từ ngữ",
		"hiệu lực thi hành",
		"điều khoản thi hành",
		"tổ chức thực hiện",
	} {
		if strings.HasPrefix(heading, prefix) {
			return true
		}
	}
	return false
}

func isOutlineHeading(raw, heading string) bool {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "**") || strings.Contains(raw, "__") {
		return true
	}

	var letters, upper int
	for _, r := range heading {
		if !unicode.IsLetter(r) {
			continue
		}
		letters++
		if unicode.IsUpper(r) {
			upper++
		}
	}
	return letters >= 5 && float64(upper)/float64(letters) >= 0.70
}

func alphaOrdinal(letter string) int {
	return diemLetterOrdinal(strings.ToLower(letter))
}

// kindName maps a blockKind to the law.Node.Kind string.
func kindName(k blockKind) string {
	switch k {
	case bkPhan:
		return "phan"
	case bkChuong:
		return "chuong"
	case bkMuc:
		return "muc"
	case bkDieu:
		return "dieu"
	case bkKhoan:
		return "khoan"
	case bkDiem:
		return "diem"
	case bkPhuLuc:
		return "phuluc"
	case bkText:
	}
	return "dieu"
}

func defaultLabel(kind blockKind, ord int) string {
	switch kind {
	case bkDieu:
		return "Điều " + strconv.Itoa(ord)
	case bkKhoan:
		return strconv.Itoa(ord) + "."
	case bkDiem:
		return "Điểm " + strconv.Itoa(ord)
	case bkMuc:
		return "Mục " + strconv.Itoa(ord)
	case bkChuong:
		return "Chương " + strconv.Itoa(ord)
	case bkPhan:
		return "Phần " + strconv.Itoa(ord)
	case bkPhuLuc:
		return "Phụ lục " + strconv.Itoa(ord)
	case bkText:
	}
	return strconv.Itoa(ord)
}

func defaultPathSegment(kind blockKind, ord int) string {
	return kindName(kind) + "-" + strconv.Itoa(ord)
}

// uniqueCitationPath disambiguates a repeated citation_path (e.g. a quoted
// amendment re-using "1." inside its source clause) by appending "~N" for the
// 2nd and later occurrences, shared by both the Markdown state machine and the
// VBPL provision-tree builder.
func uniqueCitationPath(path string, counts map[string]int) string {
	counts[path]++
	if counts[path] == 1 {
		return path
	}
	return path + "~" + strconv.Itoa(counts[path])
}
