package httpapi

import (
	"context"
	"errors"
	"net"
	"testing"
)

// stubResolver fakes DNS per hostname so the validator's resolved-address
// egress check is testable without the network. Subtests stay serial: the
// seam is a package-level var.
func stubResolver(t *testing.T) {
	t.Helper()
	orig := resolveHost
	resolveHost = func(_ context.Context, host string) ([]net.IPAddr, error) {
		switch host {
		case "private.example.com":
			return []net.IPAddr{{IP: net.ParseIP("10.0.0.5")}}, nil
		case "rebind.example.com":
			// Mixed answer: one public, one loopback — any private address
			// must reject the whole hostname.
			return []net.IPAddr{
				{IP: net.ParseIP("93.184.216.34")},
				{IP: net.ParseIP("127.0.0.1")},
			}, nil
		case "linklocal.example.com":
			return []net.IPAddr{{IP: net.ParseIP("169.254.10.10")}}, nil
		case "nxdomain.example.com":
			return nil, errors.New("no such host")
		default:
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		}
	}
	t.Cleanup(func() { resolveHost = orig })
}

func TestValidateWebhookURL(t *testing.T) {
	stubResolver(t)
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
		{"trailing dot canonicalized, localhost. rejected", "https://localhost./hook", true},
		{"empty URL rejected", "", true},
		{"no scheme rejected", "hooks.example.com/mise", true},
		{"hostname resolving to RFC1918 rejected", "https://private.example.com/hook", true},
		{"hostname with mixed public+loopback answer rejected", "https://rebind.example.com/hook", true},
		{"hostname resolving to link-local rejected", "https://linklocal.example.com/hook", true},
		{"unresolvable hostname rejected", "https://nxdomain.example.com/hook", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWebhookURL(context.Background(), tt.url)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateWebhookURL(%q) = nil, want error", tt.url)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateWebhookURL(%q) = %v, want nil", tt.url, err)
			}
		})
	}
}
