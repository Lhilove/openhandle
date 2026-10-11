package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"openhandle/internal/httpx"
	"openhandle/internal/identity"
	"openhandle/internal/store"
)

const oauthStateCookie = "oh_oauth_state"

func (s *Server) providerFor(name string) identity.Provider {
	switch name {
	case "github":
		if !s.Cfg.ProviderConfigured("github") {
			return nil
		}
		return identity.NewGitHub(s.Cfg.GitHubClientID, s.Cfg.GitHubClientSecret)
	default:
		// reddit (and others) are declared but not enabled in this release.
		return nil
	}
}

// ListIdentities returns the owner's verified identities plus which providers
// are actually configured, so the client can show an honest state.
func (s *Server) ListIdentities(c *gin.Context) {
	user, _ := httpx.CurrentUser(c)
	ids, err := s.Store.ListIdentities(c.Request.Context(), user.ID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not load identities.")
		return
	}
	out := make([]identityDTO, 0, len(ids))
	for _, ic := range ids {
		out = append(out, identityDTO{Provider: ic.Provider, Username: ic.ProviderUsername, Verified: true})
	}
	c.JSON(http.StatusOK, gin.H{
		"identities": out,
		"providers": []gin.H{
			{"provider": "github", "configured": s.Cfg.ProviderConfigured("github")},
			{"provider": "reddit", "configured": s.Cfg.ProviderConfigured("reddit")},
		},
		"note": "Verified means the provider's API confirmed control of the account. " +
			"It is not government or legal identity verification.",
	})
}

func (s *Server) oauthRedirectURI(provider string) string {
	return s.Cfg.APIBaseURL + "/api/v1/identities/" + provider + "/callback"
}

func (s *Server) signState(provider, state string) string {
	mac := hmac.New(sha256.New, []byte(s.Cfg.SessionSecret))
	mac.Write([]byte(provider + "|" + state))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return state + "." + sig
}

func (s *Server) verifyState(provider, signed string) (string, bool) {
	parts := strings.SplitN(signed, ".", 2)
	if len(parts) != 2 {
		return "", false
	}
	state, sig := parts[0], parts[1]
	mac := hmac.New(sha256.New, []byte(s.Cfg.SessionSecret))
	mac.Write([]byte(provider + "|" + state))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(sig), []byte(want)) != 1 {
		return "", false
	}
	return state, true
}

// ConnectIdentity starts an OAuth flow. Returns 501 when the provider is not
// configured rather than pretending a connection is possible.
func (s *Server) ConnectIdentity(c *gin.Context) {
	provider := c.Param("provider")
	p := s.providerFor(provider)
	if p == nil {
		httpx.Fail(c, http.StatusNotImplemented, httpx.CodeProviderDisabled,
			"This provider is not configured on this server.")
		return
	}
	state := randomToken(16)
	c.SetSameSite(s.Cfg.SameSiteMode())
	c.SetCookie(oauthStateCookie, s.signState(provider, state), 600, "/", "", s.Cfg.CookieSecure(), true)
	c.Redirect(http.StatusFound, p.AuthURL(state, s.oauthRedirectURI(provider)))
}

// CallbackIdentity finishes an OAuth flow and records a verified identity.
func (s *Server) CallbackIdentity(c *gin.Context) {
	user, ok := httpx.CurrentUser(c)
	if !ok {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "Sign in required.")
		return
	}
	provider := c.Param("provider")
	p := s.providerFor(provider)
	if p == nil {
		httpx.Fail(c, http.StatusNotImplemented, httpx.CodeProviderDisabled, "This provider is not configured on this server.")
		return
	}

	// Validate state (CSRF for the OAuth dance).
	signed, err := c.Cookie(oauthStateCookie)
	if err != nil || signed == "" {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidRequest, "Missing OAuth state.")
		return
	}
	// The state cookie is single-use: clear it before validating so a replayed
	// callback cannot reuse it, whatever the outcome below.
	c.SetSameSite(s.Cfg.SameSiteMode())
	c.SetCookie(oauthStateCookie, "", -1, "/", "", s.Cfg.CookieSecure(), true)
	state, okState := s.verifyState(provider, signed)
	if !okState || subtle.ConstantTimeCompare([]byte(state), []byte(c.Query("state"))) != 1 {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidRequest, "Invalid OAuth state.")
		return
	}

	code := c.Query("code")
	if code == "" {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidRequest, "Missing authorization code.")
		return
	}

	ctx := c.Request.Context()
	profile, err := p.Exchange(ctx, code, s.oauthRedirectURI(provider))
	if err != nil {
		s.Log.Error("oauth exchange", "provider", provider, "err", err, "request_id", c.GetString("request_id"))
		httpx.Fail(c, http.StatusBadGateway, httpx.CodeInternal, "Could not verify the account with the provider.")
		return
	}
	if err := s.Store.UpsertIdentity(ctx, user.ID, provider, profile.Subject, profile.Username); err != nil {
		if errors.Is(err, store.ErrConflict) {
			httpx.Fail(c, http.StatusConflict, httpx.CodeConflict, "That account is already linked elsewhere.")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not save the identity.")
		return
	}
	s.Store.Audit(ctx, &user.ID, "identity.verified", "identity", provider, map[string]any{"username": profile.Username})
	c.Redirect(http.StatusFound, s.Cfg.AppBaseURL+"/app/identities?connected="+provider)
}

// DeleteIdentity disconnects a provider.
func (s *Server) DeleteIdentity(c *gin.Context) {
	user, _ := httpx.CurrentUser(c)
	provider := c.Param("provider")
	if err := s.Store.DeleteIdentity(c.Request.Context(), user.ID, provider); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "No such connection.")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not disconnect.")
		return
	}
	s.Store.Audit(c.Request.Context(), &user.ID, "identity.disconnected", "identity", provider, nil)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
