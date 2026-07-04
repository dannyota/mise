package httpapi

import "testing"

func TestValidateWebhookURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid HTTPS URL", "https://hooks.example.com/mise", false},
		{"valid HTTPS with path and port", "https://hooks.example.com:8443/webhook/v1", false},
		{"HTTP rejected", "http://hooks.example.com/mise", true},
		{"literal IPv4 rejected", "https://192.168.1.1/hook", true},
		{"literal IPv6 rejected", "https://[::1]/hook", true},
		{"localhost rejected", "https://localhost/hook", true},
		{"LOCALHOST rejected (case insensitive)", "https://LOCALHOST/hook", true},
		{".local suffix rejected", "https://myhost.local/hook", true},
		{".internal suffix rejected", "https://service.internal/hook", true},
		{".localhost suffix rejected", "https://app.localhost/hook", true},
		{"empty URL rejected", "", true},
		{"no scheme rejected", "hooks.example.com/mise", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateWebhookURL(tt.url)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateWebhookURL(%q) = nil, want error", tt.url)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateWebhookURL(%q) = %v, want nil", tt.url, err)
			}
		})
	}
}
