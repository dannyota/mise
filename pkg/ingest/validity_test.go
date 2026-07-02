package ingest_test

import (
	"testing"
	"time"

	"danny.vn/mise/pkg/ingest"
)

func TestMapValidity(t *testing.T) {
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	past := now.AddDate(0, -1, 0)
	future := now.AddDate(0, 1, 0)

	tests := []struct {
		name        string
		rawStatus   string
		effectiveAt time.Time
		now         time.Time
		want        string
	}{
		// in_force family: status_class token, VN label, agclom code, vbpl code —
		// case-insensitive, date is irrelevant once the status is recognized.
		{"status_class in_force", "in_force", future, now, ingest.StatusInForce},
		{"vn label con hieu luc", "Còn hiệu lực", time.Time{}, now, ingest.StatusInForce},
		{"agclom PRINCIPAL", "PRINCIPAL", time.Time{}, now, ingest.StatusInForce},
		{"agclom principal lowercase", "principal", time.Time{}, now, ingest.StatusInForce},
		{"vbpl code CHL", "CHL", time.Time{}, now, ingest.StatusInForce},
		{"vbpl code chl lowercase", "chl", time.Time{}, now, ingest.StatusInForce},

		// amended family: partial expiry.
		{"status_class partial", "partial", time.Time{}, now, ingest.StatusAmended},
		{"vn label het hieu luc mot phan", "Hết hiệu lực một phần", time.Time{}, now, ingest.StatusAmended},
		{"vbpl code HHL1P", "HHL1P", time.Time{}, now, ingest.StatusAmended},
		{"vbpl code HHL1PHAN", "HHL1PHAN", time.Time{}, now, ingest.StatusAmended},

		// repealed family: full expiry (status_class "expired"), agclom REPEALED,
		// and suspended (TDHL) — mise never presents suspended text as current.
		{"status_class expired", "expired", time.Time{}, now, ingest.StatusRepealed},
		{"agclom REPEALED", "REPEALED", time.Time{}, now, ingest.StatusRepealed},
		{"vbpl code HHL", "HHL", time.Time{}, now, ingest.StatusRepealed},
		{"vbpl code hhl lowercase", "hhl", time.Time{}, now, ingest.StatusRepealed},
		{"status_class suspended", "suspended", time.Time{}, now, ingest.StatusRepealed},
		{"vbpl code TDHL (tam dung hieu luc)", "TDHL", time.Time{}, now, ingest.StatusRepealed},
		{"vbpl code tdhl lowercase", "tdhl", time.Time{}, now, ingest.StatusRepealed},

		// not_yet_effective family: CCHL/TNHL/CHUACOHIEULUC all classify "not_yet"
		// in banhmi's statusCodeToClass — TNHL is NOT the suspended bucket despite
		// sounding similar to TDHL; keep them distinct.
		{"status_class not_yet", "not_yet", time.Time{}, now, ingest.StatusNotYetEffective},
		{"vn label chua co hieu luc", "Chưa có hiệu lực", time.Time{}, now, ingest.StatusNotYetEffective},
		{"vbpl code CCHL", "CCHL", time.Time{}, now, ingest.StatusNotYetEffective},
		{"vbpl code cchl lowercase", "cchl", time.Time{}, now, ingest.StatusNotYetEffective},
		{"vbpl code TNHL", "TNHL", time.Time{}, now, ingest.StatusNotYetEffective},
		{"vbpl code CHUACOHIEULUC", "CHUACOHIEULUC", time.Time{}, now, ingest.StatusNotYetEffective},

		// unknown/empty falls back to the effective-date heuristic: in_force when
		// effectiveAt is zero or not after now, else not_yet_effective.
		{"empty status, zero effectiveAt", "", time.Time{}, now, ingest.StatusInForce},
		{"empty status, past effectiveAt", "", past, now, ingest.StatusInForce},
		{"empty status, effectiveAt equals now", "", now, now, ingest.StatusInForce},
		{"empty status, future effectiveAt", "", future, now, ingest.StatusNotYetEffective},
		{"whitespace-only status treated as empty", "   ", future, now, ingest.StatusNotYetEffective},
		{"unrecognized status, zero effectiveAt", "bogus_code", time.Time{}, now, ingest.StatusInForce},
		{"unrecognized status, past effectiveAt", "bogus_code", past, now, ingest.StatusInForce},
		{"unrecognized status, future effectiveAt", "bogus_code", future, now, ingest.StatusNotYetEffective},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ingest.MapValidity(tt.rawStatus, tt.effectiveAt, tt.now)
			if got != tt.want {
				t.Errorf("MapValidity(%q, %v, %v) = %q, want %q", tt.rawStatus, tt.effectiveAt, tt.now, got, tt.want)
			}
		})
	}
}

func TestEventKind(t *testing.T) {
	tests := []struct {
		name    string
		relType string
		want    string
		wantOK  bool
	}{
		// vbpl's actual wired labels (pkg/ingest/vbpl relationLabel defaults).
		{"vbpl amends_supplements", "amends_supplements", ingest.StatusAmended, true},
		{"vbpl replaces", "replaces", ingest.StatusSuperseded, true},
		{"vbpl legal_basis is informational", "legal_basis", "", false},

		// agclom's raw subsidiary-legislation codes — informational, not amending.
		{"agclom pua", "pua", "", false},
		{"agclom pub", "pub", "", false},

		// forward-compatible families documented in banhmi's SOURCES.md
		// referenceType map (1/8=abrogates, 6=corrects, 7=consolidates) even
		// though no wired crawler emits these labels yet.
		{"abrogates maps to repealed", "abrogates", ingest.StatusRepealed, true},
		{"repeals maps to repealed", "repeals", ingest.StatusRepealed, true},
		{"corrects maps to amended", "corrects", ingest.StatusAmended, true},
		{"consolidates is informational", "consolidates", "", false},
		{"generic amends synonym", "amends", ingest.StatusAmended, true},
		{"generic supersedes synonym", "supersedes", ingest.StatusSuperseded, true},

		// unmapped vbpl referenceType fallback and case-insensitivity.
		{"unmapped vbpl_type_N fallback", "vbpl_type_99", "", false},
		{"empty type", "", "", false},
		{"case-insensitive REPLACES", "REPLACES", ingest.StatusSuperseded, true},
		{"whitespace padded", "  replaces  ", ingest.StatusSuperseded, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKind, gotOK := ingest.EventKind(ingest.Relation{Type: tt.relType})
			if gotKind != tt.want || gotOK != tt.wantOK {
				t.Errorf("EventKind(%q) = (%q, %v), want (%q, %v)", tt.relType, gotKind, gotOK, tt.want, tt.wantOK)
			}
		})
	}
}

func TestTransition(t *testing.T) {
	tests := []struct {
		name      string
		current   string
		eventKind string
		want      string
	}{
		// The brief's canonical rows, verbatim.
		{"in_force amended", ingest.StatusInForce, ingest.StatusAmended, ingest.StatusAmended},
		{"in_force superseded", ingest.StatusInForce, ingest.StatusSuperseded, ingest.StatusSuperseded},
		{"in_force repealed", ingest.StatusInForce, ingest.StatusRepealed, ingest.StatusRepealed},
		{"amended repealed", ingest.StatusAmended, ingest.StatusRepealed, ingest.StatusRepealed},
		{"repealed terminal under amended", ingest.StatusRepealed, ingest.StatusAmended, ingest.StatusRepealed},
		{"repealed terminal under superseded", ingest.StatusRepealed, ingest.StatusSuperseded, ingest.StatusRepealed},
		{"repealed terminal under repealed", ingest.StatusRepealed, ingest.StatusRepealed, ingest.StatusRepealed},
		{"not_yet_effective repealed", ingest.StatusNotYetEffective, ingest.StatusRepealed, ingest.StatusRepealed},

		// Reasonable, documented extensions beyond the brief's explicit rows:
		// repeal absorbs from every state, and among non-repeal events the
		// event's own kind becomes the new current status.
		{"amended amended stays amended", ingest.StatusAmended, ingest.StatusAmended, ingest.StatusAmended},
		{"amended superseded", ingest.StatusAmended, ingest.StatusSuperseded, ingest.StatusSuperseded},
		{"superseded repealed", ingest.StatusSuperseded, ingest.StatusRepealed, ingest.StatusRepealed},
		{"not_yet_effective amended", ingest.StatusNotYetEffective, ingest.StatusAmended, ingest.StatusAmended},
		{"not_yet_effective superseded", ingest.StatusNotYetEffective, ingest.StatusSuperseded, ingest.StatusSuperseded},

		// Unrecognized eventKind is a defensive no-op — current is unchanged
		// rather than adopting a garbage status string.
		{"unrecognized eventKind is a no-op", ingest.StatusInForce, "bogus", ingest.StatusInForce},
		{"empty eventKind is a no-op", ingest.StatusAmended, "", ingest.StatusAmended},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ingest.Transition(tt.current, tt.eventKind)
			if got != tt.want {
				t.Errorf("Transition(%q, %q) = %q, want %q", tt.current, tt.eventKind, got, tt.want)
			}
		})
	}
}

func TestTransitionAt(t *testing.T) {
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	past := now.AddDate(0, 0, -1)
	future := now.AddDate(0, 0, 1)

	tests := []struct {
		name      string
		current   string
		eventKind string
		eventDate time.Time
		now       time.Time
		want      string
	}{
		{
			"past-dated event applies",
			ingest.StatusInForce, ingest.StatusRepealed, past, now,
			ingest.StatusRepealed,
		},
		{
			"same-instant event applies (not strictly future)",
			ingest.StatusInForce, ingest.StatusRepealed, now, now,
			ingest.StatusRepealed,
		},
		{
			"future-dated event produces no transition",
			ingest.StatusInForce, ingest.StatusRepealed, future, now,
			ingest.StatusInForce,
		},
		{
			"future-dated amended leaves current untouched",
			ingest.StatusInForce, ingest.StatusAmended, future, now,
			ingest.StatusInForce,
		},
		{
			"repealed stays repealed regardless of future event",
			ingest.StatusRepealed, ingest.StatusAmended, future, now,
			ingest.StatusRepealed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ingest.TransitionAt(tt.current, tt.eventKind, tt.eventDate, tt.now)
			if got != tt.want {
				t.Errorf("TransitionAt(%q, %q, %v, %v) = %q, want %q",
					tt.current, tt.eventKind, tt.eventDate, tt.now, got, tt.want)
			}
		})
	}
}
