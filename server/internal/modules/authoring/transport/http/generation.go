package handler

import (
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/gin-gonic/gin"
)

// Generate previews or atomically applies an owned generation plan.
//
//	@Summary	Preview or apply a batch test generation plan
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain				path		string					true	"Domain slug"
//	@Param		id					path		string					true	"Problem number"
//	@Param		request				body		domain.GenerationInput	true	"Generation plan and copy token"
//	@Success	200					{object}	domain.GenerationResult
//	@Failure	400,401,403,404,409	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/generation [post]
func (handler *WorkbenchHandler) Generate(c *gin.Context) {
	var input domain.GenerationInput
	if !httpx.BindJSON(c, &input, maxPackageControlBody, "invalid generation plan") {
		return
	}
	value, err := handler.service.Generate(c.Request.Context(), httpx.ResourceID(c, "id"), input)
	workbenchResult(c, value, err)
}
