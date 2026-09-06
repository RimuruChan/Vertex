package handler

import (
	"net/http"
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/RimuruChan/Vertex/server/internal/problemset"
	"github.com/RimuruChan/Vertex/server/internal/problemset/dto"
	"github.com/gin-gonic/gin"
)

// Grants lists explicit user/group collaboration, not inherited domain roles.
//
//	@Summary	List problem set collaborators
//	@Tags		problem-sets
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Problem set ID"
//	@Success	200			{object}	httpx.ListResponse[dto.SetAccessResponse]
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/problem-sets/{id}/access [get]
func (h *SetHandler) Grants(c *gin.Context) {
	grants, err := h.service.Grants(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeError(c, err, "failed to load collaborators")
		return
	}
	items := dto.FromGrants(grants)
	c.JSON(http.StatusOK, httpx.ListResponse[dto.SetAccessResponse]{Items: items, Total: len(items)})
}

// SetGrant grants one active member or same-domain group a collaboration role.
//
//	@Summary	Set problem set collaborator
//	@Tags		problem-sets
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id					path		string					true	"Problem set ID"
//	@Param		request				body		dto.SetAccessRequest	true	"Collaborator"
//	@Success	200					{object}	httpx.StatusResponse
//	@Failure	400,401,403,404,413	{object}	httpx.ErrorResponse
//	@Router		/api/problem-sets/{id}/access [put]
func (h *SetHandler) SetGrant(c *gin.Context) {
	var request dto.SetAccessRequest
	if !httpx.BindJSON(c, &request, maxSetBody, "invalid collaborator") {
		return
	}
	err := h.service.SetGrant(c.Request.Context(), c.Param("id"), problemset.GrantInput{Username: request.Username, Group: request.Group, Role: problemset.AccessRole(request.Role)})
	if err != nil {
		h.writeError(c, err, "failed to update collaborator")
		return
	}
	c.JSON(http.StatusOK, httpx.StatusResponse{Status: "updated"})
}

// RemoveGrant removes only the selected explicit grant; other grants remain.
//
//	@Summary	Remove problem set collaborator
//	@Tags		problem-sets
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id				path		string	true	"Problem set ID"
//	@Param		grantId			path		int		true	"Grant ID"
//	@Success	200				{object}	httpx.StatusResponse
//	@Failure	400,401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/problem-sets/{id}/access/{grantId} [delete]
func (h *SetHandler) RemoveGrant(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("grantId"), 10, 64)
	if err != nil {
		h.writeError(c, &problemset.ValidationError{Message: "invalid grant ID"}, "")
		return
	}
	if err := h.service.RemoveGrant(c.Request.Context(), c.Param("id"), id); err != nil {
		h.writeError(c, err, "failed to remove collaborator")
		return
	}
	c.JSON(http.StatusOK, httpx.StatusResponse{Status: "deleted"})
}

// Transfer replaces ownership without rewriting creator attribution.
//
//	@Summary	Transfer problem set ownership
//	@Tags		problem-sets
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id					path		string				true	"Problem set ID"
//	@Param		request				body		dto.SetOwnerRequest	true	"New owner"
//	@Success	200					{object}	httpx.StatusResponse
//	@Failure	400,401,403,404,413	{object}	httpx.ErrorResponse
//	@Router		/api/problem-sets/{id}/owner [put]
func (h *SetHandler) Transfer(c *gin.Context) {
	var request dto.SetOwnerRequest
	if !httpx.BindJSON(c, &request, maxSetBody, "username is required") {
		return
	}
	if err := h.service.Transfer(c.Request.Context(), c.Param("id"), request.Username); err != nil {
		h.writeError(c, err, "failed to transfer ownership")
		return
	}
	c.JSON(http.StatusOK, httpx.StatusResponse{Status: "transferred"})
}
