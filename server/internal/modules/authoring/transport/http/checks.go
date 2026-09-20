package handler

import (
	"net/http"
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/gin-gonic/gin"
)

// StartCheck queues the exact selected content snapshot.
//
//	@Summary	Queue a frozen authoring check
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain				path		string					true	"Domain slug"
//	@Param		id					path		string					true	"Problem number"
//	@Param		request				body		domain.CheckSelection	true	"Shared revision or private working-copy token"
//	@Success	200					{object}	domain.CheckRun
//	@Failure	400,401,403,404,409	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/checks [post]
func (handler *WorkbenchHandler) StartCheck(c *gin.Context) {
	var request domain.CheckSelection
	if !httpx.BindJSON(c, &request, maxPackageControlBody, "select a revision or saved working copy") {
		return
	}
	value, err := handler.service.StartCheck(c.Request.Context(), httpx.ResourceID(c, "id"), request)
	workbenchResult(c, value, err)
}

// Checks only lists shared checks and the current user's private checks.
//
//	@Summary	List readable authoring checks
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain			path		string	true	"Domain slug"
//	@Param		id				path		string	true	"Problem number"
//	@Param		limit			query		integer	false	"Page size, 1-100"
//	@Success	200				{object}	httpx.ListResponse[domain.CheckRun]
//	@Failure	400,401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/checks [get]
func (handler *WorkbenchHandler) Checks(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "30"))
	if err != nil {
		writeAuthoringError(c, domain.InvalidInput("invalid page size"))
		return
	}
	items, err := handler.service.Checks(c.Request.Context(), httpx.ResourceID(c, "id"), limit)
	workbenchResult(c, httpx.ListResponse[domain.CheckRun]{Items: items, Total: len(items)}, err)
}

// Check returns a report without exposing another author's private draft.
//
//	@Summary	Read an authoring check
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain		path		string	true	"Domain slug"
//	@Param		id			path		string	true	"Problem number"
//	@Param		checkId		path		string	true	"Check ID"
//	@Success	200			{object}	domain.CheckRun
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/checks/{checkId} [get]
func (handler *WorkbenchHandler) Check(c *gin.Context) {
	value, err := handler.service.Check(c.Request.Context(), httpx.ResourceID(c, "id"), c.Param("checkId"))
	workbenchResult(c, value, err)
}

// CheckStatement downloads an immutable rendering only after checking access to
// the exact shared or private check, in the repository's authorization transaction.
//
//	@Summary	Download a checked TeX statement PDF
//	@Tags		authoring
//	@Produce	application/pdf
//	@Security	BearerAuth
//	@Param		domain		path		string	true	"Domain slug"
//	@Param		id			path		string	true	"Problem number"
//	@Param		checkId		path		string	true	"Check ID"
//	@Param		statementId	path		string	true	"Statement ID"
//	@Success	200			{file}		file
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/checks/{checkId}/statements/{statementId} [get]
func (handler *WorkbenchHandler) CheckStatement(c *gin.Context) {
	data, err := handler.service.CheckStatement(c.Request.Context(), httpx.ResourceID(c, "id"), c.Param("checkId"), c.Param("statementId"))
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "sandbox; default-src 'none'")
	c.Header("Content-Disposition", `attachment; filename="statement.pdf"`)
	c.Data(http.StatusOK, "application/pdf", data)
}

// CancelCheck cancels a readable check only when the actor can edit the problem.
//
//	@Summary	Cancel an authoring check
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain		path		string	true	"Domain slug"
//	@Param		id			path		string	true	"Problem number"
//	@Param		checkId		path		string	true	"Check ID"
//	@Success	200			{object}	domain.CheckRun
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/checks/{checkId}/cancel [post]
func (handler *WorkbenchHandler) CancelCheck(c *gin.Context) {
	value, err := handler.service.CancelCheck(c.Request.Context(), httpx.ResourceID(c, "id"), c.Param("checkId"))
	workbenchResult(c, value, err)
}

func (handler *WorkbenchHandler) RegisterContentRoute(internal *gin.RouterGroup, requireJudge gin.HandlerFunc) {
	internal.GET("/judge/v1/builds/:buildId/content/:digest", requireJudge, handler.CheckContent)
}

// CheckContent serves only content referenced by a live leased build snapshot.
//
//	@Summary	Download sealed build content
//	@Tags		judge-internal
//	@Produce	octet-stream
//	@Security	JudgeServiceAuth
//	@Param		buildId					path		string	true	"Build ID"
//	@Param		digest					path		string	true	"Content SHA-256"
//	@Param		X-Vertex-Worker-Id		header		string	true	"Worker ID"
//	@Param		X-Vertex-Lease-Token	header		string	true	"Current lease token"
//	@Success	200						{file}		file
//	@Failure	401,404,409				{object}	httpx.ErrorResponse
//	@Router		/internal/judge/v1/builds/{buildId}/content/{digest} [get]
func (handler *WorkbenchHandler) CheckContent(c *gin.Context) {
	ref, reader, err := handler.service.CheckContent(c.Request.Context(), c.Param("buildId"), c.GetHeader("X-Vertex-Worker-Id"), c.GetHeader("X-Vertex-Lease-Token"), c.Param("digest"))
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	defer reader.Close()
	c.DataFromReader(http.StatusOK, ref.Bytes, "application/octet-stream", reader, map[string]string{"Cache-Control": "private, no-store", "X-Content-Type-Options": "nosniff"})
}
