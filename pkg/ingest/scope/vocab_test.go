package scope

import "testing"

func TestNewVNReferenceVocab(t *testing.T) {
	m := NewVN()
	if m.Empty() {
		t.Fatal("NewVN() returned an empty matcher")
	}
	tests := []struct {
		name    string
		number  string
		title   string
		inScope bool
	}{
		{
			"NHNN infosec circular", "09/2020/TT-NHNN",
			"Quy định về an toàn hệ thống thông tin trong hoạt động ngân hàng", true,
		},
		{
			"e-transaction law, non-bank issuer", "20/2023/QH15",
			"Luật Giao dịch điện tử", true,
		},
		{
			"weak term with NHNN signal from số ký hiệu", "43/2023/TT-NHNN",
			"Ứng dụng công nghệ thông tin", true,
		},
		{
			"health-IT circular stays out", "10/2024/TT-BYT",
			"Ứng dụng công nghệ thông tin trong khám bệnh, chữa bệnh", false,
		},
		{
			"interest-rate circular stays out", "12/2024/TT-NHNN",
			"Quy định về lãi suất tái cấp vốn của Ngân hàng Nhà nước", false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.Match(tt.number, tt.title, ""); got.InScope != tt.inScope {
				t.Errorf("Match(%q, %q).InScope = %v, want %v (matched=%v)",
					tt.number, tt.title, got.InScope, tt.inScope, got.Matched)
			}
		})
	}
}

func TestNewMYReferenceVocab(t *testing.T) {
	m := NewMY()
	if m.Empty() {
		t.Fatal("NewMY() returned an empty matcher")
	}
	tests := []struct {
		name    string
		title   string
		inScope bool
	}{
		{"RMiT policy document", "Risk Management in Technology (RMiT)", true},
		{"personal data act", "Personal Data Protection Act 2010", true},
		{"e-money guideline", "Guideline on Electronic Money (E-Money)", true},
		{"weak term with banking signal", "Technology outsourcing by a licensed bank", true},
		{"weak term without banking signal stays out", "National Technology Strategy for Agriculture", false},
		{"generic act stays out", "Stamp Act 1949", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.Match("", tt.title, ""); got.InScope != tt.inScope {
				t.Errorf("Match(%q).InScope = %v, want %v (matched=%v)",
					tt.title, got.InScope, tt.inScope, got.Matched)
			}
		})
	}
}

func TestForJurisdiction(t *testing.T) {
	if For("vn").Empty() {
		t.Error(`For("vn") should carry the VN vocabulary`)
	}
	if For("my").Empty() {
		t.Error(`For("my") should carry the MY vocabulary`)
	}
	if !For("xx").Empty() {
		t.Error(`For("xx") should be empty (fail-open marker) for unknown jurisdictions`)
	}
}
