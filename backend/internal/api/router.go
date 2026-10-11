package api

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"openhandle/internal/config"
	"openhandle/internal/httpx"
	"openhandle/internal/rpc"
	"openhandle/internal/store"
)

// Router builds the HTTP engine with middleware and routes.
func Router(cfg *config.Config, st *store.Store, log *slog.Logger, rpcClient *rpc.Client) *gin.Engine {
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}
	srv := NewServer(cfg, st, log, rpcClient)

	// Rate limiters. Limits are documented in README/API docs.
	authLimiter := httpx.NewLimiter(10, 5)      // login/register per IP per minute
	availLimiter := httpx.NewLimiter(60, 20)    // handle availability
	challengeLimiter := httpx.NewLimiter(20, 5) // wallet challenges
	txLimiter := httpx.NewLimiter(30, 10)       // transaction submissions
	resolveLimiter := httpx.NewLimiter(120, 40) // public resolution

	r := gin.New()
	// Client IP resolution. By default no proxy is trusted: a spoofed
	// X-Forwarded-For must not be able to bypass IP-based rate limiting, so
	// operators behind a real proxy list its CIDRs in TRUSTED_PROXIES.
	// CLIENT_IP_MODE=forwarded is for an edge proxy that overwrites
	// X-Forwarded-For with the visitor's address (Vercel does): Gin then walks
	// the header right-to-left and, with every peer trusted, lands on the
	// left-most entry -- the real client -- instead of the proxy's own IP.
	switch {
	case cfg.TrustsForwardedClientIP():
		_ = r.SetTrustedProxies([]string{"0.0.0.0/0", "::/0"})
	case len(cfg.TrustedProxies) == 0:
		_ = r.SetTrustedProxies(nil)
	default:
		_ = r.SetTrustedProxies(cfg.TrustedProxies)
	}

	r.Use(
		httpx.RequestID(),
		httpx.Recovery(log),
		httpx.Logger(log),
		httpx.SecurityHeaders(cfg.IsProduction()),
		httpx.RequireOriginSecret(cfg.OriginSecret),
		httpx.NoStore(),
		httpx.CORS(cfg.CORSAllowedOrigins),
		httpx.Auth(st),
	)

	r.NoRoute(func(c *gin.Context) {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "Not found.")
	})

	// ---- operations (unversioned) ----
	r.GET("/healthz", srv.Healthz)
	r.GET("/readyz", srv.Readyz)

	v1 := r.Group("/api/v1")

	// ---- auth ----
	auth := v1.Group("/auth")
	auth.POST("/register", authLimiter.Middleware("register"), srv.Register)
	auth.POST("/login", authLimiter.Middleware("login"), srv.Login)
	auth.POST("/logout", srv.Logout)

	// ---- current user ----
	me := v1.Group("/me", httpx.RequireAuth(), httpx.RequireCSRF())
	me.GET("", srv.Me)
	me.GET("/wallets", srv.ListWallets)
	me.GET("/transactions", srv.ListMyTransactions)
	me.GET("/identities", srv.ListIdentities)
	me.PATCH("/profile", srv.UpdateProfile)
	me.PATCH("/handle", srv.UpdateHandle)
	me.DELETE("/wallets/:wallet_id", srv.DeleteWallet)
	me.DELETE("", srv.DeleteAccount)

	// Me (read-only variants that do not require CSRF, since they are GET).
	// (already covered above; RequireCSRF passes GETs through)

	// ---- handles ----
	v1.GET("/handles/availability", availLimiter.Middleware("availability"), srv.HandleAvailability)
	v1.GET("/handles/:handle", resolveLimiter.Middleware("resolve"), srv.ResolveHandle)

	protected := v1.Group("", httpx.RequireAuth(), httpx.RequireCSRF())
	protected.POST("/handles", srv.ClaimHandle)
	protected.POST("/wallet-challenges", challengeLimiter.Middleware("challenge"), srv.CreateWalletChallenge)
	protected.POST("/wallets", srv.AddWallet)
	protected.POST("/wallets/verify", srv.VerifyWallet)
	protected.POST("/transactions", txLimiter.Middleware("tx"), srv.CreateTransaction)
	protected.GET("/transactions/:signature", srv.GetTransaction)

	// ---- identities ----
	protected.POST("/identities/:provider/connect", srv.ConnectIdentity)
	protected.DELETE("/identities/:provider", srv.DeleteIdentity)
	// Browser-navigable OAuth start: an authenticated GET that redirects.
	v1.GET("/identities/:provider/start", httpx.RequireAuth(), srv.ConnectIdentity)
	// OAuth callback is a browser navigation (GET); CSRF is not applicable, the
	// signed state cookie provides the CSRF guarantee.
	v1.GET("/identities/:provider/callback", srv.CallbackIdentity)

	return r
}
