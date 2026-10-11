package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"openhandle/internal/httpx"
	"openhandle/internal/store"
	"openhandle/internal/validate"
)

// HandleAvailability checks whether a handle can be claimed. Public, rate-limited.
func (s *Server) HandleAvailability(c *gin.Context) {
	raw := c.Query("handle")
	handle, err := validate.ValidateHandle(raw)
	if err != nil {
		// An invalid handle is simply not available, with a reason.
		c.JSON(http.StatusOK, gin.H{"handle": validate.NormalizeHandle(raw), "available": false, "reason": "invalid"})
		return
	}
	exists, err := s.Store.HandleExists(c.Request.Context(), handle)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not check availability.")
		return
	}
	c.JSON(http.StatusOK, gin.H{"handle": handle, "available": !exists, "reason": reasonFor(!exists)})
}

func reasonFor(available bool) string {
	if available {
		return ""
	}
	return "taken"
}

// ClaimHandle claims a handle for the authenticated user.
func (s *Server) ClaimHandle(c *gin.Context) {
	user, _ := httpx.CurrentUser(c)

	var req struct {
		Handle string `json:"handle"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidRequest, "A valid JSON body is required.")
		return
	}
	handle, err := validate.ValidateHandle(req.Handle)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeHandleInvalid, err.Error())
		return
	}

	ctx := c.Request.Context()

	// Handles are immutable in the MVP: an account may hold exactly one.
	if existing, err := s.Store.GetActiveHandleForUser(ctx, user.ID); err == nil && existing != nil {
		httpx.Fail(c, http.StatusConflict, httpx.CodeConflict,
			"You already have a handle. Handles are immutable in this release.")
		return
	}

	h, err := s.Store.ClaimHandle(ctx, user.ID, handle)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			httpx.Fail(c, http.StatusConflict, httpx.CodeHandleTaken, "That handle is already taken.")
			return
		}
		s.Log.Error("claim handle", "err", err, "request_id", c.GetString("request_id"))
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not claim the handle.")
		return
	}

	s.Store.Audit(ctx, &user.ID, "handle.claimed", "handle", h.ID.String(), map[string]any{"handle": h.Handle})
	c.JSON(http.StatusCreated, s.handlePrivate(h))
}

// UpdateHandle documents that handles are immutable in this release.
func (s *Server) UpdateHandle(c *gin.Context) {
	httpx.Fail(c, http.StatusConflict, httpx.CodeConflict,
		"Handles cannot be changed in this release. Contact support for account issues.")
}

// ResolveHandle is the public resolution endpoint. It returns only intentionally
// public fields and never reveals internal ids or private account data.
func (s *Server) ResolveHandle(c *gin.Context) {
	handle, err := validate.ValidateHandle(c.Param("handle"))
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeHandleInvalid, "That handle is not valid.")
		return
	}
	ctx := c.Request.Context()

	h, err := s.Store.GetHandle(ctx, handle)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.Fail(c, http.StatusNotFound, httpx.CodeHandleNotFound, "No public handle was found.")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not resolve the handle.")
		return
	}
	if !h.IsPublic {
		// Do not reveal that a private handle exists.
		httpx.Fail(c, http.StatusNotFound, httpx.CodeHandleNotFound, "No public handle was found.")
		return
	}

	dto := handlePublicDTO{
		Handle:      h.Handle,
		ProfileURL:  s.Cfg.AppBaseURL + "/@" + h.Handle,
		DisplayName: h.DisplayName,
		Bio:         h.Bio,
		Network:     s.Cfg.SolanaCluster,
		Wallets:     []walletPublicDTO{},
		Identities:  []identityPublicDTO{},
	}
	if ws, err := s.Store.ListWallets(ctx, h.UserID); err == nil {
		for _, w := range ws {
			dto.Wallets = append(dto.Wallets, walletPublicDTO{
				Network:           w.Network,
				Address:           w.Address,
				IsPrimary:         w.IsPrimary,
				OwnershipVerified: w.OwnershipVerifiedAt != nil,
			})
		}
	}
	if ids, err := s.Store.ListIdentities(ctx, h.UserID); err == nil {
		for _, ic := range ids {
			dto.Identities = append(dto.Identities, identityPublicDTO{
				Provider: ic.Provider, Username: ic.ProviderUsername, Verified: true,
			})
		}
	}
	c.Header("Cache-Control", "public, max-age=30")
	c.JSON(http.StatusOK, dto)
}
