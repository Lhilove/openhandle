package api

import (
	"fmt"
	"time"

	"openhandle/internal/models"
)

// ---- private (owner-only) DTOs ----

type walletPrivateDTO struct {
	ID                  string     `json:"id"`
	Network             string     `json:"network"`
	Address             string     `json:"address"`
	IsPrimary           bool       `json:"is_primary"`
	OwnershipVerified   bool       `json:"ownership_verified"`
	OwnershipVerifiedAt *time.Time `json:"ownership_verified_at,omitempty"`
}

type identityDTO struct {
	Provider string `json:"provider"`
	Username string `json:"username"`
	Verified bool   `json:"verified"`
}

type handlePrivateDTO struct {
	ID          string  `json:"id"`
	Handle      string  `json:"handle"`
	DisplayName *string `json:"display_name"`
	Bio         *string `json:"bio"`
	IsPublic    bool    `json:"is_public"`
	PublicURL   string  `json:"public_url"`
}

type meDTO struct {
	Email      string             `json:"email"`
	CreatedAt  time.Time          `json:"created_at"`
	CSRFToken  string             `json:"csrf_token"`
	Handle     *handlePrivateDTO  `json:"handle"`
	Wallets    []walletPrivateDTO `json:"wallets"`
	Identities []identityDTO      `json:"identities"`
}

func (s *Server) walletPrivate(w models.Wallet) walletPrivateDTO {
	return walletPrivateDTO{
		ID:                  w.ID.String(),
		Network:             w.Network,
		Address:             w.Address,
		IsPrimary:           w.IsPrimary,
		OwnershipVerified:   w.OwnershipVerifiedAt != nil,
		OwnershipVerifiedAt: w.OwnershipVerifiedAt,
	}
}

func (s *Server) handlePrivate(h *models.Handle) *handlePrivateDTO {
	if h == nil {
		return nil
	}
	return &handlePrivateDTO{
		ID:          h.ID.String(),
		Handle:      h.Handle,
		DisplayName: h.DisplayName,
		Bio:         h.Bio,
		IsPublic:    h.IsPublic,
		PublicURL:   fmt.Sprintf("%s/@%s", s.Cfg.AppBaseURL, h.Handle),
	}
}

// ---- public DTOs (must contain no internal ids or private data) ----

type walletPublicDTO struct {
	Network           string `json:"network"`
	Address           string `json:"address"`
	IsPrimary         bool   `json:"is_primary"`
	OwnershipVerified bool   `json:"ownership_verified"`
}

type identityPublicDTO struct {
	Provider string `json:"provider"`
	Username string `json:"username"`
	Verified bool   `json:"verified"`
}

type handlePublicDTO struct {
	Handle      string              `json:"handle"`
	ProfileURL  string              `json:"profile_url"`
	DisplayName *string             `json:"display_name"`
	Bio         *string             `json:"bio"`
	Network     string              `json:"network"`
	Wallets     []walletPublicDTO   `json:"wallets"`
	Identities  []identityPublicDTO `json:"identities"`
}
