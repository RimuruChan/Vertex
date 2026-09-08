package handler

import (
	"net/http"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/authoring/transport/http/dto"
	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/gin-gonic/gin"
)

// Copy creates an independent draft in the path-selected destination domain.
//
//	@Summary	Copy a published problem into a domain
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain				path		string			true	"Destination domain slug"
//	@Param		request				body		dto.CopyRequest	true	"Source release and attribution"
//	@Success	201					{object}	dto.CopyResponse
//	@Failure	400,401,403,404,413	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/problem-copies [post]
func (h *PackageHandler) Copy(c *gin.Context) {
	var request dto.CopyRequest
	if !httpx.BindJSON(c, &request, 16<<10, "source release and attribution are required") {
		return
	}
	result, err := h.service.Copy(c.Request.Context(), authoringdomain.CopyInput{SourceDomain: request.SourceDomain, SourceProblem: request.SourceProblem, SourceVersion: request.SourceVersion, Attribution: request.Attribution})
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	c.JSON(http.StatusCreated, dto.FromCopy(*result))
}

// Origin exposes historical provenance only through package authorization.
//
//	@Summary	Get the problem copy provenance
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain		path		string	true	"Destination domain slug"
//	@Param		id			path		string	true	"Problem ID"
//	@Success	200			{object}	dto.ProblemOriginResponse
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/admin/problems/{id}/origin [get]
func (h *PackageHandler) Origin(c *gin.Context) {
	origin, err := h.service.Origin(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	response := dto.ProblemOriginResponse{}
	if origin != nil {
		value := dto.FromOrigin(*origin)
		response.Origin = &value
	}
	c.JSON(http.StatusOK, response)
}
