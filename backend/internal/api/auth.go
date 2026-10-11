package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"openhandle/internal/httpx"
	"openhandle/internal/models"
	"openhandle/internal/store"
	"openhandle/internal/validate"
)

// dummyHash is compared against when a login email is unknown, to keep the
// response time roughly constant and avoid user enumeration.
var dummyHash = func() []byte {
	h, _ := bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), bcrypt.DefaultCost)
	return h
}()

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Register creates an account and starts a session.
func (s *Server) Register(c *gin.Context) {
	var req credentials
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidRequest, "A valid JSON body is required.")
		return
	}
	email, err := validate.ValidateEmail(req.Email)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidRequest, err.Error())
		return
	}
	if err := validate.ValidatePassword(req.Password); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidRequest, err.Error())
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not create the account.")
		return
	}
	user, err := s.Store.CreateUser(c.Request.Context(), email, string(hash))
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			httpx.Fail(c, http.StatusConflict, httpx.CodeEmailTaken, "An account with this email already exists.")
			return
		}
		s.Log.Error("register", "err", err, "request_id", c.GetString("request_id"))
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not create the account.")
		return
	}

	sess, raw, err := s.createSession(c, user.ID)
	if err != nil {
		s.Log.Error("register session", "err", err, "request_id", c.GetString("request_id"))
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not start a session.")
		return
	}
	s.Store.Audit(c.Request.Context(), &user.ID, "user.registered", "user", user.ID.String(), nil)
	s.setSessionCookie(c, raw, sess.ExpiresAt)
	c.JSON(http.StatusCreated, s.buildMe(c, user, sess))
}

// Login authenticates and starts a session.
func (s *Server) Login(c *gin.Context) {
	var req credentials
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidRequest, "A valid JSON body is required.")
		return
	}
	email, err := validate.ValidateEmail(req.Email)
	if err != nil {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeInvalidCredential, "Email or password is incorrect.")
		return
	}

	user, err := s.Store.GetUserByEmail(c.Request.Context(), email)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(req.Password))
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeInvalidCredential, "Email or password is incorrect.")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not sign you in.")
		return
	}
	if user.Status != "active" {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeInvalidCredential, "Email or password is incorrect.")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeInvalidCredential, "Email or password is incorrect.")
		return
	}

	sess, raw, err := s.createSession(c, user.ID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not start a session.")
		return
	}
	s.Store.Audit(c.Request.Context(), &user.ID, "user.login", "user", user.ID.String(), nil)
	s.setSessionCookie(c, raw, sess.ExpiresAt)
	c.JSON(http.StatusOK, s.buildMe(c, user, sess))
}

// Logout invalidates the current session.
func (s *Server) Logout(c *gin.Context) {
	if raw, err := c.Cookie(httpx.SessionCookie); err == nil && raw != "" {
		_ = s.Store.DeleteSession(c.Request.Context(), httpx.HashToken(raw))
	}
	if user, ok := httpx.CurrentUser(c); ok {
		s.Store.Audit(c.Request.Context(), &user.ID, "user.logout", "user", user.ID.String(), nil)
	}
	s.clearSessionCookie(c)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Me returns the current user, handle, wallets and identities, plus a CSRF
// token the client must echo on mutating requests.
func (s *Server) Me(c *gin.Context) {
	user, ok := httpx.CurrentUser(c)
	if !ok {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized, "Sign in required.")
		return
	}
	sess, _ := httpx.CurrentSession(c)
	c.JSON(http.StatusOK, s.buildMe(c, user, sess))
}

// DeleteAccount deactivates and scrubs the authenticated account, then clears
// the session cookie. On-chain records are permanent and are not deleted.
func (s *Server) DeleteAccount(c *gin.Context) {
	user, _ := httpx.CurrentUser(c)
	ctx := c.Request.Context()
	if err := s.Store.DeleteAccount(ctx, user.ID); err != nil {
		s.Log.Error("delete account", "err", err, "request_id", c.GetString("request_id"))
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not delete the account.")
		return
	}
	s.clearSessionCookie(c)
	c.JSON(http.StatusOK, gin.H{
		"ok":   true,
		"note": "Your account data was removed and your handle released. On-chain transactions remain public and cannot be deleted.",
	})
}

// buildMe assembles the owner-only payload. It never includes internal ids
// beyond the user's own resources, and includes the session CSRF token.
func (s *Server) buildMe(c *gin.Context, user *models.User, sess *models.Session) meDTO {
	ctx := c.Request.Context()
	out := meDTO{
		Email:      user.Email,
		CreatedAt:  user.CreatedAt,
		Wallets:    []walletPrivateDTO{},
		Identities: []identityDTO{},
	}
	if sess != nil {
		out.CSRFToken = sess.CSRFToken
	}
	if h, err := s.Store.GetActiveHandleForUser(ctx, user.ID); err == nil {
		out.Handle = s.handlePrivate(h)
	}
	if ws, err := s.Store.ListWallets(ctx, user.ID); err == nil {
		for _, w := range ws {
			out.Wallets = append(out.Wallets, s.walletPrivate(w))
		}
	}
	if ids, err := s.Store.ListIdentities(ctx, user.ID); err == nil {
		for _, ic := range ids {
			out.Identities = append(out.Identities, identityDTO{Provider: ic.Provider, Username: ic.ProviderUsername, Verified: true})
		}
	}
	return out
}
