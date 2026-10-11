package api

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"openhandle/internal/config"
	"openhandle/internal/httpx"
	"openhandle/internal/models"
	"openhandle/internal/rpc"
	"openhandle/internal/store"
)

const sessionTTL = 30 * 24 * time.Hour

// Server bundles dependencies for HTTP handlers.
type Server struct {
	Cfg   *config.Config
	Store *store.Store
	Log   *slog.Logger
	RPC   *rpc.Client
}

// NewServer constructs a Server.
func NewServer(cfg *config.Config, st *store.Store, log *slog.Logger, rpcClient *rpc.Client) *Server {
	return &Server{Cfg: cfg, Store: st, Log: log, RPC: rpcClient}
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure is unrecoverable and must never be silenced.
		panic("crypto/rand failure: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func (s *Server) setSessionCookie(c *gin.Context, token string, expires time.Time) {
	c.SetSameSite(s.Cfg.SameSiteMode())
	c.SetCookie(httpx.SessionCookie, token, int(time.Until(expires).Seconds()), "/", "", s.Cfg.CookieSecure(), true)
}

func (s *Server) clearSessionCookie(c *gin.Context) {
	c.SetSameSite(s.Cfg.SameSiteMode())
	c.SetCookie(httpx.SessionCookie, "", -1, "/", "", s.Cfg.CookieSecure(), true)
}

// createSession issues a new server-side session and returns the raw cookie
// token (only its hash is persisted).
func (s *Server) createSession(c *gin.Context, userID uuid.UUID) (*models.Session, string, error) {
	raw := randomToken(32)
	csrf := randomToken(32)
	exp := time.Now().Add(sessionTTL)
	id, err := s.Store.CreateSession(c.Request.Context(), userID, httpx.HashToken(raw), csrf,
		c.Request.UserAgent(), c.ClientIP(), exp)
	if err != nil {
		return nil, "", err
	}
	return &models.Session{ID: id, UserID: userID, CSRFToken: csrf, ExpiresAt: exp}, raw, nil
}
