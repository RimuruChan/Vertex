package handler

import (
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/transport/http/dto"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/gin-gonic/gin"
)

// Inspect reports structural readiness for a fixed draft token or commit.
//
//	@Summary	Inspect authoring material references
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain				path		string	true	"Domain slug"
//	@Param		id					path		string	true	"Problem number"
//	@Param		revision			query		integer	false	"Shared revision; omit for my working copy"
//	@Param		etag				query		string	false	"Expected working-copy token"
//	@Success	200					{object}	domain.MaterialInspection
//	@Failure	400,401,403,404,409	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/inspection [get]
func (handler *WorkbenchHandler) Inspect(c *gin.Context) {
	revision, err := strconv.ParseInt(c.DefaultQuery("revision", "0"), 10, 64)
	if err != nil {
		writeAuthoringError(c, domain.InvalidInput("invalid revision"))
		return
	}
	value, err := handler.service.Inspect(c.Request.Context(), httpx.ResourceID(c, "id"), revision, c.Query("etag"))
	workbenchResult(c, value, err)
}

// Material returns the typed content for one form or a historical review.
//
//	@Summary	Read structured authoring material
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain			path		string	true	"Domain slug"
//	@Param		id				path		string	true	"Problem number"
//	@Param		entryId			path		string	true	"Stable entry ID"
//	@Param		revision		query		integer	false	"Shared revision; omitted selects my working copy"
//	@Success	200				{object}	domain.MaterialView
//	@Failure	400,401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/materials/{entryId} [get]
func (handler *WorkbenchHandler) Material(c *gin.Context) {
	revision, err := strconv.ParseInt(c.DefaultQuery("revision", "0"), 10, 64)
	if err != nil {
		writeAuthoringError(c, domain.InvalidInput("invalid revision"))
		return
	}
	value, err := handler.service.Material(c.Request.Context(), httpx.ResourceID(c, "id"), c.Param("entryId"), revision)
	workbenchResult(c, value, err)
}

// SaveEntry saves one material, leaving every other entry unchanged.
//
//	@Summary	Save working copy material
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain					path		string						true	"Domain slug"
//	@Param		id						path		string						true	"Problem number"
//	@Param		entryId					path		string						true	"Stable entry ID"
//	@Param		request					body		dto.MaterialEntryRequest	true	"Entry, optional text and expected token"
//	@Success	200						{object}	domain.WorkingCopy
//	@Failure	400,401,403,404,409,413	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/working-copy/entries/{entryId} [put]
func (handler *WorkbenchHandler) SaveEntry(c *gin.Context) {
	var input dto.MaterialEntryRequest
	if !httpx.BindJSON(c, &input, maxPackageBody, "invalid material entry") {
		return
	}
	if input.Entry.ID != c.Param("entryId") {
		writeAuthoringError(c, domain.InvalidInput("entry ID does not match the route"))
		return
	}
	if input.Text == nil && input.Entry.Blob == nil {
		writeAuthoringError(c, domain.InvalidInput("provide text or an uploaded blob"))
		return
	}
	value, err := handler.service.SaveEntry(c.Request.Context(), httpx.ResourceID(c, "id"), input.ETag, input.Entry.Domain(), input.Text)
	workbenchResult(c, value, err)
}

// DeleteEntry removes a material from my draft, without rewriting history.
//
//	@Summary	Remove working copy material
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain				path		string						true	"Domain slug"
//	@Param		id					path		string						true	"Problem number"
//	@Param		entryId				path		string						true	"Stable entry ID"
//	@Param		request				body		dto.WorkingCopyTokenRequest	true	"Expected working copy token"
//	@Success	200					{object}	domain.WorkingCopy
//	@Failure	400,401,403,404,409	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/working-copy/entries/{entryId} [delete]
func (handler *WorkbenchHandler) DeleteEntry(c *gin.Context) {
	var input dto.WorkingCopyTokenRequest
	if !httpx.BindJSON(c, &input, maxPackageControlBody, "working copy token required") {
		return
	}
	value, err := handler.service.DeleteEntry(c.Request.Context(), httpx.ResourceID(c, "id"), input.ETag, c.Param("entryId"))
	workbenchResult(c, value, err)
}

// Changes compares my draft with its base, or two shared revisions.
//
//	@Summary	Compare authoring content
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain			path		string	true	"Domain slug"
//	@Param		id				path		string	true	"Problem number"
//	@Param		from			query		integer	false	"Base revision; omitted uses parent or working copy base"
//	@Param		revision		query		integer	false	"Target revision; omitted uses my working copy"
//	@Success	200				{object}	domain.ContentComparison
//	@Failure	400,401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/changes [get]
func (handler *WorkbenchHandler) Changes(c *gin.Context) {
	from, err := strconv.ParseInt(c.DefaultQuery("from", "0"), 10, 64)
	if err != nil {
		writeAuthoringError(c, domain.InvalidInput("invalid base revision"))
		return
	}
	to, err := strconv.ParseInt(c.DefaultQuery("revision", "0"), 10, 64)
	if err != nil {
		writeAuthoringError(c, domain.InvalidInput("invalid target revision"))
		return
	}
	value, err := handler.service.Changes(c.Request.Context(), httpx.ResourceID(c, "id"), from, to)
	workbenchResult(c, value, err)
}
