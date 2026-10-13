package config

import (
	"net/http"
	"strings"
	"testing"
)

func TestSameSiteModeMapping(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  http.SameSite
	}{
		{"lax", http.SameSiteLaxMode},
		{"none", http.SameSiteNoneMode},
		{"strict", http.SameSiteStrictMode},
		{"", http.SameSiteLaxMode}, // unresolved config behaves like the default
	} {
		c := &Config{CookieSameSite: tc.value}
		if got := c.SameSiteMode(); got != tc.want {
			t.Fatalf("CookieSameSite=%q: got %v, want %v", tc.value, got, tc.want)
		}
	}
}

func TestCookieSecure(t *testing.T) {
	if (&Config{Env: "development", CookieSameSite: "lax"}).CookieSecure() {
		t.Fatal("development cookies must not be Secure")
	}
	if !(&Config{Env: "production", CookieSameSite: "lax"}).CookieSecure() {
		t.Fatal("production cookies must be Secure")
	}
	// Secure is mandatory with SameSite=None regardless of environment, since
	// browsers reject the combination otherwise.
	if !(&Config{Env: "development", CookieSameSite: "none"}).CookieSecure() {
		t.Fatal("SameSite=None requires Secure in every environment")
	}
}

func TestTrustsForwardedClientIP(t *testing.T) {
	if (&Config{ClientIPMode: "remote"}).TrustsForwardedClientIP() {
		t.Fatal("remote mode must not trust a forwarded header")
	}
	if !(&Config{ClientIPMode: "forwarded"}).TrustsForwardedClientIP() {
		t.Fatal("forwarded mode must trust the edge proxy header")
	}
}

func TestLoadRejectsInvalidModes(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("COOKIE_SAMESITE", "laxx")
	if _, err := Load(); err == nil {
		t.Fatal("expected an error for an unknown COOKIE_SAMESITE value")
	}

	t.Setenv("COOKIE_SAMESITE", "lax")
	t.Setenv("CLIENT_IP_MODE", "trust-everything")
	if _, err := Load(); err == nil {
		t.Fatal("expected an error for an unknown CLIENT_IP_MODE value")
	}
}

// TestProductionRejectsInsecureCrossSiteCookies locks in the fail-fast rule: a
// SameSite=None cookie can only ever be delivered over HTTPS, so starting
// production with http origins would issue cookies that are always dropped.
func TestProductionRejectsInsecureCrossSiteCookies(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("CLIENT_IP_MODE", "forwarded")
	t.Setenv("COOKIE_SAMESITE", "none")
	t.Setenv("SESSION_SECRET", strings.Repeat("a", 32))
	t.Setenv("DATABASE_URL", "postgres://user:pass@db:5432/openhandle?sslmode=disable")

	t.Setenv("APP_BASE_URL", "http://app.example.com")
	t.Setenv("API_BASE_URL", "https://api.example.com")
	if _, err := Load(); err == nil {
		t.Fatal("expected an error for a http APP_BASE_URL with COOKIE_SAMESITE=none")
	}

	t.Setenv("APP_BASE_URL", "https://app.example.com")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("https origins must be accepted: %v", err)
	}
	if !cfg.CookieSecure() || cfg.SameSiteMode() != http.SameSiteNoneMode {
		t.Fatal("cross-site production config must produce Secure SameSite=None cookies")
	}
}

// TestOriginSecretValidation keeps the origin lockdown from being configured in
// a way that would break the deployment it is meant to protect.
func TestOriginSecretValidation(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("COOKIE_SAMESITE", "lax")

	t.Setenv("ORIGIN_SECRET", "too-short")
	t.Setenv("CLIENT_IP_MODE", "forwarded")
	if _, err := Load(); err == nil {
		t.Fatal("expected an error for a short ORIGIN_SECRET")
	}

	t.Setenv("ORIGIN_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("CLIENT_IP_MODE", "remote")
	if _, err := Load(); err == nil {
		t.Fatal("ORIGIN_SECRET without CLIENT_IP_MODE=forwarded would lump every visitor into one rate-limit bucket")
	}

	t.Setenv("CLIENT_IP_MODE", "forwarded")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("a valid origin lockdown must load: %v", err)
	}
	if cfg.OriginSecret == "" {
		t.Fatal("origin secret was not retained")
	}
}

// TestLoadRejectsMismatchedSolanaClusterAndRPC locks in the mainnet-migration
// guard: a cluster label and an RPC endpoint on different networks would make
// the API publish one network while verifying payments against another.
func TestLoadRejectsMismatchedSolanaClusterAndRPC(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("SOLANA_CLUSTER", "mainnet-beta")
	// The endpoint carries a credential, so the failure must not echo it.
	t.Setenv("SOLANA_RPC_URL", "https://devnet.helius-rpc.com/?api-key=secret-key-value")

	_, err := Load()
	if err == nil {
		t.Fatal("expected an error when SOLANA_CLUSTER and SOLANA_RPC_URL name different networks")
	}
	if strings.Contains(err.Error(), "secret-key-value") {
		t.Fatalf("the error must not echo the RPC URL: %v", err)
	}

	// The same endpoint is accepted once both sides agree.
	t.Setenv("SOLANA_RPC_URL", "https://mainnet.helius-rpc.com/?api-key=test")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("a matching mainnet configuration must load: %v", err)
	}
	if cfg.SolanaCluster != "mainnet-beta" {
		t.Fatalf("got cluster %q, want mainnet-beta", cfg.SolanaCluster)
	}

	// An endpoint that is not recognised says nothing about its network, so it is
	// allowed (a private node or an internal proxy, for instance).
	t.Setenv("SOLANA_RPC_URL", "https://rpc.internal.example/rpc")
	if _, err := Load(); err != nil {
		t.Fatalf("an unrecognised RPC host must be accepted: %v", err)
	}
}

// TestLoadDefaultsAreSafe documents the default posture: same-site cookies and
// no trust in a forwarded header.
func TestLoadDefaultsAreSafe(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.CookieSameSite != "lax" || cfg.ClientIPMode != "remote" {
		t.Fatalf("unexpected defaults: samesite=%q clientip=%q", cfg.CookieSameSite, cfg.ClientIPMode)
	}
	if len(cfg.TrustedProxies) != 0 {
		t.Fatalf("no proxy should be trusted by default, got %v", cfg.TrustedProxies)
	}
}
