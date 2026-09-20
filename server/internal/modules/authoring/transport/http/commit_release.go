package handler

import (
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/gin-gonic/gin"
)

// PublishCommit releases one reviewed commit with a matching successful check.
//
//	@Summary	Publish a checked problem commit
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain				path		string						true	"Domain slug"
//	@Param		id					path		string						true	"Problem number"
//	@Param		request				body		domain.CommitPublication	true	"Commit, check and expected release"
//	@Success	200					{object}	domain.CommitRelease
//	@Failure	400,401,403,404,409	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/releases [post]
func (handler *WorkbenchHandler) PublishCommit(c *gin.Context) {
	var input domain.CommitPublication
	if !httpx.BindJSON(c, &input, maxPackageControlBody, "invalid publication request") {
		return
	}
	value, err := handler.service.PublishCommit(c.Request.Context(), httpx.ResourceID(c, "id"), input)
	workbenchResult(c, value, err)
}

// CommitReleases lists immutable published commit bindings.
//
//	@Summary	List committed problem releases
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain		path		string	true	"Domain slug"
//	@Param		id			path		string	true	"Problem number"
//	@Success	200			{object}	httpx.ListResponse[domain.CommitRelease]
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/releases [get]
func (handler *WorkbenchHandler) CommitReleases(c *gin.Context) {
	items, err := handler.service.CommitReleases(c.Request.Context(), httpx.ResourceID(c, "id"))
	workbenchResult(c, httpx.ListResponse[domain.CommitRelease]{Items: items, Total: len(items)}, err)
}
