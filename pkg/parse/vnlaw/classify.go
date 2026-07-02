package vnlaw

import (
	"strings"
)

// ---- line classification (patterns.go's regexes -> tokens) --------------------

func numericArticleToken(line string) (token, bool) {
	clean := stripMDStructuralMarkers(line)
	m := numericHeadingRe.FindStringSubmatch(clean)
	if m == nil {
		return token{}, false
	}
	return token{kind: bkDieu, heading: strings.TrimSpace(m[2])}, true
}

// phuLucToken recognises an appendix heading: either a line that is exactly the
// appendix label (any case), or an all-caps "PHỤ LỤC …" heading carrying its
// title inline. Appendices sit at the root level (level 1), so an appendix
// closes every open article — which is how trailing forms/tables stop polluting
// the last Điều's content.
func phuLucToken(clean string) (token, bool) {
	if m := phuLucLabelRe.FindStringSubmatch(clean); m != nil {
		return phuLucTokenWithDesignator(m[1], ""), true
	}
	if m := phuLucCapsRe.FindStringSubmatch(clean); m != nil {
		return phuLucTokenWithDesignator(m[1], strings.TrimSpace(clean[len(m[0]):])), true
	}
	return token{}, false
}

func phuLucTokenWithDesignator(des, heading string) token {
	des = strings.TrimSpace(des)
	t := token{kind: bkPhuLuc, heading: heading, label: "Phụ lục"}
	if des == "" {
		return t // ordinal and path segment fall back to the running counter
	}
	t.label = "Phụ lục " + des
	t.pathSeg = "phuluc-" + strings.ToLower(des)
	if ord := atoiLeading(des); ord > 0 {
		t.ordinal = ord
	} else if ord := romanToInt(strings.ToUpper(des)); ord > 0 {
		t.ordinal = ord
	}
	return t
}

func clauseToken(clean string) (token, bool) {
	if m := khoanRe.FindStringSubmatch(clean); m != nil {
		return token{
			kind: bkKhoan, ordinal: atoiLeading(m[1]), label: m[1] + ".",
			body: strings.TrimSpace(clean[len(m[0]):]), pathSeg: "khoan-" + m[1],
		}, true
	}
	if m := diemRe.FindStringSubmatch(clean); m != nil {
		letter := m[1]
		return token{
			kind: bkDiem, ordinal: diemLetterOrdinal(letter), letter: letter, label: letter + ")",
			body: strings.TrimSpace(clean[len(m[0]):]), pathSeg: "diem-" + letter,
		}, true
	}
	return token{}, false
}

// classifyLine identifies the structural role of a single trimmed line.
// Khoản/Điểm are only recognised when an open Điều/Khoản or a legacy outline
// section is on the stack.
func classifyLine(line string, canStartClause, inExplicitArticle bool) token {
	clean := stripMDStructuralMarkers(line)
	if t, ok := classifyHeading(line, clean); ok {
		return t
	}
	if canStartClause && inExplicitArticle {
		if t, ok := clauseToken(clean); ok {
			return t
		}
	}
	if t, ok := classifyLegacyNumericOrOutline(line, clean); ok {
		return t
	}
	if canStartClause {
		if t, ok := clauseToken(clean); ok {
			return t
		}
	}
	return token{kind: bkText, body: line}
}

// classifyHeading recognises the statutory Phần/Chương/Mục/Điều/Phụ lục
// headings, in the fixed precedence order the original parser relies on
// (Phụ lục before Điều, since an appendix designator can look like an article
// reference). line is the pre-strip line, needed verbatim for the "looked
// like Điều N but isn't followed by a separator" text fallback.
func classifyHeading(line, clean string) (token, bool) {
	if m := phanRe.FindStringSubmatch(clean); m != nil {
		label := "Phần " + m[1]
		return token{
			kind: bkPhan, ordinal: romanToInt(m[1]), label: label,
			heading: afterLabel(clean, label), pathSeg: "phan-" + m[1],
		}, true
	}
	if m := chuongRe.FindStringSubmatch(clean); m != nil {
		label := "Chương " + m[1]
		return token{
			kind: bkChuong, ordinal: romanToInt(m[1]), label: label,
			heading: afterLabel(clean, label), pathSeg: "chuong-" + m[1],
		}, true
	}
	if m := mucRe.FindStringSubmatch(clean); m != nil {
		label := "Mục " + m[1]
		ord := romanToInt(m[1])
		if ord == 0 {
			ord = atoiLeading(m[1])
		}
		return token{kind: bkMuc, ordinal: ord, label: label, heading: afterLabel(clean, label), pathSeg: "muc-" + m[1]}, true
	}
	if t, ok := phuLucToken(clean); ok {
		return t, true
	}
	if m := dieuRe.FindStringSubmatch(clean); m != nil {
		key := m[1] // "1" or "21b"
		rest := strings.TrimSpace(clean[len(m[0]):])
		if rest != "" && !strings.HasPrefix(rest, ".") && !strings.HasPrefix(rest, ":") {
			return token{kind: bkText, body: line}, true
		}
		label := "Điều " + key
		h := strings.TrimSpace(strings.TrimPrefix(afterLabel(clean, label), "."))
		return token{
			kind: bkDieu, ordinal: atoiLeading(key), label: label,
			heading: h, pathSeg: "dieu-" + key, explicitDieu: true,
		}, true
	}
	return token{}, false
}

// classifyLegacyNumericOrOutline recognises the numeric-heading promotions and
// the older Roman/alpha outline headings that stand in for Điều/Mục in older
// circulars.
func classifyLegacyNumericOrOutline(line, clean string) (token, bool) {
	if isArticleLikeNumericHeading(clean) || hasBoldNumericHeading(line) {
		if m := numericHeadingRe.FindStringSubmatch(clean); m != nil {
			return token{kind: bkDieu, heading: strings.TrimSpace(m[2])}, true
		}
	}
	if m := romanOutlineHeadingRe.FindStringSubmatch(clean); m != nil && isOutlineHeading(line, m[2]) {
		label := m[1] + "."
		return token{
			kind: bkChuong, ordinal: romanToInt(m[1]), label: label,
			heading: strings.TrimSpace(m[2]), pathSeg: "chuong-" + m[1],
			legacyOutline: true,
		}, true
	}
	if m := alphaOutlineHeadingRe.FindStringSubmatch(clean); m != nil && isOutlineHeading(line, m[2]) {
		label := m[1] + "."
		return token{
			kind: bkMuc, ordinal: alphaOrdinal(m[1]), label: label,
			heading: strings.TrimSpace(m[2]), pathSeg: "muc-" + m[1],
			legacyOutline: true,
		}, true
	}
	return token{}, false
}
