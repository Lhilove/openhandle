// Package identity defines a pluggable interface for verified social accounts.
// Providers are only enabled when official OAuth credentials are configured.
// Verifications always come from the provider's authoritative API response --
// never from a user-supplied URL, and never by scraping.
package identity

import "context"

// Profile is a verified identity as returned by the provider's own API.
type Profile struct {
	Subject  string // provider's stable user id
	Username string // display handle only
}

// Provider is implemented by each supported identity provider.
type Provider interface {
	// Name is the provider slug, e.g. "github".
	Name() string
	// AuthURL builds the authorization redirect URL.
	AuthURL(state, redirectURI string) string
	// Exchange swaps an authorization code for the authenticated profile.
	Exchange(ctx context.Context, code, redirectURI string) (*Profile, error)
}
