package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"openhandle/internal/config"
)

// These tests protect behaviour that only matters once the API is deployed
// behind a proxy, but that fails silently if it regresses: cookie attributes,
// cacheability of personal data, and how rate limiting derives a client IP.
// They require PostgreSQL like the other integration tests.

// doWith is `do` plus extra request headers (for X-Forwarded-For).
func (c *client) doWith(method, path string, body any, csrf bool, headers map[string]string) *httptest.ResponseRecorder {
	c.t.Helper()
	var rdr io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rdr = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if c.cookie != "" {
		req.Header.Set("Cookie", c.cookie)
	}
	if csrf {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	return rec
}

// registerWith registers an account, forwarding the given X-Forwarded-For.
func (c *client) registerWith(email, password, forwardedFor string) *httptest.ResponseRecorder {
	c.t.Helper()
	return c.doWith(http.MethodPost, "/api/v1/auth/register",
		map[string]string{"email": email, "password": password}, false,
		map[string]string{"X-Forwarded-For": forwardedFor})
}

func mustSessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == "oh_session" {
			return ck
		}
	}
	t.Fatalf("no session cookie in response: %v", rec.Result().Header)
	return nil
}

// TestSessionCookieAttributes checks that the session cookie carries the
// configured SameSite policy and the Secure flag whenever it is required.
// A SameSite=None cookie without Secure is dropped by every browser, which
// would break sign-in with no server-side error to point at.
func TestSessionCookieAttributes(t *testing.T) {
	t.Run("default is lax and not secure outside production", func(t *testing.T) {
		e := newEnv(t)
		c := e.newClient()
		rec := c.registerWith("cookie-lax@example.com", "supersecret123", "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("register: status %d body %s", rec.Code, rec.Body.String())
		}
		ck := mustSessionCookie(t, rec)
		if ck.SameSite != http.SameSiteLaxMode {
			t.Fatalf("expected SameSite=Lax, got %v", ck.SameSite)
		}
		if ck.Secure {
			t.Fatal("a dev-mode cookie must not be Secure or browsers drop it over http")
		}
		if !ck.HttpOnly {
			t.Fatal("session cookie must be HttpOnly")
		}
		if ck.Path != "/" {
			t.Fatalf("unexpected cookie path %q", ck.Path)
		}
		if ck.Domain != "" {
			// A Domain attribute cannot be first-party on a proxied host such
			// as *.vercel.app, which is on the public suffix list.
			t.Fatalf("session cookie must stay host-only, got Domain=%q", ck.Domain)
		}
	})

	t.Run("cross-site mode is None and Secure", func(t *testing.T) {
		e := newEnvWith(t, func(cfg *config.Config) { cfg.CookieSameSite = "none" })
		c := e.newClient()
		rec := c.registerWith("cookie-none@example.com", "supersecret123", "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("register: status %d body %s", rec.Code, rec.Body.String())
		}
		ck := mustSessionCookie(t, rec)
		if ck.SameSite != http.SameSiteNoneMode {
			t.Fatalf("expected SameSite=None, got %v", ck.SameSite)
		}
		if !ck.Secure {
			t.Fatal("SameSite=None without Secure is dropped by browsers")
		}
	})

	t.Run("logout expires the cookie", func(t *testing.T) {
		e := newEnv(t)
		c := e.newClient()
		c.register("cookie-logout@example.com", "supersecret123")
		rec := c.do(http.MethodPost, "/api/v1/auth/logout", nil, false)
		ck := mustSessionCookie(t, rec)
		if ck.Value != "" || ck.MaxAge >= 0 {
			t.Fatalf("logout must expire the cookie, got value=%q maxAge=%d", ck.Value, ck.MaxAge)
		}
	})
}

// TestResponsesAreNotCacheable guards against an edge cache in front of the API
// (Vercel proxies /api/* to the origin) storing one user's personal data and
// then serving it to another.
func TestResponsesAreNotCacheable(t *testing.T) {
	e := newEnv(t)
	c := e.newClient()
	c.register("cache@example.com", "supersecret123")

	for _, tc := range []struct {
		name string
		rec  *httptest.ResponseRecorder
	}{
		{"authenticated me", c.do(http.MethodGet, "/api/v1/me", nil, false)},
		{"unauthenticated me", e.newClient().do(http.MethodGet, "/api/v1/me", nil, false)},
		{"public resolution", e.newClient().do(http.MethodGet, "/api/v1/handles/nobody", nil, false)},
		{"health", e.newClient().do(http.MethodGet, "/healthz", nil, false)},
	} {
		got := tc.rec.Header().Get("Cache-Control")
		if !strings.Contains(got, "no-store") {
			t.Fatalf("%s: expected Cache-Control to forbid caching, got %q", tc.name, got)
		}
	}
}

// TestOriginSecretGate covers the optional origin lockdown: when a secret is
// configured, only requests that carry it (i.e. that came through the edge
// proxy which attaches it) reach a handler. That is what makes trusting a
// forwarded client IP safe.
func TestOriginSecretGate(t *testing.T) {
	const secret = "9f2c4e6a8b0d1f3a5c7e9b1d3f5a7c9e"
	e := newEnvWith(t, func(cfg *config.Config) {
		cfg.OriginSecret = secret
		cfg.ClientIPMode = "forwarded"
	})
	c := e.newClient()

	rec := c.do(http.MethodGet, "/api/v1/handles/someone", nil, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a direct request must be refused, got %d", rec.Code)
	}

	rec = c.doWith(http.MethodGet, "/api/v1/handles/someone", nil, false,
		map[string]string{"X-Origin-Secret": "not-the-secret-value-0000"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a wrong secret must be refused, got %d", rec.Code)
	}

	// The container liveness probe talks to the process directly.
	if rec := c.do(http.MethodGet, "/healthz", nil, false); rec.Code != http.StatusOK {
		t.Fatalf("healthz must stay reachable for the runtime probe, got %d", rec.Code)
	}

	rec = c.doWith(http.MethodGet, "/api/v1/handles/someone", nil, false,
		map[string]string{"X-Origin-Secret": secret})
	if rec.Code == http.StatusForbidden {
		t.Fatal("the correct secret must pass the gate")
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected the handler's own 404 for an unknown handle, got %d", rec.Code)
	}
}

// TestForwardedClientIPKeysRateLimit covers the deployment in which an edge
// proxy overwrites X-Forwarded-For. Without CLIENT_IP_MODE=forwarded every
// visitor shares the proxy's own address and the auth limiter throttles them
// all together.
func TestForwardedClientIPKeysRateLimit(t *testing.T) {
	const proxyIP = "76.76.21.21" // stands in for the edge proxy's own address

	// The auth limiter allows a burst of 5 per minute per key; a wrong password
	// on a non-existent account still consumes it.
	attempts := func(c *client, forwardedFor string) int {
		return c.doWith(http.MethodPost, "/api/v1/auth/login",
			map[string]string{"email": "nobody@example.com", "password": "wrong-password-123"}, false,
			map[string]string{"X-Forwarded-For": forwardedFor}).Code
	}

	t.Run("forwarded mode separates clients behind one proxy", func(t *testing.T) {
		e := newEnvWith(t, func(cfg *config.Config) { cfg.ClientIPMode = "forwarded" })
		first := "203.0.113.7, " + proxyIP
		for i := 0; i < 5; i++ {
			if code := attempts(e.newClient(), first); code != http.StatusUnauthorized {
				t.Fatalf("attempt %d: expected 401 before the limit, got %d", i, code)
			}
		}
		if code := attempts(e.newClient(), first); code != http.StatusTooManyRequests {
			t.Fatalf("expected 429 once the burst is spent, got %d", code)
		}
		// A different visitor arriving through the same proxy must not inherit
		// the exhausted bucket.
		if code := attempts(e.newClient(), "203.0.113.8, "+proxyIP); code != http.StatusUnauthorized {
			t.Fatalf("a second client must not be throttled by the first, got %d", code)
		}
	})

	t.Run("remote mode ignores a spoofed header", func(t *testing.T) {
		e := newEnv(t)
		for i := 0; i < 5; i++ {
			attempts(e.newClient(), "198.51.100.1, "+proxyIP)
		}
		// Every request shares one socket peer, so rotating the header must not
		// reset the limiter.
		if code := attempts(e.newClient(), "198.51.100.99, "+proxyIP); code != http.StatusTooManyRequests {
			t.Fatalf("a spoofed X-Forwarded-For must not bypass the limiter, got %d", code)
		}
	})
}
