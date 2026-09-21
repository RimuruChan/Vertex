package handler

import (
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/gin-gonic/gin"
)

// PreviewStatement resolves only explicitly public samples into draft Markdown.
//
//	@Summary	Preview a statement with its public samples
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain				path		string							true	"Domain slug"
//	@Param		id					path		string							true	"Problem number"
//	@Param		request				body		domain.StatementPreviewInput	true	"Snapshot and unsaved Markdown"
//	@Success	200					{object}	domain.DraftStatementPreview
//	@Failure	400,401,403,404,409	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/statement-preview [post]
func (handler *WorkbenchHandler) PreviewStatement(c *gin.Context) {
	var input domain.StatementPreviewInput
	if !httpx.BindJSON(c, &input, maxPackageControlBody, "invalid statement preview") {
		return
	}
	value, err := handler.service.PreviewStatement(c.Request.Context(), httpx.ResourceID(c, "id"), input)
	workbenchResult(c, value, err)
}
