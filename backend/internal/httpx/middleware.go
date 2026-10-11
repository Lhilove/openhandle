package httpx

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"openhandle/internal/models"
	"openhandle/internal/store"
)

const (
	// SessionCookie is the name of the session cookie.
	SessionCookie = "oh_session"
	// CSRFHeader must accompany mutating requests from the browser client.
	CSRFHeader = "X-CSRF-Token"
	// OriginSecretHeader is the header an edge proxy sets so the origin can
	// reject traffic that bypassed it.
	OriginSecretHeader = "X-Origin-Secret"

	ctxUserKey    = "auth.user"
	ctxSessionKey = "auth.session"
)

// HashToken returns the hex sha256 of a raw session token.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// RequestID assigns/propagates a request id and echoes it in the response.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}
		c.Set("request_id", id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

// CORS applies an explicit origin allow-list. Credentials are never combined
// with a wildcard origin.
func CORS(allowed []string) gin.HandlerFunc {
	set := make(map[string]struct{}, len(allowed))
	for _, o := range allowed {
		set[strings.TrimRight(o, "/")] = struct{}{}
	}
	return func(c *gin.Context) {
		origin := strings.TrimRight(c.GetHeader("Origin"), "/")
		if origin != "" {
			if _, ok := set[origin]; ok {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Vary", "Origin")
				c.Header("Access-Control-Allow-Credentials", "true")
				c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
				c.Header("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token, X-Request-ID")
				c.Header("Access-Control-Max-Age", "600")
			}
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// SecurityHeaders sets conservative security headers. CSP is intentionally
// restrictive; the API returns JSON only.
func SecurityHeaders(isProd bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		c.Header("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		if isProd {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}

// RequireOriginSecret rejects requests that did not arrive through the trusted
// edge proxy. It is a no-op when no secret is configured.
//
// /healthz is deliberately exempt so the container runtime can probe liveness
// over the loopback interface without carrying the secret; it reveals nothing
// beyond "the process is up".
func RequireOriginSecret(secret string) gin.HandlerFunc {
	if secret == "" {
		return func(c *gin.Context) { c.Next() }
	}
	want := []byte(secret)
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/healthz" {
			c.Next()
			return
		}
		got := c.GetHeader(OriginSecretHeader)
		if subtle.ConstantTimeCompare([]byte(got), want) != 1 || got == "" {
			// Do not echo the header or explain which part failed.
			Fail(c, http.StatusForbidden, CodeForbidden, "This endpoint must be reached through the application.")
			return
		}
		c.Next()
	}
}

// NoStore forbids caching of API responses. Every response here is either
// personal data or data that changes on user action, and a CDN sitting in front
// of the API (for example an edge proxy that forwards to the origin) may cache
// based on the origin's own cache headers. Marking responses uncacheable keeps
// one user's session data from ever being served to another.
func NoStore() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private")
		c.Header("Pragma", "no-cache")
		c.Next()
	}
}

// Logger emits structured request logs, redacting the URL query to avoid
// leaking sensitive parameters.
func Logger(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info("request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", c.ClientIP(),
			"request_id", c.GetString("request_id"),
		)
	}
}

// Recovery converts panics into a 500 without leaking internals.
func Recovery(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic", "err", r, "request_id", c.GetString("request_id"))
				Fail(c, http.StatusInternalServerError, CodeInternal, "Something went wrong.")
			}
		}()
		c.Next()
	}
}

// Auth loads the current user from the session cookie if present. It never
// aborts; use RequireAuth for protected routes.
func Auth(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, err := c.Cookie(SessionCookie)
		if err != nil || raw == "" {
			c.Next()
			return
		}
		sess, err := st.GetSessionByTokenHash(c.Request.Context(), HashToken(raw))
		if err != nil {
			c.Next()
			return
		}
		user, err := st.GetUserByID(c.Request.Context(), sess.UserID)
		if err != nil || user.Status != "active" {
			c.Next()
			return
		}
		c.Set(ctxUserKey, user)
		c.Set(ctxSessionKey, sess)
		c.Next()
	}
}

// RequireAuth aborts with 401 unless a user is authenticated.
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := CurrentUser(c); !ok {
			Fail(c, http.StatusUnauthorized, CodeUnauthorized, "Sign in required.")
			return
		}
		c.Next()
	}
}

// RequireCSRF enforces a double-check on mutating requests: the X-CSRF-Token
// header must match the token stored with the session.
func RequireCSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}
		sess, ok := CurrentSession(c)
		if !ok {
			Fail(c, http.StatusUnauthorized, CodeUnauthorized, "Sign in required.")
			return
		}
		hdr := c.GetHeader(CSRFHeader)
		if subtle.ConstantTimeCompare([]byte(hdr), []byte(sess.CSRFToken)) != 1 || hdr == "" {
			Fail(c, http.StatusForbidden, CodeCSRF, "Invalid or missing CSRF token.")
			return
		}
		c.Next()
	}
}

// CurrentUser returns the authenticated user, if any.
func CurrentUser(c *gin.Context) (*models.User, bool) {
	v, ok := c.Get(ctxUserKey)
	if !ok {
		return nil, false
	}
	u, ok := v.(*models.User)
	return u, ok
}

// CurrentSession returns the authenticated session, if any.
func CurrentSession(c *gin.Context) (*models.Session, bool) {
	v, ok := c.Get(ctxSessionKey)
	if !ok {
		return nil, false
	}
	s, ok := v.(*models.Session)
	return s, ok
}
