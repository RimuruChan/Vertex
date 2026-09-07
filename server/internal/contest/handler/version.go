package handler

import (
	"net/http"

	"github.com/RimuruChan/Vertex/server/internal/contest/dto"
	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/gin-gonic/gin"
)

// UseProblemVersion adopts a reviewed release without silently rejudging old results.
//
//	@Summary	Adopt a published version for a contest problem
//	@Tags		contests
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id						path		string						true	"Contest ID"
//	@Param		problemId				path		string						true	"Problem ID or contest label"
//	@Param		request					body		dto.ProblemVersionRequest	true	"Expected and target versions"
//	@Success	200						{object}	httpx.StatusResponse
//	@Failure	400,401,403,404,409,413	{object}	httpx.ErrorResponse
//	@Param		domain					path		string	true	"Domain slug"
//	@Router		/api/contests/{id}/problems/{problemId}/version [put]
//	@Router		/api/domains/{domain}/contests/{id}/problems/{problemId}/version [put]
func (h *ContestHandler) UseProblemVersion(c *gin.Context) {
	var request dto.ProblemVersionRequest
	if !httpx.BindJSON(c, &request, 16<<10, "versions are required") {
		return
	}
	if err := h.service.UseProblemVersion(c.Request.Context(), c.Param("id"), c.Param("problemId"), request.Version, request.ExpectedVersion); err != nil {
		h.writeError(c, err, "failed to adopt problem version")
		return
	}
	c.JSON(http.StatusOK, httpx.StatusResponse{Status: "updated"})
}
