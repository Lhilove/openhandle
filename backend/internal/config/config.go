package config

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

// Config holds all runtime configuration. It is loaded once at startup from the
// environment. Required production values are validated eagerly so the process
// fails clearly rather than at first request.
type Config struct {
	Env         string
	AppBaseURL  string
	APIBaseURL  string
	DatabaseURL string

	SessionSecret      string
	CORSAllowedOrigins []string
	// TrustedProxies lists CIDRs whose X-Forwarded-For header may be trusted.
	// Empty means trust none (safe default), so client IPs come from RemoteAddr.
	TrustedProxies []string
	// CookieSameSite is the SameSite attribute for the session and OAuth state
	// cookies: "lax" (default), "strict" or "none".
	//
	// "none" is only for deployments where the browser calls the API on a
	// different *registrable domain* than the app (e.g. a static frontend on
	// Vercel talking straight to api.example.com). Browsers then treat the
	// session cookie as third-party, which Safari blocks outright and Firefox
	// partitions. Prefer serving the app and the API from one origin (a reverse
	// proxy) and keeping "lax".
	CookieSameSite string
	// ClientIPMode selects how rate limiting and logged IPs derive the client
	// address: "remote" (default) uses the TCP peer and honours TRUSTED_PROXIES;
	// "forwarded" takes the left-most X-Forwarded-For entry for deployments
	// behind an edge proxy that overwrites that header (e.g. Vercel).
	ClientIPMode string
	// OriginSecret, when set, is a shared secret every request must present in
	// the X-Origin-Secret header. It lets an edge proxy (Vercel can attach the
	// header on the way in) lock the origin down to proxy-only traffic. That is
	// what makes CLIENT_IP_MODE=forwarded safe: a caller who never reaches the
	// handler cannot choose its own rate-limit bucket. Leave empty to accept
	// direct traffic, e.g. for a cross-site deployment or local testing.
	OriginSecret string

	SolanaCluster string
	SolanaRPCURL  string

	GitHubClientID     string
	GitHubClientSecret string
	RedditClientID     string
	RedditClientSecret string

	LogLevel string
}

func getenv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

// IsProduction reports whether the app is running in a production environment.
func (c *Config) IsProduction() bool {
	return strings.EqualFold(c.Env, "production")
}

// SameSiteMode maps the configured CookieSameSite value to an http.SameSite.
func (c *Config) SameSiteMode() http.SameSite {
	switch c.CookieSameSite {
	case "none":
		return http.SameSiteNoneMode
	case "strict":
		return http.SameSiteStrictMode
	default:
		return http.SameSiteLaxMode
	}
}

// CookieSecure reports whether cookies must carry the Secure attribute. Secure
// is mandatory in production and whenever SameSite=None is used, since browsers
// reject SameSite=None cookies that are not Secure.
func (c *Config) CookieSecure() bool {
	return c.IsProduction() || c.CookieSameSite == "none"
}

// TrustsForwardedClientIP reports whether the left-most X-Forwarded-For entry
// should be treated as the client address.
func (c *Config) TrustsForwardedClientIP() bool {
	return c.ClientIPMode == "forwarded"
}

// ProviderConfigured reports whether an optional OAuth provider has credentials.
func (c *Config) ProviderConfigured(provider string) bool {
	switch provider {
	case "github":
		return c.GitHubClientID != "" && c.GitHubClientSecret != ""
	case "reddit":
		return c.RedditClientID != "" && c.RedditClientSecret != ""
	default:
		return false
	}
}

// Load reads configuration from the environment, applying safe development
// defaults and validating strict production requirements.
func Load() (*Config, error) {
	loadDotEnv()
	c := &Config{
		Env:         getenv("APP_ENV", "development"),
		AppBaseURL:  strings.TrimRight(getenv("APP_BASE_URL", "http://localhost:5173"), "/"),
		APIBaseURL:  strings.TrimRight(getenv("API_BASE_URL", "http://localhost:8080"), "/"),
		DatabaseURL: getenv("DATABASE_URL", "postgres://openhandle:openhandle@localhost:5433/openhandle?sslmode=disable"),

		SessionSecret: getenv("SESSION_SECRET", ""),
		SolanaCluster: getenv("SOLANA_CLUSTER", "devnet"),
		SolanaRPCURL:  getenv("SOLANA_RPC_URL", "https://api.devnet.solana.com"),

		GitHubClientID:     getenv("GITHUB_CLIENT_ID", ""),
		GitHubClientSecret: getenv("GITHUB_CLIENT_SECRET", ""),
		RedditClientID:     getenv("REDDIT_CLIENT_ID", ""),
		RedditClientSecret: getenv("REDDIT_CLIENT_SECRET", ""),

		LogLevel: getenv("LOG_LEVEL", "info"),

		CookieSameSite: strings.ToLower(getenv("COOKIE_SAMESITE", "lax")),
		ClientIPMode:   strings.ToLower(getenv("CLIENT_IP_MODE", "remote")),
		OriginSecret:   getenv("ORIGIN_SECRET", ""),
	}

	origins := getenv("CORS_ALLOWED_ORIGINS", "http://localhost:5173")
	for _, o := range strings.Split(origins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			if o == "*" {
				return nil, fmt.Errorf("CORS_ALLOWED_ORIGINS must not contain a wildcard '*'")
			}
			c.CORSAllowedOrigins = append(c.CORSAllowedOrigins, strings.TrimRight(o, "/"))
		}
	}

	for _, p := range strings.Split(getenv("TRUSTED_PROXIES", ""), ",") {
		if p = strings.TrimSpace(p); p != "" {
			c.TrustedProxies = append(c.TrustedProxies, p)
		}
	}

	switch c.SolanaCluster {
	case "devnet", "testnet", "mainnet-beta":
	default:
		return nil, fmt.Errorf("SOLANA_CLUSTER must be one of devnet, testnet, mainnet-beta (got %q)", c.SolanaCluster)
	}

	switch c.CookieSameSite {
	case "lax", "strict", "none":
	default:
		return nil, fmt.Errorf("COOKIE_SAMESITE must be one of lax, strict, none (got %q)", c.CookieSameSite)
	}

	switch c.ClientIPMode {
	case "remote", "forwarded":
	default:
		return nil, fmt.Errorf("CLIENT_IP_MODE must be either remote or forwarded (got %q)", c.ClientIPMode)
	}

	if c.OriginSecret != "" && len(c.OriginSecret) < 16 {
		return nil, fmt.Errorf("ORIGIN_SECRET must be at least 16 characters when set")
	}
	if c.OriginSecret != "" && c.ClientIPMode != "forwarded" {
		return nil, fmt.Errorf("ORIGIN_SECRET requires CLIENT_IP_MODE=forwarded, otherwise every visitor shares the proxy's address")
	}

	if c.IsProduction() {
		if len(c.SessionSecret) < 32 {
			return nil, fmt.Errorf("SESSION_SECRET must be at least 32 characters in production")
		}
		if c.DatabaseURL == "" {
			return nil, fmt.Errorf("DATABASE_URL is required in production")
		}
		if c.AppBaseURL == "" {
			return nil, fmt.Errorf("APP_BASE_URL is required in production")
		}
		// SameSite=None cookies are dropped by every browser unless the request
		// that carries them is HTTPS, so refuse to start rather than issue
		// cookies that silently never come back.
		if c.CookieSameSite == "none" {
			for name, url := range map[string]string{"APP_BASE_URL": c.AppBaseURL, "API_BASE_URL": c.APIBaseURL} {
				if !strings.HasPrefix(url, "https://") {
					return nil, fmt.Errorf("%s must use https:// when COOKIE_SAMESITE=none (got %q)", name, url)
				}
			}
		}
	}

	if c.SessionSecret == "" {
		// Development-only fallback. Never used in production (validated above).
		c.SessionSecret = "dev-insecure-session-secret-do-not-use-in-production"
	}

	return c, nil
}
