package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"openhandle/internal/httpx"
	"openhandle/internal/store"
	"openhandle/internal/validate"
	"openhandle/internal/wallet"
)

const challengeTTL = 5 * time.Minute

func (s *Server) challengeDomain() string {
	u, err := url.Parse(s.Cfg.AppBaseURL)
	if err != nil || u.Host == "" {
		return "localhost"
	}
	return u.Host
}

// ListWallets returns the owner's wallets.
func (s *Server) ListWallets(c *gin.Context) {
	user, _ := httpx.CurrentUser(c)
	ws, err := s.Store.ListWallets(c.Request.Context(), user.ID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not load wallets.")
		return
	}
	out := make([]walletPrivateDTO, 0, len(ws))
	for _, w := range ws {
		out = append(out, s.walletPrivate(w))
	}
	c.JSON(http.StatusOK, gin.H{"wallets": out})
}

// CreateWalletChallenge issues a single-use, short-lived, domain-bound message
// for the user's wallet to sign.
func (s *Server) CreateWalletChallenge(c *gin.Context) {
	user, _ := httpx.CurrentUser(c)

	var req struct {
		Address string `json:"address"`
		Network string `json:"network"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidRequest, "A valid JSON body is required.")
		return
	}
	if !validate.IsValidSolanaAddress(req.Address) {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeWalletInvalid, "That is not a valid Solana address.")
		return
	}
	network := req.Network
	if network == "" {
		network = "solana"
	}

	nonce := randomToken(16)
	domain := s.challengeDomain()
	now := time.Now().UTC()
	expires := now.Add(challengeTTL)
	message := fmt.Sprintf(
		"OpenHandle wallet verification\n\n"+
			"Domain: %s\n"+
			"Wallet: %s\n"+
			"Network: %s\n"+
			"Nonce: %s\n"+
			"Issued At: %s\n"+
			"Expires At: %s\n\n"+
			"This signature proves you control this wallet. It authorizes no transaction.",
		domain, req.Address, network, nonce,
		now.Format(time.RFC3339), expires.Format(time.RFC3339))

	if err := s.Store.CreateChallenge(c.Request.Context(), user.ID, req.Address, httpx.HashToken(nonce), domain, message, expires); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not create a challenge.")
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"message":    message,
		"nonce":      nonce,
		"domain":     domain,
		"network":    network,
		"expires_at": expires,
	})
}

// VerifyWallet verifies a signed challenge and stores the wallet as ownership
// verified. The signature is checked server-side; the client's claim alone is
// never trusted.
func (s *Server) VerifyWallet(c *gin.Context) {
	user, _ := httpx.CurrentUser(c)

	var req struct {
		Address   string `json:"address"`
		Nonce     string `json:"nonce"`
		Signature string `json:"signature"`
		Network   string `json:"network"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidRequest, "A valid JSON body is required.")
		return
	}
	if !validate.IsValidSolanaAddress(req.Address) || req.Nonce == "" || req.Signature == "" {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeWalletInvalid, "Address, nonce and signature are required.")
		return
	}
	network := req.Network
	if network == "" {
		network = "solana"
	}

	ctx := c.Request.Context()

	// Atomically consume the challenge (single-use, unexpired, bound to this
	// user, wallet and domain). This prevents replay.
	message, err := s.Store.ConsumeChallenge(ctx, user.ID, req.Address, httpx.HashToken(req.Nonce), s.challengeDomain())
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeChallengeInvalid, "That challenge is expired, already used, or unknown.")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not verify the challenge.")
		return
	}

	if err := wallet.VerifySignature(req.Address, []byte(message), req.Signature); err != nil {
		s.Store.Audit(ctx, &user.ID, "wallet.verify.failed", "wallet", req.Address, nil)
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeWalletInvalid, "The signature did not match this wallet.")
		return
	}

	verifiedAt := time.Now()
	w, err := s.Store.AddWallet(ctx, user.ID, network, req.Address, &verifiedAt)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			httpx.Fail(c, http.StatusConflict, httpx.CodeConflict, "That wallet is already linked to another account.")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not save the wallet.")
		return
	}
	s.Store.Audit(ctx, &user.ID, "wallet.verified", "wallet", w.ID.String(), map[string]any{"network": network})
	c.JSON(http.StatusOK, s.walletPrivate(*w))
}

// AddWallet attaches an address without ownership verification. The wallet is
// clearly marked unverified.
func (s *Server) AddWallet(c *gin.Context) {
	user, _ := httpx.CurrentUser(c)

	var req struct {
		Address string `json:"address"`
		Network string `json:"network"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidRequest, "A valid JSON body is required.")
		return
	}
	if !validate.IsValidSolanaAddress(req.Address) {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeWalletInvalid, "That is not a valid Solana address.")
		return
	}
	network := req.Network
	if network == "" {
		network = "solana"
	}
	w, err := s.Store.AddWallet(c.Request.Context(), user.ID, network, req.Address, nil)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			httpx.Fail(c, http.StatusConflict, httpx.CodeConflict, "That wallet is already linked to another account.")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not save the wallet.")
		return
	}
	c.JSON(http.StatusCreated, s.walletPrivate(*w))
}

// DeleteWallet removes one of the owner's wallets (object-level authz enforced
// by the scoped query).
func (s *Server) DeleteWallet(c *gin.Context) {
	user, _ := httpx.CurrentUser(c)
	id, err := uuid.Parse(c.Param("wallet_id"))
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidRequest, "Invalid wallet id.")
		return
	}
	if err := s.Store.DeleteWallet(c.Request.Context(), user.ID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "Wallet not found.")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not remove the wallet.")
		return
	}
	s.Store.Audit(c.Request.Context(), &user.ID, "wallet.removed", "wallet", id.String(), nil)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
