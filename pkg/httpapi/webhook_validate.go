package httpapi

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidateWebhookURL checks that raw is acceptable as a webhook delivery
// endpoint per DECISIONS 19 (egress policy): HTTPS only, no literal IPs,
// no localhost/private-range hostnames. Returns nil if valid.
func ValidateWebhookURL(raw string) error {
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
	host := u.Hostname()
	if host == "" {
		return errors.New("webhook URL must have a hostname")
	}
	// Reject literal IPs.
	if ip := net.ParseIP(host); ip != nil {
		return errors.New("webhook URL must not use a literal IP address")
	}
	// Reject localhost and common private hostnames.
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".local") ||
		strings.HasSuffix(lower, ".internal") || strings.HasSuffix(lower, ".localhost") {
		return errors.New("webhook URL must not target localhost or private-range hostnames")
	}
	return nil
}
