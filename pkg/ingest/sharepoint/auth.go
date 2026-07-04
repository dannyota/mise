package sharepoint

import (
	"errors"
	"net/http"
)

// StaticAuth applies static credentials from adopter config. It supports two
// modes: cookie-based auth (FedAuth/rtFa value as a Cookie header) and bearer-
// token auth (Authorization: Bearer <token>). The adopter chooses based on their
// scoped account provisioning.
type StaticAuth struct {
	Cookie string // FedAuth/rtFa cookie string; takes precedence when both set
	Bearer string // OAuth2 bearer token
}

// Apply sets the auth headers/cookies on the outgoing request. Returns an
// error when both Cookie and Bearer are empty — an unauthenticated request
// must never be sent to SharePoint (fail-closed).
func (a *StaticAuth) Apply(req *http.Request) error {
	if a.Cookie != "" {
		req.Header.Set("Cookie", a.Cookie)
		return nil
	}
	if a.Bearer != "" {
		req.Header.Set("Authorization", "Bearer "+a.Bearer)
		return nil
	}
	return errors.New(
		"sharepoint: no credentials configured" +
			" (both Cookie and Bearer are empty)",
	)
}

// FakeAuth is a test-only authenticator that does nothing.
type FakeAuth struct{}

// Apply is a no-op for tests.
func (FakeAuth) Apply(_ *http.Request) error { return nil }
