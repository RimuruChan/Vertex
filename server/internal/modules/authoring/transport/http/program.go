package handler

import (
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/gin-gonic/gin"
)

// SaveProgram saves a whole program and its source membership atomically.
//
//	@Summary	Save an authoring program
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain				path		string					true	"Domain slug"
//	@Param		id					path		string					true	"Problem number"
//	@Param		request				body		domain.ProgramSaveInput	true	"Program and sources"
//	@Success	200					{object}	domain.ProgramSaveResult
//	@Failure	400,401,403,404,409	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/programs [put]
func (handler *WorkbenchHandler) SaveProgram(c *gin.Context) {
	var input domain.ProgramSaveInput
	if !httpx.BindJSON(c, &input, maxPackageControlBody, "invalid program") {
		return
	}
	value, err := handler.service.SaveProgram(c.Request.Context(), httpx.ResourceID(c, "id"), input)
	workbenchResult(c, value, err)
}
