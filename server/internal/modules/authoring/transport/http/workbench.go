package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/application"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/transport/http/dto"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/gin-gonic/gin"
)

type WorkbenchHandler struct {
	service      *application.Workbench
	maxBlobBytes int64
}

func NewWorkbenchHandler(service *application.Workbench, maxBlobBytes int64) *WorkbenchHandler {
	return &WorkbenchHandler{service: service, maxBlobBytes: maxBlobBytes}
}

func (handler *WorkbenchHandler) RegisterRoutes(api *gin.RouterGroup, auth gin.HandlerFunc, scope ...gin.HandlerFunc) {
	copies := api.Group("/authoring/problem-copies", auth)
	copies.Use(scope...)
	copies.POST("", handler.CopyRelease)
	library := api.Group("/authoring/problems", auth)
	library.Use(scope...)
	library.GET("", handler.Library)
	group := library.Group("/:id")
	group.Use(httpx.NumberParam("problems", "id"))
	group.POST("/working-copy", handler.Open)
	group.GET("/working-copy", handler.Read)
	group.PUT("/working-copy", handler.Save)
	group.PUT("/working-copy/entries/:entryId/text", handler.SaveText)
	group.PUT("/working-copy/entries/:entryId", handler.SaveEntry)
	group.DELETE("/working-copy/entries/:entryId", handler.DeleteEntry)
	group.POST("/working-copy/batch", handler.Batch)
	group.GET("/changes", handler.Changes)
	group.GET("/materials/:entryId", handler.Material)
	group.GET("/materials", handler.Materials)
	group.GET("/inspection", handler.Inspect)
	group.POST("/checks", handler.StartCheck)
	group.GET("/checks", handler.Checks)
	group.GET("/checks/:checkId", handler.Check)
	group.GET("/checks/:checkId/statements/:statementId", handler.CheckStatement)
	group.POST("/checks/:checkId/cancel", handler.CancelCheck)
	group.POST("/releases", handler.PublishCommit)
	group.GET("/releases", handler.CommitReleases)
	group.GET("/origin", handler.CopyOrigin)
	group.PUT("/visibility", handler.SetVisibility)
	group.POST("/imports", handler.PreviewImport)
	group.GET("/imports/:importId", handler.Import)
	group.POST("/imports/:importId/apply", handler.ApplyImport)
	group.POST("/exports", handler.ExportPackage)
	group.POST("/working-copy/update", handler.Update)
	group.POST("/working-copy/discard", handler.Discard)
	group.POST("/working-copy/restore", handler.Restore)
	group.POST("/commits", handler.Commit)
	group.GET("/commits", handler.History)
	group.GET("/commits/:revision", handler.Revision)
	group.POST("/blobs", handler.Upload)
	group.GET("/blobs/:digest", handler.Blob)
	group.GET("/merges/:mergeId", handler.Merge)
	group.PUT("/merges/:mergeId", handler.SaveMerge)
	group.POST("/merges/:mergeId/complete", handler.CompleteMerge)
}

// Open starts or resumes the authenticated editor's own working copy.
//
//	@Summary	Start a private working copy
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain		path		string	true	"Domain slug"
//	@Param		id			path		string	true	"Problem number"
//	@Success	200			{object}	domain.WorkingCopy
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/working-copy [post]
func (handler *WorkbenchHandler) Open(c *gin.Context) {
	value, err := handler.service.Open(c.Request.Context(), httpx.ResourceID(c, "id"))
	workbenchResult(c, value, err)
}

// Read never creates a working copy as a side effect.
//
//	@Summary	Read my working copy
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain		path		string	true	"Domain slug"
//	@Param		id			path		string	true	"Problem number"
//	@Success	200			{object}	domain.WorkingCopy
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/working-copy [get]
func (handler *WorkbenchHandler) Read(c *gin.Context) {
	value, err := handler.service.Read(c.Request.Context(), httpx.ResourceID(c, "id"))
	workbenchResult(c, value, err)
}

// Save updates a draft without creating a commit.
//
//	@Summary	Save my working copy
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain					path		string						true	"Domain slug"
//	@Param		id						path		string						true	"Problem number"
//	@Param		request					body		dto.WorkingCopySaveRequest	true	"Expected token and content manifest"
//	@Success	200						{object}	domain.WorkingCopy
//	@Failure	400,401,403,404,409,413	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/working-copy [put]
func (handler *WorkbenchHandler) Save(c *gin.Context) {
	var input dto.WorkingCopySaveRequest
	if !httpx.BindJSON(c, &input, maxPackageBody, "invalid working copy") {
		return
	}
	value, err := handler.service.Save(c.Request.Context(), httpx.ResourceID(c, "id"), input.ETag, input.Tree)
	workbenchResult(c, value, err)
}

// Commit atomically reconciles and appends a shared revision.
//
//	@Summary	Commit working copy changes
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain				path		string				true	"Domain slug"
//	@Param		id					path		string				true	"Problem number"
//	@Param		request				body		domain.CommitInput	true	"Commit message and idempotency key"
//	@Success	200					{object}	domain.CommitOutcome
//	@Failure	400,401,403,404,409	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/commits [post]
func (handler *WorkbenchHandler) Commit(c *gin.Context) {
	var input domain.CommitInput
	if !httpx.BindJSON(c, &input, maxPackageControlBody, "invalid commit request") {
		return
	}
	value, err := handler.service.Commit(c.Request.Context(), httpx.ResourceID(c, "id"), input)
	workbenchResult(c, value, err)
}

// History lists shared commits, excluding uncommitted private material.
//
//	@Summary	List problem commit history
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain			path		string	true	"Domain slug"
//	@Param		id				path		string	true	"Problem number"
//	@Param		before			query		integer	false	"Exclusive revision cursor"
//	@Param		limit			query		integer	false	"Page size, 1-100"
//	@Success	200				{object}	dto.CommitHistoryResponse
//	@Failure	400,401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/commits [get]
func (handler *WorkbenchHandler) History(c *gin.Context) {
	before, err := strconv.ParseInt(c.DefaultQuery("before", "0"), 10, 64)
	if err != nil {
		writeAuthoringError(c, domain.InvalidInput("invalid history cursor"))
		return
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "30"))
	if err != nil {
		writeAuthoringError(c, domain.InvalidInput("invalid page size"))
		return
	}
	items, err := handler.service.History(c.Request.Context(), httpx.ResourceID(c, "id"), before, limit)
	workbenchResult(c, dto.CommitHistoryResponse{Items: items}, err)
}

// Revision returns the immutable content of one shared commit.
//
//	@Summary	Read a problem commit
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain			path		string	true	"Domain slug"
//	@Param		id				path		string	true	"Problem number"
//	@Param		revision		path		integer	true	"Revision"
//	@Success	200				{object}	dto.CommitDetailResponse
//	@Failure	400,401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/commits/{revision} [get]
func (handler *WorkbenchHandler) Revision(c *gin.Context) {
	revision, err := pathInt64(c, "revision")
	if err != nil {
		writeAuthoringError(c, domain.InvalidInput("invalid revision"))
		return
	}
	commit, tree, err := handler.service.Revision(c.Request.Context(), httpx.ResourceID(c, "id"), revision)
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	workbenchResult(c, dto.CommitDetailResponse{Commit: *commit, Tree: tree}, nil)
}

// SaveText stores editor content under the original working copy token.
//
//	@Summary	Save working copy text
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain					path		string						true	"Domain slug"
//	@Param		id						path		string						true	"Problem number"
//	@Param		entryId					path		string						true	"Stable entry ID"
//	@Param		request					body		dto.WorkingCopyTextRequest	true	"Editor text and expected token"
//	@Success	200						{object}	domain.WorkingCopy
//	@Failure	400,401,403,404,409,413	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/working-copy/entries/{entryId}/text [put]
func (handler *WorkbenchHandler) SaveText(c *gin.Context) {
	var input dto.WorkingCopyTextRequest
	if !httpx.BindJSON(c, &input, maxPackageBody, "invalid editor request") {
		return
	}
	value, err := handler.service.SaveText(c.Request.Context(), httpx.ResourceID(c, "id"), input.ETag, c.Param("entryId"), input.Text)
	workbenchResult(c, value, err)
}

// Update merges the shared head into my draft without committing.
//
//	@Summary	Update working copy
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain				path		string						true	"Domain slug"
//	@Param		id					path		string						true	"Problem number"
//	@Param		request				body		dto.WorkingCopyTokenRequest	true	"Expected working copy token"
//	@Success	200					{object}	domain.CommitOutcome
//	@Failure	400,401,403,404,409	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/working-copy/update [post]
func (handler *WorkbenchHandler) Update(c *gin.Context) {
	var input dto.WorkingCopyTokenRequest
	if !httpx.BindJSON(c, &input, maxPackageControlBody, "working copy token required") {
		return
	}
	value, err := handler.service.Update(c.Request.Context(), httpx.ResourceID(c, "id"), input.ETag)
	workbenchResult(c, value, err)
}

// Discard explicitly replaces my draft with the shared head.
//
//	@Summary	Discard working copy changes
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain				path		string						true	"Domain slug"
//	@Param		id					path		string						true	"Problem number"
//	@Param		request				body		dto.WorkingCopyTokenRequest	true	"Expected working copy token"
//	@Success	200					{object}	domain.WorkingCopy
//	@Failure	400,401,403,404,409	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/working-copy/discard [post]
func (handler *WorkbenchHandler) Discard(c *gin.Context) {
	var input dto.WorkingCopyTokenRequest
	if !httpx.BindJSON(c, &input, maxPackageControlBody, "working copy token required") {
		return
	}
	value, err := handler.service.Reset(c.Request.Context(), httpx.ResourceID(c, "id"), input.ETag, 0)
	workbenchResult(c, value, err)
}

// Restore loads a historical tree into my draft, keeping history immutable.
//
//	@Summary	Restore a revision to working copy
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain				path		string							true	"Domain slug"
//	@Param		id					path		string							true	"Problem number"
//	@Param		request				body		dto.WorkingCopyRestoreRequest	true	"Revision and expected token"
//	@Success	200					{object}	domain.WorkingCopy
//	@Failure	400,401,403,404,409	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/working-copy/restore [post]
func (handler *WorkbenchHandler) Restore(c *gin.Context) {
	var input dto.WorkingCopyRestoreRequest
	if !httpx.BindJSON(c, &input, maxPackageControlBody, "restore requires a token and revision") {
		return
	}
	value, err := handler.service.Reset(c.Request.Context(), httpx.ResourceID(c, "id"), input.ETag, input.Revision)
	workbenchResult(c, value, err)
}

// Merge reads only the authenticated user's merge session.
//
//	@Summary	Read my merge session
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain		path		string	true	"Domain slug"
//	@Param		id			path		string	true	"Problem number"
//	@Param		mergeId		path		string	true	"Merge session ID"
//	@Success	200			{object}	domain.MergeSession
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/merges/{mergeId} [get]
func (handler *WorkbenchHandler) Merge(c *gin.Context) {
	value, err := handler.service.Merge(c.Request.Context(), httpx.ResourceID(c, "id"), c.Param("mergeId"))
	workbenchResult(c, value, err)
}

// SaveMerge preserves partial decisions without completing the merge.
//
//	@Summary	Save conflict resolutions
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain					path		string					true	"Domain slug"
//	@Param		id						path		string					true	"Problem number"
//	@Param		mergeId					path		string					true	"Merge session ID"
//	@Param		request					body		dto.MergeSaveRequest	true	"Provisional tree, resolved conflicts and token"
//	@Success	200						{object}	domain.MergeSession
//	@Failure	400,401,403,404,409,413	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/merges/{mergeId} [put]
func (handler *WorkbenchHandler) SaveMerge(c *gin.Context) {
	var input dto.MergeSaveRequest
	if !httpx.BindJSON(c, &input, maxPackageBody, "invalid merge draft") {
		return
	}
	value, err := handler.service.SaveMerge(c.Request.Context(), httpx.ResourceID(c, "id"), c.Param("mergeId"), input.ETag, input.Tree, input.Resolved)
	workbenchResult(c, value, err)
}

// CompleteMerge adopts a resolved tree without creating a shared commit.
//
//	@Summary	Complete working copy merge
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain				path		string						true	"Domain slug"
//	@Param		id					path		string						true	"Problem number"
//	@Param		mergeId				path		string						true	"Merge session ID"
//	@Param		request				body		dto.WorkingCopyTokenRequest	true	"Expected merge token"
//	@Success	200					{object}	domain.WorkingCopy
//	@Failure	400,401,403,404,409	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/merges/{mergeId}/complete [post]
func (handler *WorkbenchHandler) CompleteMerge(c *gin.Context) {
	var input dto.WorkingCopyTokenRequest
	if !httpx.BindJSON(c, &input, maxPackageControlBody, "merge token required") {
		return
	}
	value, err := handler.service.CompleteMerge(c.Request.Context(), httpx.ResourceID(c, "id"), c.Param("mergeId"), input.ETag)
	workbenchResult(c, value, err)
}

// Upload stores a private content object without changing the working copy.
//
//	@Summary	Upload authoring content
//	@Tags		authoring
//	@Accept		multipart/form-data
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain				path		string	true	"Domain slug"
//	@Param		id					path		string	true	"Problem number"
//	@Param		file				formData	file	true	"File contents"
//	@Success	200					{object}	domain.BlobRef
//	@Failure	400,401,403,404,413	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/blobs [post]
func (handler *WorkbenchHandler) Upload(c *gin.Context) {
	if err := handler.service.AuthorizeEdit(c.Request.Context(), httpx.ResourceID(c, "id")); err != nil {
		writeAuthoringError(c, err)
		return
	}
	const multipartOverhead = int64(64 << 10)
	if c.Request.ContentLength > handler.maxBlobBytes+multipartOverhead {
		writeAuthoringError(c, domain.ErrPackageTooBig)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, handler.maxBlobBytes+multipartOverhead)
	err := c.Request.ParseMultipartForm(1 << 20)
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeAuthoringError(c, domain.ErrPackageTooBig)
		return
	}
	if err != nil || c.Request.MultipartForm == nil {
		writeAuthoringError(c, domain.InvalidInput("a multipart file is required"))
		return
	}
	files := c.Request.MultipartForm.File["file"]
	if len(files) != 1 || len(c.Request.MultipartForm.File) != 1 || len(c.Request.MultipartForm.Value) != 0 {
		writeAuthoringError(c, domain.InvalidInput("upload exactly one file"))
		return
	}
	if files[0].Size > handler.maxBlobBytes {
		writeAuthoringError(c, domain.ErrPackageTooBig)
		return
	}
	file, err := files[0].Open()
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	defer file.Close()
	ref, err := handler.service.Upload(c.Request.Context(), httpx.ResourceID(c, "id"), file)
	workbenchResult(c, ref, err)
}

// Blob authorizes a content reference through its problem and readable history.
//
//	@Summary	Download authorized authoring content
//	@Tags		authoring
//	@Produce	octet-stream
//	@Security	BearerAuth
//	@Param		domain		path		string	true	"Domain slug"
//	@Param		id			path		string	true	"Problem number"
//	@Param		digest		path		string	true	"Content SHA-256"
//	@Success	200			{file}		file
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/blobs/{digest} [get]
func (handler *WorkbenchHandler) Blob(c *gin.Context) {
	ref, reader, err := handler.service.Blob(c.Request.Context(), httpx.ResourceID(c, "id"), c.Param("digest"))
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	defer reader.Close()
	c.DataFromReader(http.StatusOK, ref.Bytes, "application/octet-stream", reader, map[string]string{"Content-Disposition": "attachment", "Cache-Control": "private, no-store", "X-Content-Type-Options": "nosniff"})
}

func workbenchResult(c *gin.Context, value any, err error) {
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, value)
}
