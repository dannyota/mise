// Package quality gates extracted legal text for trustworthiness before it is
// parsed or embedded. It is a deterministic, local check — no model or cloud
// call — scoring mojibake (TCVN3/VNI PUA runes, UTF-8/CP1251 double-encode
// markers), diacritic density, and whitespace ratio against tuned thresholds.
// This is a light port of banhmi's pkg/extract quality gate: the operator-
// tunable GateConfig/Assess mechanics are kept byte-faithful; the
// settings-map loader and source-placeholder sniffing (a crawl-layer concern,
// not a text-quality one) are not.
package quality

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Default gate thresholds, matching banhmi's tuned defaults.
const (
	defaultMaxBadRatio         = 0.01
	defaultMinDiacriticDensity = 0.02
	defaultMinLetters          = 50
	defaultPassThreshold       = 0.6
	defaultMaxWhitespaceRatio  = 0.40
	defaultMaxPUARatio         = 0.005
	defaultMaxMojibakeRatio    = 0.005
	// Cyrillic (U+0400-U+04FF) never appears in Latin-script legal text (VN,
	// MY). Any non-trivial density is a UTF-8->CP1251->UTF-8 double-encode; the
	// absolute floor keeps a stray quoted character from tripping the gate.
	cyrillicMinCount = 8
	cyrillicMinRatio = 0.005
)

// GateConfig holds the tunable thresholds for the content quality gate. Use
// DefaultGate for the compiled-in defaults.
type GateConfig struct {
	// MaxBadRatio is the maximum fraction of replacement/control characters
	// tolerated (e.g. 0.01 = 1%).
	MaxBadRatio float64
	// MinDiacriticDensity is the minimum ratio of non-ASCII letters to all
	// letters below which real Vietnamese is unlikely (e.g. 0.02).
	MinDiacriticDensity float64
	// MinLetters is the minimum letter count below which text is too short to
	// judge (e.g. 50).
	MinLetters int
	// PassThreshold is the minimum confidence score for the gate to pass
	// (e.g. 0.6).
	PassThreshold float64
	// MaxWhitespaceRatio is the maximum fraction of whitespace runes above
	// which the document is probably image-heavy or badly extracted (e.g.
	// 0.40).
	MaxWhitespaceRatio float64
	// MaxPUARatio is the maximum fraction of Unicode Private-Use-Area runes
	// (U+E000-U+F8FF), a strong mojibake signal from TCVN3/VNI legacy fonts
	// (e.g. 0.005).
	MaxPUARatio float64
}

// DefaultGate returns the compiled-in default thresholds.
func DefaultGate() GateConfig {
	return GateConfig{
		MaxBadRatio:         defaultMaxBadRatio,
		MinDiacriticDensity: defaultMinDiacriticDensity,
		MinLetters:          defaultMinLetters,
		PassThreshold:       defaultPassThreshold,
		MaxWhitespaceRatio:  defaultMaxWhitespaceRatio,
		MaxPUARatio:         defaultMaxPUARatio,
	}
}

// AssessResult is the detailed verdict from GateConfig.Assess.
type AssessResult struct {
	Confidence float64 // 0.0-1.0
	OK         bool    // true -> text passes the gate
	Reason     string  // short human-readable diagnosis (non-empty when !OK)
}

// Assess scores extracted Vietnamese text against the tuned thresholds and
// decides whether to trust it. It is deterministic - no model or cloud call.
//
// Signals checked:
//   - Bad/replacement character ratio (strong negative).
//   - Diacritic density: real VN text is non-ASCII-letter-dense.
//   - Whitespace ratio: very high whitespace -> image-heavy or mis-extracted.
//   - PUA rune ratio: TCVN3/VNI mojibake surfaces in U+E000-U+F8FF.
//   - UTF-8 and Cyrillic (CP1251 double-encode) mojibake marker runes.
func (g GateConfig) Assess(text string) AssessResult {
	text = norm.NFC.String(text)

	sig := scanSignals(text)
	if sig.total == 0 {
		return AssessResult{Reason: "empty text"}
	}
	return sig.assess(g)
}

// textSignals holds the raw rune counts scanned from one Assess call.
type textSignals struct {
	total, letters, nonASCIILetters  int
	bad, ws, pua, mojibake, cyrillic int
}

func scanSignals(text string) textSignals {
	var sig textSignals
	for _, r := range text {
		sig.total++
		if r == '�' || (unicode.IsControl(r) && r != '\n' && r != '\t' && r != '\r' && r != '\f') {
			sig.bad++
		}
		if unicode.IsLetter(r) {
			sig.letters++
			if r > unicode.MaxASCII {
				sig.nonASCIILetters++
			}
		}
		if unicode.IsSpace(r) {
			sig.ws++
		}
		if r >= 0xE000 && r <= 0xF8FF {
			sig.pua++
		}
		if isMojibakeMarker(r) {
			sig.mojibake++
		}
		if r >= 0x0400 && r <= 0x04FF {
			sig.cyrillic++
		}
	}
	return sig
}

// assess scores the scanned signals against g's thresholds.
func (sig textSignals) assess(g GateConfig) AssessResult {
	badRatio := float64(sig.bad) / float64(sig.total)
	wsRatio := float64(sig.ws) / float64(sig.total)
	puaRatio := float64(sig.pua) / float64(sig.total)
	mojibakeRatio := float64(sig.mojibake) / float64(sig.total)
	cyrillicMojibake := sig.cyrillic >= cyrillicMinCount && float64(sig.cyrillic)/float64(sig.total) >= cyrillicMinRatio
	diacriticDensity := 0.0
	if sig.letters > 0 {
		diacriticDensity = float64(sig.nonASCIILetters) / float64(sig.letters)
	}

	confidence := 1.0
	var reasons []string
	addPenalty := func(cond bool, penalty float64, reason string) {
		if cond {
			confidence -= penalty
			reasons = append(reasons, reason)
		}
	}
	addPenalty(badRatio > g.MaxBadRatio, badRatio*5.0, "bad chars")
	addPenalty(sig.letters < g.MinLetters, 0.3, "too few letters")
	addPenalty(wsRatio > g.MaxWhitespaceRatio, 0.3, "high whitespace")
	addPenalty(puaRatio > g.MaxPUARatio, 0.5, "PUA runes (TCVN3/VNI mojibake)")
	addPenalty(sig.mojibake >= 8 && mojibakeRatio > defaultMaxMojibakeRatio, 0.7, "UTF-8 mojibake markers")
	addPenalty(cyrillicMojibake, 0.7, "Cyrillic mojibake (CP1251 double-encode)")
	addPenalty(sig.letters >= 200 && diacriticDensity < g.MinDiacriticDensity, 0.5, "low diacritic density")
	confidence = clamp01(confidence)

	hardFail := badRatio >= g.MaxBadRatio ||
		puaRatio > g.MaxPUARatio ||
		(sig.mojibake >= 8 && mojibakeRatio > defaultMaxMojibakeRatio) ||
		cyrillicMojibake ||
		sig.letters < g.MinLetters
	ok := confidence >= g.PassThreshold && !hardFail

	reason := ""
	if !ok {
		reason = strings.Join(reasons, "; ")
		if reason == "" {
			reason = "below confidence threshold"
		}
	}
	return AssessResult{Confidence: confidence, OK: ok, Reason: reason}
}

func isMojibakeMarker(r rune) bool {
	return strings.ContainsRune("√∆·ªƒ∫≠‚ÄØ", r)
}

// Check runs the default gate against text and reports why it failed as an
// error, or nil when the text passes. This is the entry point downstream
// ingest/parse code should call; use GateConfig.Assess directly when the
// confidence score or operator-tuned thresholds are needed.
func Check(text string) error {
	r := DefaultGate().Assess(text)
	if !r.OK {
		return fmt.Errorf("quality: %s (confidence %.2f)", r.Reason, r.Confidence)
	}
	return nil
}

func clamp01(x float64) float64 {
	switch {
	case x < 0:
		return 0
	case x > 1:
		return 1
	default:
		return x
	}
}
