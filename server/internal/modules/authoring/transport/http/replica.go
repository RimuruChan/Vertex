package handler

import (
	"net/http"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/transport/http/dto"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/gin-gonic/gin"
)

// CopyRelease creates a private workbench from an immutable published release.
//
//	@Summary	Copy a released authoring snapshot
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain				path		string			true	"Destination domain slug"
//	@Param		request				body		dto.CopyRequest	true	"Source release and attribution"
//	@Success	201					{object}	dto.CopyResponse
//	@Failure	400,401,403,404,409	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problem-copies [post]
func (handler *WorkbenchHandler) CopyRelease(c *gin.Context) {
	var input dto.CopyRequest
	if !httpx.BindJSON(c, &input, 16<<10, "source release and attribution required") {
		return
	}
	result, err := handler.service.CopyRelease(c.Request.Context(), domain.CopyInput{SourceDomain: input.SourceDomain, SourceProblem: input.SourceProblem, SourceVersion: input.SourceVersion, Attribution: input.Attribution})
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusCreated, dto.FromCopy(*result))
}

// CopyOrigin returns provenance only to destination collaborators.
//
//	@Summary	Read copied problem provenance
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain		path		string	true	"Domain slug"
//	@Param		id			path		string	true	"Problem number"
//	@Success	200			{object}	dto.ProblemOriginResponse
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/origin [get]
func (handler *WorkbenchHandler) CopyOrigin(c *gin.Context) {
	origin, err := handler.service.Origin(c.Request.Context(), httpx.ResourceID(c, "id"))
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	result := dto.ProblemOriginResponse{}
	if origin != nil {
		value := dto.FromOrigin(*origin)
		result.Origin = &value
	}
	workbenchResult(c, result, nil)
}
