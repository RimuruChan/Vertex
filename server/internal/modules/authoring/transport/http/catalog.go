package handler

import (
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/gin-gonic/gin"
)

// Materials returns a bounded, authorized page of structured descriptors.
//
//	@Summary	List authoring material descriptors
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain				path		string	true	"Domain slug"
//	@Param		id					path		string	true	"Problem number"
//	@Param		kind				query		string	true	"test, program or group"
//	@Param		after				query		string	false	"Previous page's final stable entry ID"
//	@Param		limit				query		integer	false	"Page size, 1-100"
//	@Param		revision			query		integer	false	"Shared revision; omit for my working copy"
//	@Param		etag				query		string	false	"Expected private copy token"
//	@Success	200					{object}	domain.MaterialPage
//	@Failure	400,401,403,404,409	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/materials [get]
func (handler *WorkbenchHandler) Materials(c *gin.Context) {
	revision, err := strconv.ParseInt(c.DefaultQuery("revision", "0"), 10, 64)
	if err != nil {
		writeAuthoringError(c, domain.InvalidInput("invalid revision"))
		return
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil {
		writeAuthoringError(c, domain.InvalidInput("invalid page limit"))
		return
	}
	value, err := handler.service.Materials(c.Request.Context(), httpx.ResourceID(c, "id"), domain.MaterialQuery{Kind: c.Query("kind"), After: c.Query("after"), Limit: limit, Revision: revision, ETag: c.Query("etag")})
	workbenchResult(c, value, err)
}

// Batch changes the entire selected batch through a single copy token.
//
//	@Summary	Atomically edit or remove authoring materials
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain				path		string					true	"Domain slug"
//	@Param		id					path		string					true	"Problem number"
//	@Param		request				body		domain.MaterialBatch	true	"Selected material changes"
//	@Success	200					{object}	domain.WorkingCopy
//	@Failure	400,401,403,404,409	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/working-copy/batch [post]
func (handler *WorkbenchHandler) Batch(c *gin.Context) {
	var input domain.MaterialBatch
	if !httpx.BindJSON(c, &input, maxPackageControlBody, "invalid material batch") {
		return
	}
	value, err := handler.service.Batch(c.Request.Context(), httpx.ResourceID(c, "id"), input)
	workbenchResult(c, value, err)
}
