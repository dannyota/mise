package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// resolveHost is the DNS lookup ValidateWebhookURL uses — a seam so tests
// can exercise the private-address rejection without real DNS.
var resolveHost = func(ctx context.Context, host string) ([]net.IPAddr, error) {
	return net.DefaultResolver.LookupIPAddr(ctx, host)
}

// validateResolveTimeout bounds the DNS lookup so a slow resolver can't
// stall the webhook-create request.
const validateResolveTimeout = 5 * time.Second

// ValidateWebhookURL checks that raw is acceptable as a webhook delivery
// endpoint per DECISIONS 19 (egress policy): HTTPS only, no literal IPs, no
// localhost/private hostnames — and, because a public-looking hostname can
// simply resolve to a private address, the hostname is resolved and every
// returned address must be public. Registration-time resolution alone does
// not close the DNS-rebinding window: the future delivery pipeline must
// re-resolve per attempt, disable redirect-following, and re-check the
// address in the dialer just before connect.
func ValidateWebhookURL(ctx context.Context, raw string) error {
	if raw == "" {
		return errors.New("webhook URL must not be empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "https" {
		return errors.New("webhook URL must use HTTPS")
	}
	// Canonicalize: a trailing dot ("example.com.") is the same DNS name but
	// would dodge suffix checks below.
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "" {
		return errors.New("webhook URL must have a hostname")
	}
	if ip := net.ParseIP(host); ip != nil {
		return errors.New("webhook URL must not use a literal IP address")
	}
	if host == "localhost" || strings.HasSuffix(host, ".local") ||
		strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".localhost") {
		return errors.New("webhook URL must not target localhost or private-range hostnames")
	}

	rctx, cancel := context.WithTimeout(ctx, validateResolveTimeout)
	defer cancel()
	addrs, err := resolveHost(rctx, host)
	if err != nil {
		return fmt.Errorf("webhook URL hostname does not resolve: %w", err)
	}
	for _, a := range addrs {
		if isDisallowedEgressIP(a.IP) {
			return fmt.Errorf("webhook URL resolves to a non-public address (%s)", a.IP)
		}
	}
	return nil
}

// isDisallowedEgressIP reports whether ip is outside the public unicast
// space webhooks may target: loopback, RFC 1918/ULA private ranges,
// link-local (169.254.0.0/16, fe80::/10), unspecified, and multicast are
// all egress-denied per DECISIONS 19.
func isDisallowedEgressIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}
