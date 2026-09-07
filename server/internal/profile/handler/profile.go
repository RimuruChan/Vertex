package handler

import (
	"errors"
	"net/http"

	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/RimuruChan/Vertex/server/internal/profile"
	"github.com/RimuruChan/Vertex/server/internal/profile/dto"
	"github.com/gin-gonic/gin"
)

type ProfileHandler struct {
	service *profile.Service
}

func NewProfileHandler(service *profile.Service) *ProfileHandler {
	return &ProfileHandler{service: service}
}

// Get returns the public profile aggregate for one username.
//
//	@Summary	Get user profile
//	@Tags		users
//	@Produce	json
//	@Param		username	path		string	true	"Username"
//	@Success	200			{object}	dto.ProfileResponse
//	@Failure	404			{object}	httpx.ErrorResponse
//	@Param		domain		path		string	true	"Domain slug"
//	@Router		/api/users/{username} [get]
//	@Router		/api/domains/{domain}/users/{username} [get]
func (h *ProfileHandler) Get(c *gin.Context) {
	result, err := h.service.ByUsername(c.Request.Context(), c.Param("username"))
	if err != nil {
		if errors.Is(err, profile.ErrNotFound) {
			httpx.WriteError(c, http.StatusNotFound, "profile.not_found", "user not found")
			return
		}
		httpx.WriteError(c, http.StatusInternalServerError, "profile.load_failed", "failed to load profile")
		return
	}
	c.JSON(http.StatusOK, dto.FromProfile(*result))
}
