package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"danny.vn/mise/pkg/ingest"
	"danny.vn/mise/pkg/ingest/scope"
)

func TestDiscoveryHashFingerprintsDiscoveryFields(t *testing.T) {
	base := ingest.DiscoveredDoc{
		Number: "11/2026/TT-NHNN", Title: "Quy định về an toàn hệ thống thông tin",
		DetailURL: "https://vbpl.vn/van-ban/chi-tiet/1", DocType: "Thông tư",
	}
	dup := base
	if discoveryHash(dup) != discoveryHash(base) {
		t.Error("discoveryHash must be deterministic")
	}
	if len(discoveryHash(base)) != 64 {
		t.Errorf("discoveryHash length = %d, want 64 hex chars", len(discoveryHash(base)))
	}
	for name, mutate := range map[string]func(d *ingest.DiscoveredDoc){
		"number":             func(d *ingest.DiscoveredDoc) { d.Number = "12/2026/TT-NHNN" },
		"title":              func(d *ingest.DiscoveredDoc) { d.Title = "khác" },
		"detailURL":          func(d *ingest.DiscoveredDoc) { d.DetailURL = "https://vbpl.vn/van-ban/chi-tiet/2" },
		"docType":            func(d *ingest.DiscoveredDoc) { d.DocType = "Nghị định" },
		"contentFingerprint": func(d *ingest.DiscoveredDoc) { d.ContentFingerprint = "deadbeef" },
	} {
		changed := base
		mutate(&changed)
		if discoveryHash(changed) == discoveryHash(base) {
			t.Errorf("discoveryHash must change when %s changes", name)
		}
	}
	// Fields outside the fingerprint (e.g. status) must NOT reopen a document.
	same := base
	same.Status = "HHL"
	if discoveryHash(same) != discoveryHash(base) {
		t.Error("discoveryHash must ignore non-fingerprint fields")
	}
}

// TestDiscoveryHashLegacyRecipeUnchanged pins the empty-fingerprint hash to
// the original four-field recipe byte-for-byte: existing ledger rows were
// written with it, and any drift would re-open every completed document of
// every source that never sets ContentFingerprint.
func TestDiscoveryHashLegacyRecipeUnchanged(t *testing.T) {
	d := ingest.DiscoveredDoc{
		Number: "11/2026/TT-NHNN", Title: "Quy định về an toàn hệ thống thông tin",
		DetailURL: "https://vbpl.vn/van-ban/chi-tiet/1", DocType: "Thông tư",
	}
	sum := sha256.Sum256([]byte(d.Number + "|" + d.Title + "|" + d.DetailURL + "|" + string(d.DocType)))
	if got, want := discoveryHash(d), hex.EncodeToString(sum[:]); got != want {
		t.Fatalf("discoveryHash without fingerprint = %s, want legacy recipe %s", got, want)
	}
}

func TestInScope(t *testing.T) {
	m := scope.New(
		[]string{"an toàn thông tin"}, nil,
		[]string{"công nghệ thông tin"},
		[]string{"ngân hàng"},
	)
	inScopeDoc := ingest.DiscoveredDoc{Number: "09/2020/TT-NHNN", Title: "Quy định về an toàn thông tin"}
	outDoc := ingest.DiscoveredDoc{Number: "200/2014/TT-BTC", Title: "Hướng dẫn chế độ kế toán doanh nghiệp"}

	if in, matched := inScope(m, "", inScopeDoc); !in || len(matched) == 0 {
		t.Errorf("inScope(matching doc) = %v, %v — want in scope with provenance", in, matched)
	}
	if in, _ := inScope(m, "", outDoc); in {
		t.Error("inScope(non-matching doc) = true, want out of scope")
	}
	if in, matched := inScope(m, "an ninh mạng", outDoc); !in || len(matched) != 1 || matched[0] != "an ninh mạng" {
		t.Errorf("inScope with keyword = %v, %v — a keyword-selected doc is in scope by construction", in, matched)
	}
	if in, _ := inScope(scope.New(nil, nil, nil, nil), "", outDoc); !in {
		t.Error("inScope(empty matcher) = false, want fail-open")
	}
	if in, _ := inScope(nil, "", outDoc); !in {
		t.Error("inScope(nil matcher) = false, want fail-open")
	}
}
