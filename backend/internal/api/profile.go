package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"openhandle/internal/httpx"
	"openhandle/internal/store"
	"openhandle/internal/validate"
)

// UpdateProfile updates the owner's display name, bio and public visibility.
func (s *Server) UpdateProfile(c *gin.Context) {
	user, _ := httpx.CurrentUser(c)

	var req struct {
		DisplayName *string `json:"display_name"`
		Bio         *string `json:"bio"`
		IsPublic    *bool   `json:"is_public"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidRequest, "A valid JSON body is required.")
		return
	}

	var displayName, bio *string
	if req.DisplayName != nil {
		v, err := validate.ValidateDisplayName(*req.DisplayName)
		if err != nil {
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidRequest, err.Error())
			return
		}
		displayName = &v
	}
	if req.Bio != nil {
		v, err := validate.ValidateBio(*req.Bio)
		if err != nil {
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidRequest, err.Error())
			return
		}
		bio = &v
	}

	ctx := c.Request.Context()
	h, err := s.Store.UpdateHandleProfile(ctx, user.ID, displayName, bio, req.IsPublic)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.Fail(c, http.StatusNotFound, httpx.CodeHandleNotFound, "Claim a handle before editing a profile.")
			return
		}
		s.Log.Error("update profile", "err", err, "request_id", c.GetString("request_id"))
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "Could not update the profile.")
		return
	}
	s.Store.Audit(ctx, &user.ID, "profile.updated", "handle", h.ID.String(), nil)
	c.JSON(http.StatusOK, s.handlePrivate(h))
}
