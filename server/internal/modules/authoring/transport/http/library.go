package handler

import (
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/gin-gonic/gin"
)

// Library lists summaries selected through this actor's draft or shared head.
//
//	@Summary	List my authoring workbench
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain			path		string	true	"Domain slug"
//	@Param		keyword			query		string	false	"Title, source or problem number"
//	@Param		visibility		query		string	false	"draft, private or public"
//	@Param		status			query		string	false	"changes, conflicts or unpublished"
//	@Param		page			query		integer	false	"Page number"
//	@Param		size			query		integer	false	"Page size, 1-100"
//	@Success	200				{object}	domain.LibraryPage
//	@Failure	400,401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems [get]
func (handler *WorkbenchHandler) Library(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 || page > 100000 {
		writeAuthoringError(c, domain.InvalidInput("invalid page"))
		return
	}
	size, err := strconv.Atoi(c.DefaultQuery("size", "20"))
	if err != nil || size < 1 || size > 100 {
		writeAuthoringError(c, domain.InvalidInput("invalid page size"))
		return
	}
	value, err := handler.service.Library(c.Request.Context(), domain.LibraryQuery{Keyword: c.Query("keyword"), Visibility: c.Query("visibility"), Status: c.Query("status"), Limit: size, Offset: (page - 1) * size})
	workbenchResult(c, value, err)
}
