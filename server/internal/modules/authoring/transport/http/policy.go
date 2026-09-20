package handler

import (
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/gin-gonic/gin"
)

// SetVisibility changes resource access without editing versioned materials.
//
//	@Summary	Change published problem visibility
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain				path		string					true	"Domain slug"
//	@Param		id					path		string					true	"Problem number"
//	@Param		request				body		domain.VisibilityChange	true	"New and expected visibility"
//	@Success	200					{object}	domain.VisibilityState
//	@Failure	400,401,403,404,409	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/visibility [put]
func (handler *WorkbenchHandler) SetVisibility(c *gin.Context) {
	var input domain.VisibilityChange
	if !httpx.BindJSON(c, &input, 4096, "invalid visibility change") {
		return
	}
	value, err := handler.service.SetVisibility(c.Request.Context(), httpx.ResourceID(c, "id"), input)
	workbenchResult(c, value, err)
}
