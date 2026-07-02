// Package quality_test ports the Assess/GateConfig acceptance tests from
// banhmi's pkg/extract/extract_test.go (mojibake, diacritic-density, and
// whitespace signal cases). Tests for banhmi surface this light port drops
// (GateFromSettings, SourceUnavailable/OfficialPlaceholder, the standalone
// CyrillicMojibake helper, the package-level Assess back-compat wrapper) are
// not ported — see quality.go's package doc for why.
package quality_test

import (
	"strings"
	"testing"

	"danny.vn/mise/pkg/parse/quality"
)

func TestDefaultGate(t *testing.T) {
	g := quality.DefaultGate()
	if g.MaxBadRatio != 0.01 {
		t.Errorf("MaxBadRatio: got %v, want 0.01", g.MaxBadRatio)
	}
	if g.MinLetters != 50 {
		t.Errorf("MinLetters: got %v, want 50", g.MinLetters)
	}
	if g.PassThreshold != 0.6 {
		t.Errorf("PassThreshold: got %v, want 0.6", g.PassThreshold)
	}
}

func TestGate_GoodVietnameseText(t *testing.T) {
	// Real-looking VN legal text: diacritic-rich, no bad chars.
	text := strings.Repeat("Điều 1. Ngân hàng Nhà nước Việt Nam quy định các điều kiện sau đây. Khoản 1 áp dụng. ", 10)
	r := quality.DefaultGate().Assess(text)
	if !r.OK {
		t.Errorf("good text should pass, got ok=false reason=%q confidence=%f", r.Reason, r.Confidence)
	}
	if r.Confidence < 0.6 {
		t.Errorf("confidence too low: %f", r.Confidence)
	}
}

func TestGate_EmptyText(t *testing.T) {
	r := quality.DefaultGate().Assess("")
	if r.OK {
		t.Error("empty text should not pass")
	}
	if r.Reason == "" {
		t.Error("empty text should have a reason")
	}
}

func TestGate_MojibakeText(t *testing.T) {
	// Simulate TCVN3/VNI PUA mojibake: fill text with U+E001-U+E01E runes.
	var sb strings.Builder
	for i := range 200 {
		sb.WriteRune('\ue001' + rune(i%30)) // PUA range
	}
	r := quality.DefaultGate().Assess(sb.String())
	if r.OK {
		t.Errorf("mojibake text should not pass, got ok=true confidence=%f", r.Confidence)
	}
}

func TestGate_UTF8MojibakeMarkers(t *testing.T) {
	text := strings.Repeat("NG√ÇN H√ÄNG NH√Ä N∆Ø·ªöC VI·ªÜT NAM C·ªòNG H√íA X√É H·ªòI CH·ª¶ NGHƒ®A ", 12)
	r := quality.DefaultGate().Assess(text)
	if r.OK {
		t.Fatalf("UTF-8 mojibake text passed, confidence=%f", r.Confidence)
	}
	if !strings.Contains(r.Reason, "mojibake") {
		t.Fatalf("reason = %q, want mojibake diagnosis", r.Reason)
	}
}

func TestGate_CyrillicMojibake(t *testing.T) {
	// CP1251 double-encode: Vietnamese UTF-8 mis-decoded as Windows-1251 surfaces
	// as Cyrillic. Latin-script legal text never legitimately contains Cyrillic.
	text := strings.Repeat("Дҗiб»Ғu 1. Hб»“ sЖЎ Д‘б»Ғ nghб»Ӣ cбәҘp GiбәҘy chб»©ng nhбәӯn dб»Ҝ liб»Үu cГЎ nhГўn ", 12)
	r := quality.DefaultGate().Assess(text)
	if r.OK {
		t.Fatalf("Cyrillic mojibake text passed, confidence=%f", r.Confidence)
	}
	if !strings.Contains(r.Reason, "Cyrillic mojibake") {
		t.Fatalf("reason = %q, want Cyrillic mojibake diagnosis", r.Reason)
	}
}

func TestGate_LowDiacriticsWithLegalKeywordStillFails(t *testing.T) {
	// The gate must not special-case legal keywords. Low-diacritic OCR remains
	// suspect even if it contains words such as "Điều".
	body := strings.Repeat("abc def ghi jkl mno pqr stu vwx ", 10) // ASCII-only words
	text := body + "Điều 1 " + body
	r := quality.DefaultGate().Assess(text)
	if r.OK {
		t.Fatalf("low-diacritic text passed because of keyword, confidence=%f reason=%q", r.Confidence, r.Reason)
	}
	if !strings.Contains(r.Reason, "low diacritic density") {
		t.Fatalf("reason = %q, want low diacritic density", r.Reason)
	}
}

func TestGate_HighWhitespace(t *testing.T) {
	// Text that is mostly whitespace — likely mis-extracted image-heavy PDF.
	text := strings.Repeat("a ", 30) + strings.Repeat("   ", 200)
	r := quality.DefaultGate().Assess(text)
	if r.Confidence >= 0.9 {
		t.Errorf("high-whitespace text has suspiciously high confidence: %f", r.Confidence)
	}
}

func TestCheck(t *testing.T) {
	good := strings.Repeat("Điều 1. Ngân hàng Nhà nước Việt Nam quy định các điều kiện sau đây. ", 10)
	if err := quality.Check(good); err != nil {
		t.Errorf("Check(good text) = %v, want nil", err)
	}

	if err := quality.Check(""); err == nil {
		t.Error("Check(empty text) = nil, want an error")
	}

	var sb strings.Builder
	for i := range 200 {
		sb.WriteRune('\ue001' + rune(i%30))
	}
	if err := quality.Check(sb.String()); err == nil {
		t.Error("Check(mojibake text) = nil, want an error")
	}
}
