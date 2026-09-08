package httpapi

import (
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/httpx"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/problem/domain"
	dto "github.com/RimuruChan/Vertex/server/internal/problem/transport/http/dto"
	"github.com/gin-gonic/gin"
)

// Grants lists direct grants without flattening inherited group access.
//
//	@Summary	List problem collaborators
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id				path		string	true	"Problem ID"
//	@Success	200				{object}	httpx.ListResponse[dto.ProblemGrantResponse]
//	@Failure	401,403,404,500	{object}	httpx.ErrorResponse
//	@Param		domain			path		string	true	"Domain slug"
//	@Router		/api/admin/problems/{id}/access [get]
//	@Router		/api/domains/{domain}/admin/problems/{id}/access [get]
func (h *AdminProblemHandler) Grants(c *gin.Context) {
	grants, err := h.service.Grants(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeProblemError(c, err)
		return
	}
	c.JSON(200, httpx.ListResponse[dto.ProblemGrantResponse]{Items: dto.GrantsFromDomain(grants), Total: len(grants)})
}

// SetGrant grants a domain user or group read/edit access.
//
//	@Summary	Grant problem collaboration access
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id						path		string					true	"Problem ID"
//	@Param		request					body		dto.ProblemGrantRequest	true	"Collaborator"
//	@Success	200						{object}	httpx.StatusResponse
//	@Failure	400,401,403,404,413,500	{object}	httpx.ErrorResponse
//	@Param		domain					path		string	true	"Domain slug"
//	@Router		/api/admin/problems/{id}/access [put]
//	@Router		/api/domains/{domain}/admin/problems/{id}/access [put]
func (h *AdminProblemHandler) SetGrant(c *gin.Context) {
	var request dto.ProblemGrantRequest
	if !httpx.BindJSON(c, &request, 16<<10, "invalid collaborator") {
		return
	}
	err := h.service.SetGrant(c.Request.Context(), c.Param("id"), problemdomain.GrantInput{Username: request.Username, Group: request.Group, Role: problemdomain.AccessRole(request.Role)})
	if err != nil {
		writeProblemError(c, err)
		return
	}
	c.JSON(200, httpx.StatusResponse{Status: "saved"})
}

// RemoveGrant only removes the selected direct grant; group inheritance remains.
//
//	@Summary	Remove problem collaboration grant
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id					path		string	true	"Problem ID"
//	@Param		grant				path		int		true	"Grant ID"
//	@Success	200					{object}	httpx.StatusResponse
//	@Failure	400,401,403,404,500	{object}	httpx.ErrorResponse
//	@Param		domain				path		string	true	"Domain slug"
//	@Router		/api/admin/problems/{id}/access/{grant} [delete]
//	@Router		/api/domains/{domain}/admin/problems/{id}/access/{grant} [delete]
func (h *AdminProblemHandler) RemoveGrant(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("grant"), 10, 64)
	if err != nil {
		writeAPIError(c, 400, "request.invalid", "invalid grant ID")
		return
	}
	if err := h.service.RemoveGrant(c.Request.Context(), c.Param("id"), id); err != nil {
		writeProblemError(c, err)
		return
	}
	c.JSON(200, httpx.StatusResponse{Status: "removed"})
}

// TransferOwner changes ownership without rewriting authorship.
//
//	@Summary	Transfer problem ownership
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id						path		string					true	"Problem ID"
//	@Param		request					body		dto.ProblemOwnerRequest	true	"New owner"
//	@Success	200						{object}	httpx.StatusResponse
//	@Failure	400,401,403,404,413,500	{object}	httpx.ErrorResponse
//	@Param		domain					path		string	true	"Domain slug"
//	@Router		/api/admin/problems/{id}/owner [put]
//	@Router		/api/domains/{domain}/admin/problems/{id}/owner [put]
func (h *AdminProblemHandler) TransferOwner(c *gin.Context) {
	var request dto.ProblemOwnerRequest
	if !httpx.BindJSON(c, &request, 16<<10, "new owner is required") {
		return
	}
	if err := h.service.Transfer(c.Request.Context(), c.Param("id"), request.Username); err != nil {
		writeProblemError(c, err)
		return
	}
	c.JSON(200, httpx.StatusResponse{Status: "transferred"})
}
