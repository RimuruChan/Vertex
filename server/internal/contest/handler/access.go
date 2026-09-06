package handler

import (
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/contest"
	"github.com/RimuruChan/Vertex/server/internal/contest/dto"
	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/gin-gonic/gin"
)

// Grants lists direct grants without flattening inherited group access.
//
//	@Summary	List contest collaborators
//	@Tags		contests
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id				path		string	true	"Contest ID"
//	@Success	200				{object}	httpx.ListResponse[dto.ContestGrantResponse]
//	@Failure	401,403,404,500	{object}	httpx.ErrorResponse
//	@Router		/api/contests/{id}/access [get]
func (h *ContestHandler) Grants(c *gin.Context) {
	grants, err := h.service.Grants(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeError(c, err, "contest request failed")
		return
	}
	c.JSON(200, httpx.ListResponse[dto.ContestGrantResponse]{Items: dto.GrantsFromDomain(grants), Total: len(grants)})
}

// SetGrant grants a domain user or group one explicit contest role.
//
//	@Summary	Grant contest collaboration access
//	@Tags		contests
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id						path		string					true	"Contest ID"
//	@Param		request					body		dto.ContestGrantRequest	true	"Collaborator"
//	@Success	200						{object}	httpx.StatusResponse
//	@Failure	400,401,403,404,413,500	{object}	httpx.ErrorResponse
//	@Router		/api/contests/{id}/access [put]
func (h *ContestHandler) SetGrant(c *gin.Context) {
	var request dto.ContestGrantRequest
	if !httpx.BindJSON(c, &request, 16<<10, "invalid collaborator") {
		return
	}
	err := h.service.SetGrant(c.Request.Context(), c.Param("id"), contest.GrantInput{Username: request.Username, Group: request.Group, Role: request.Role})
	if err != nil {
		h.writeError(c, err, "contest request failed")
		return
	}
	c.JSON(200, httpx.StatusResponse{Status: "saved"})
}

// RemoveGrant only removes the selected direct grant; group inheritance remains.
//
//	@Summary	Remove contest collaboration grant
//	@Tags		contests
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id					path		string	true	"Contest ID"
//	@Param		grant				path		int		true	"Grant ID"
//	@Success	200					{object}	httpx.StatusResponse
//	@Failure	400,401,403,404,500	{object}	httpx.ErrorResponse
//	@Router		/api/contests/{id}/access/{grant} [delete]
func (h *ContestHandler) RemoveGrant(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("grant"), 10, 64)
	if err != nil {
		writeAPIError(c, 400, "request.invalid", "invalid grant ID")
		return
	}
	if err := h.service.RemoveGrant(c.Request.Context(), c.Param("id"), id); err != nil {
		h.writeError(c, err, "contest request failed")
		return
	}
	c.JSON(200, httpx.StatusResponse{Status: "removed"})
}

// TransferOwner changes ownership without rewriting authorship.
//
//	@Summary	Transfer contest ownership
//	@Tags		contests
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id						path		string					true	"Contest ID"
//	@Param		request					body		dto.ContestOwnerRequest	true	"New owner"
//	@Success	200						{object}	httpx.StatusResponse
//	@Failure	400,401,403,404,413,500	{object}	httpx.ErrorResponse
//	@Router		/api/contests/{id}/owner [put]
func (h *ContestHandler) TransferOwner(c *gin.Context) {
	var request dto.ContestOwnerRequest
	if !httpx.BindJSON(c, &request, 16<<10, "new owner is required") {
		return
	}
	if err := h.service.Transfer(c.Request.Context(), c.Param("id"), request.Username); err != nil {
		h.writeError(c, err, "contest request failed")
		return
	}
	c.JSON(200, httpx.StatusResponse{Status: "transferred"})
}

// Delete removes the contest and its contest-scoped records, not its problems.
//
//	@Summary	Delete contest
//	@Tags		contests
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id				path		string	true	"Contest ID"
//	@Success	200				{object}	httpx.StatusResponse
//	@Failure	401,403,404,500	{object}	httpx.ErrorResponse
//	@Router		/api/contests/{id} [delete]
func (h *ContestHandler) Delete(c *gin.Context) {
	if err := h.service.Delete(c.Request.Context(), c.Param("id")); err != nil {
		h.writeError(c, err, "failed to delete contest")
		return
	}
	c.JSON(200, httpx.StatusResponse{Status: "deleted"})
}
