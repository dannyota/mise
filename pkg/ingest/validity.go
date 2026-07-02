package ingest

import (
	"strings"
	"time"
)

// mise's validity_status enum (migrations/002_document_tables.sql). MapValidity
// only ever produces StatusInForce, StatusAmended, StatusRepealed, or
// StatusNotYetEffective; StatusSuperseded is reachable only via an amendment
// event (EventKind + Transition) — a source never self-reports "superseded".
const (
	StatusInForce         = "in_force"
	StatusAmended         = "amended"
	StatusSuperseded      = "superseded"
	StatusRepealed        = "repealed"
	StatusNotYetEffective = "not_yet_effective"
)

// statusTokens classifies every raw validity signal mise's current sources
// emit into mise's validity_status enum, keyed lowercase/trimmed. It covers
// three namespaces, verified against banhmi (pkg/pipeline/normalize_activities.go
// statusCodeToClass + deploy/seed/validity_status.csv — the canonical source
// for "status_class" values), since a source may emit any of them:
//
//   - banhmi's normalized status_class tokens (in_force/partial/expired/
//     not_yet/suspended) — for callers that pre-classify before calling here.
//   - the raw source codes DiscoveredDoc.Status actually carries today: vbpl's
//     effStatus.code (CHL/HHL/HHL1P/HHL1PHAN/CCHL/TNHL/CHUACOHIEULUC/TDHL) and
//     agclom's actStatus() (PRINCIPAL/REPEALED).
//   - the Vietnamese status labels vbpl's effStatus.name carries alongside the
//     code, for defensiveness.
//
// Two corrections versus a naive reading of banhmi: (1) TNHL classifies as
// "not_yet" in banhmi's statusCodeToClass, not "suspended" — despite sounding
// like a suspension code, it is grouped with CCHL/CHUACOHIEULUC. (2) TDHL
// ("Tạm dừng hiệu lực") is the actual "suspended" code; "Ngưng hiệu lực" does
// not appear anywhere in banhmi and was not used here.
var statusTokens = map[string]string{
	// status_class tokens.
	"in_force":  StatusInForce,
	"partial":   StatusAmended,
	"expired":   StatusRepealed,
	"suspended": StatusRepealed,
	"not_yet":   StatusNotYetEffective,

	// vbpl raw effStatus.code values.
	"chl":           StatusInForce,
	"hhl":           StatusRepealed,
	"hhl1p":         StatusAmended,
	"hhl1phan":      StatusAmended,
	"cchl":          StatusNotYetEffective,
	"tnhl":          StatusNotYetEffective,
	"chuacohieuluc": StatusNotYetEffective,
	"tdhl":          StatusRepealed,

	// agclom raw actStatus() values.
	"principal": StatusInForce,
	"repealed":  StatusRepealed,

	// Vietnamese labels (vbpl effStatus.name), verified verbatim against
	// banhmi's code comments and test fixtures.
	"còn hiệu lực":          StatusInForce,
	"hết hiệu lực một phần": StatusAmended,
	"hết hiệu lực":          StatusRepealed,
	"chưa có hiệu lực":      StatusNotYetEffective,
	"tạm dừng hiệu lực":     StatusRepealed,
}

// MapValidity classifies a source-native validity signal into mise's
// validity_status enum. rawStatus is matched case-insensitively (trimmed)
// against statusTokens; an unrecognized or empty rawStatus falls back to
// effectiveAt: in_force when effectiveAt is zero or not after now, else
// not_yet_effective. The fallback never guesses "amended" or "repealed" from
// silence — those require positive evidence.
func MapValidity(rawStatus string, effectiveAt, now time.Time) string {
	key := strings.ToLower(strings.TrimSpace(rawStatus))
	if status, ok := statusTokens[key]; ok {
		return status
	}
	if effectiveAt.IsZero() || !effectiveAt.After(now) {
		return StatusInForce
	}
	return StatusNotYetEffective
}

// relationEventKinds classifies a source Relation.Type label into the
// amendment-event family it belongs to. Populated from the labels mise's
// wired crawlers actually emit today (vbpl's amends_supplements/replaces/
// legal_basis, agclom's pua/pub) plus the broader referenceType families
// banhmi documents (docs/design/SOURCES.md: "referenceType int map:
// 10=amends, 12=replaces, 7=consolidates, 3=basis, 6=corrects, 1/8=abrogates")
// for forward compatibility once those codes get a wired string label — no
// crawler emits "abrogates"/"repeals"/"corrects" yet, so those branches are
// currently unreached in production but are cheap, documented, and tested.
var relationEventKinds = map[string]string{
	"amends":             StatusAmended,
	"amend":              StatusAmended,
	"amends_supplements": StatusAmended,
	"corrects":           StatusAmended,
	"correction":         StatusAmended,

	"replaces":   StatusSuperseded,
	"replace":    StatusSuperseded,
	"supersedes": StatusSuperseded,
	"supersede":  StatusSuperseded,
	"superseded": StatusSuperseded,

	"repeals":   StatusRepealed,
	"repeal":    StatusRepealed,
	"abrogates": StatusRepealed,
	"abrogate":  StatusRepealed,
}

// EventKind classifies rel into a mise amendment-event kind (StatusAmended,
// StatusSuperseded, or StatusRepealed). ok is false when rel is an
// informational graph edge that does not change the target document's
// validity — a "legal_basis" citation, an agclom "pua"/"pub" subsidiary-
// legislation pointer, a "consolidates" reprint marker, an unmapped vbpl
// "vbpl_type_N" fallback label, or an empty Type.
func EventKind(rel Relation) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(rel.Type))
	kind, ok := relationEventKinds[key]
	return kind, ok
}

// Transition applies one amendment event of kind eventKind to a document
// currently at status current, and returns the new status. Two absorbing
// rules dominate: StatusRepealed is terminal (current == StatusRepealed
// stays StatusRepealed regardless of eventKind), and an eventKind of
// StatusRepealed always wins (any current moves to StatusRepealed). Between
// non-repeal events the latest event's own kind becomes the new current
// status. An eventKind that is not one of StatusAmended/StatusSuperseded/
// StatusRepealed is a defensive no-op — current is returned unchanged rather
// than adopting an unrecognized status string.
func Transition(current, eventKind string) string {
	if current == StatusRepealed || eventKind == StatusRepealed {
		return StatusRepealed
	}
	switch eventKind {
	case StatusAmended, StatusSuperseded:
		return eventKind
	default:
		return current
	}
}

// TransitionAt is Transition gated on eventDate: a future-dated event
// (eventDate after now) is recorded as an event row by the caller but must
// not yet change the document's current validity_status, so current is
// returned unchanged. eventDate equal to now counts as not future and the
// transition applies.
func TransitionAt(current, eventKind string, eventDate, now time.Time) string {
	if eventDate.After(now) {
		return current
	}
	return Transition(current, eventKind)
}
