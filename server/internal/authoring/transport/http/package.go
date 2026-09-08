// Package handler exposes the authoring workflow over HTTP: the admin-facing
// package editor and the internal, lease-fenced build worker protocol.
package handler

import (
	"errors"
	"net/http"
	"strconv"

	authoringapp "github.com/RimuruChan/Vertex/server/internal/authoring/application"
	authoringdomain "github.com/RimuruChan/Vertex/server/internal/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/authoring/transport/http/dto"
	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
	"github.com/gin-gonic/gin"
)

// maxPackageBody bounds an editor request. Sources are capped at 1 MB by the
// service; the extra headroom covers a large manual test.
const (
	maxPackageBody        = 8 << 20
	maxPackageControlBody = 64 << 10
)

// PackageHandler serves the administrator problem workspace.
type PackageHandler struct{ service *authoringapp.Service }

func NewPackageHandler(service *authoringapp.Service) *PackageHandler {
	return &PackageHandler{service: service}
}

// Workspace returns the whole package overview in one request.
//
//	@Summary	Get the problem package workspace
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Problem ID"
//	@Success	200			{object}	dto.WorkspaceResponse
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Param		domain		path		string	true	"Domain slug"
//	@Router		/api/admin/problems/{id}/package [get]
//	@Router		/api/domains/{domain}/admin/problems/{id}/package [get]
func (h *PackageHandler) Workspace(c *gin.Context) {
	workspace, err := h.service.Workspace(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.FromWorkspace(*workspace))
}

// Templates lists the built-in testlib starters.
//
//	@Summary	List package source templates
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200		{object}	httpx.ListResponse[dto.TemplateResponse]
//	@Failure	401,403	{object}	httpx.ErrorResponse
//	@Param		domain	path		string	true	"Domain slug"
//	@Router		/api/admin/package-templates [get]
//	@Router		/api/domains/{domain}/admin/package-templates [get]
func (h *PackageHandler) Templates(c *gin.Context) {
	templates := dto.FromTemplates(authoringdomain.Templates())
	c.JSON(http.StatusOK, httpx.ListResponse[dto.TemplateResponse]{
		Items: templates, Total: len(templates),
	})
}

// ---------- statements ----------

// ListStatements returns every localized statement of a package.
//
//	@Summary	List package statements
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Problem ID"
//	@Success	200			{object}	httpx.ListResponse[dto.StatementResponse]
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Param		domain		path		string	true	"Domain slug"
//	@Router		/api/admin/problems/{id}/statements [get]
//	@Router		/api/domains/{domain}/admin/problems/{id}/statements [get]
func (h *PackageHandler) ListStatements(c *gin.Context) {
	statements, err := h.service.Statements(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	items := dto.FromStatements(statements)
	c.JSON(http.StatusOK, httpx.ListResponse[dto.StatementResponse]{Items: items, Total: len(items)})
}

// SaveStatement upserts one localized statement and refreshes the published
// Markdown when it is the problem's primary language.
//
//	@Summary	Save a package statement
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id					path		string						true	"Problem ID"
//	@Param		language			path		string						true	"Statement language"
//	@Param		request				body		dto.StatementUpsertRequest	true	"Statement"
//	@Success	200					{object}	dto.StatementResponse
//	@Failure	400,401,403,404,413	{object}	httpx.ErrorResponse
//	@Param		domain				path		string	true	"Domain slug"
//	@Router		/api/admin/problems/{id}/statements/{language} [put]
//	@Router		/api/domains/{domain}/admin/problems/{id}/statements/{language} [put]
func (h *PackageHandler) SaveStatement(c *gin.Context) {
	var request dto.StatementUpsertRequest
	if !httpx.BindJSON(c, &request, maxPackageBody, "invalid statement payload") {
		return
	}
	saved, err := h.service.SaveStatement(
		c.Request.Context(), request.Domain(c.Param("id"), c.Param("language")))
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.FromStatement(*saved))
}

// DeleteStatement removes one localized statement.
//
//	@Summary	Delete a package statement
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Problem ID"
//	@Param		language	path		string	true	"Statement language"
//	@Success	200			{object}	httpx.StatusResponse
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Param		domain		path		string	true	"Domain slug"
//	@Router		/api/admin/problems/{id}/statements/{language} [delete]
//	@Router		/api/domains/{domain}/admin/problems/{id}/statements/{language} [delete]
func (h *PackageHandler) DeleteStatement(c *gin.Context) {
	if err := h.service.DeleteStatement(c.Request.Context(), c.Param("id"), c.Param("language")); err != nil {
		writeAuthoringError(c, err)
		return
	}
	c.JSON(http.StatusOK, httpx.StatusResponse{Status: "deleted"})
}

// PreviewStatement renders a draft without saving it, using the samples from
// the last successful build.
//
//	@Summary	Preview a statement as published Markdown
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id				path		string						true	"Problem ID"
//	@Param		language		path		string						true	"Statement language"
//	@Param		request			body		dto.StatementUpsertRequest	true	"Statement draft"
//	@Success	200				{object}	dto.StatementPreviewResponse
//	@Failure	400,401,403,413	{object}	httpx.ErrorResponse
//	@Param		domain			path		string	true	"Domain slug"
//	@Router		/api/admin/problems/{id}/statements/{language}/preview [post]
//	@Router		/api/domains/{domain}/admin/problems/{id}/statements/{language}/preview [post]
func (h *PackageHandler) PreviewStatement(c *gin.Context) {
	var request dto.StatementUpsertRequest
	if !httpx.BindJSON(c, &request, maxPackageBody, "invalid statement payload") {
		return
	}
	problemID := c.Param("id")
	samples, err := h.service.CandidateSamples(c.Request.Context(), problemID)
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	rendered := h.service.PreviewStatement(
		c.Request.Context(), request.Domain(problemID, c.Param("language")), samples)
	c.JSON(http.StatusOK, dto.StatementPreviewResponse{StatementMD: rendered})
}

// ---------- files ----------

// ListFiles returns package sources without their bodies.
//
//	@Summary	List package files
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Problem ID"
//	@Success	200			{object}	httpx.ListResponse[dto.FileResponse]
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Param		domain		path		string	true	"Domain slug"
//	@Router		/api/admin/problems/{id}/files [get]
//	@Router		/api/domains/{domain}/admin/problems/{id}/files [get]
func (h *PackageHandler) ListFiles(c *gin.Context) {
	files, err := h.service.Files(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	items := dto.FromFiles(files)
	c.JSON(http.StatusOK, httpx.ListResponse[dto.FileResponse]{Items: items, Total: len(items)})
}

// GetFile returns one source file including its body.
//
//	@Summary	Get a package file
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Problem ID"
//	@Param		fileId		path		int		true	"File ID"
//	@Success	200			{object}	dto.FileResponse
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Param		domain		path		string	true	"Domain slug"
//	@Router		/api/admin/problems/{id}/files/{fileId} [get]
//	@Router		/api/domains/{domain}/admin/problems/{id}/files/{fileId} [get]
func (h *PackageHandler) GetFile(c *gin.Context) {
	id, err := pathInt64(c, "fileId")
	if err != nil {
		writeAPIError(c, http.StatusBadRequest, "request.invalid", "invalid file ID")
		return
	}
	file, err := h.service.File(c.Request.Context(), c.Param("id"), id)
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.FromFile(*file))
}

// SaveFile upserts a source file by kind and name.
//
//	@Summary	Save a package file
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id					path		string					true	"Problem ID"
//	@Param		request				body		dto.FileUpsertRequest	true	"File"
//	@Success	200					{object}	dto.FileResponse
//	@Failure	400,401,403,404,413	{object}	httpx.ErrorResponse
//	@Param		domain				path		string	true	"Domain slug"
//	@Router		/api/admin/problems/{id}/files [put]
//	@Router		/api/domains/{domain}/admin/problems/{id}/files [put]
func (h *PackageHandler) SaveFile(c *gin.Context) {
	var request dto.FileUpsertRequest
	if !httpx.BindJSON(c, &request, maxPackageBody, "kind, name, language and sourceCode are required") {
		return
	}
	saved, err := h.service.SaveFile(c.Request.Context(), request.Domain(c.Param("id")))
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.FromFile(*saved))
}

// DeleteFile removes a source file from the package.
//
//	@Summary	Delete a package file
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Problem ID"
//	@Param		fileId		path		int		true	"File ID"
//	@Success	200			{object}	httpx.StatusResponse
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Param		domain		path		string	true	"Domain slug"
//	@Router		/api/admin/problems/{id}/files/{fileId} [delete]
//	@Router		/api/domains/{domain}/admin/problems/{id}/files/{fileId} [delete]
func (h *PackageHandler) DeleteFile(c *gin.Context) {
	id, err := pathInt64(c, "fileId")
	if err != nil {
		writeAPIError(c, http.StatusBadRequest, "request.invalid", "invalid file ID")
		return
	}
	if err := h.service.DeleteFile(c.Request.Context(), c.Param("id"), id); err != nil {
		writeAuthoringError(c, err)
		return
	}
	c.JSON(http.StatusOK, httpx.StatusResponse{Status: "deleted"})
}

// ---------- tests ----------

// ListTests returns the test plan with truncated inline inputs.
//
//	@Summary	List package tests
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Problem ID"
//	@Success	200			{object}	httpx.ListResponse[dto.TestResponse]
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Param		domain		path		string	true	"Domain slug"
//	@Router		/api/admin/problems/{id}/tests [get]
//	@Router		/api/domains/{domain}/admin/problems/{id}/tests [get]
func (h *PackageHandler) ListTests(c *gin.Context) {
	tests, err := h.service.Tests(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	items := dto.FromTests(tests)
	c.JSON(http.StatusOK, httpx.ListResponse[dto.TestResponse]{Items: items, Total: len(items)})
}

// CreateTest appends one test to the plan.
//
//	@Summary	Add a package test
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id					path		string					true	"Problem ID"
//	@Param		request				body		dto.TestUpsertRequest	true	"Test"
//	@Success	201					{object}	dto.TestResponse
//	@Failure	400,401,403,404,413	{object}	httpx.ErrorResponse
//	@Param		domain				path		string	true	"Domain slug"
//	@Router		/api/admin/problems/{id}/tests [post]
//	@Router		/api/domains/{domain}/admin/problems/{id}/tests [post]
func (h *PackageHandler) CreateTest(c *gin.Context) {
	var request dto.TestUpsertRequest
	if !httpx.BindJSON(c, &request, maxPackageBody, "source is required") {
		return
	}
	created, err := h.service.CreateTest(c.Request.Context(), request.Domain(c.Param("id"), 0))
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	c.JSON(http.StatusCreated, dto.FromTest(*created))
}

// UpdateTest replaces one test definition.
//
//	@Summary	Update a package test
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id					path		string					true	"Problem ID"
//	@Param		testId				path		int						true	"Test ID"
//	@Param		request				body		dto.TestUpsertRequest	true	"Test"
//	@Success	200					{object}	dto.TestResponse
//	@Failure	400,401,403,404,413	{object}	httpx.ErrorResponse
//	@Param		domain				path		string	true	"Domain slug"
//	@Router		/api/admin/problems/{id}/tests/{testId} [put]
//	@Router		/api/domains/{domain}/admin/problems/{id}/tests/{testId} [put]
func (h *PackageHandler) UpdateTest(c *gin.Context) {
	id, err := pathInt64(c, "testId")
	if err != nil {
		writeAPIError(c, http.StatusBadRequest, "request.invalid", "invalid test ID")
		return
	}
	var request dto.TestUpsertRequest
	if !httpx.BindJSON(c, &request, maxPackageBody, "source is required") {
		return
	}
	updated, err := h.service.UpdateTest(c.Request.Context(), request.Domain(c.Param("id"), id))
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.FromTest(*updated))
}

// DeleteTest removes a test and closes the numbering gap it leaves.
//
//	@Summary	Delete a package test
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Problem ID"
//	@Param		testId		path		int		true	"Test ID"
//	@Success	200			{object}	httpx.StatusResponse
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Param		domain		path		string	true	"Domain slug"
//	@Router		/api/admin/problems/{id}/tests/{testId} [delete]
//	@Router		/api/domains/{domain}/admin/problems/{id}/tests/{testId} [delete]
func (h *PackageHandler) DeleteTest(c *gin.Context) {
	id, err := pathInt64(c, "testId")
	if err != nil {
		writeAPIError(c, http.StatusBadRequest, "request.invalid", "invalid test ID")
		return
	}
	if err := h.service.DeleteTest(c.Request.Context(), c.Param("id"), id); err != nil {
		writeAuthoringError(c, err)
		return
	}
	c.JSON(http.StatusOK, httpx.StatusResponse{Status: "deleted"})
}

// MoveTest changes a test's position in the plan.
//
//	@Summary	Reorder a package test
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id					path		string				true	"Problem ID"
//	@Param		testId				path		int					true	"Test ID"
//	@Param		request				body		dto.TestMoveRequest	true	"Target position"
//	@Success	200					{object}	httpx.StatusResponse
//	@Failure	400,401,403,404,413	{object}	httpx.ErrorResponse
//	@Param		domain				path		string	true	"Domain slug"
//	@Router		/api/admin/problems/{id}/tests/{testId}/move [post]
//	@Router		/api/domains/{domain}/admin/problems/{id}/tests/{testId}/move [post]
func (h *PackageHandler) MoveTest(c *gin.Context) {
	id, err := pathInt64(c, "testId")
	if err != nil {
		writeAPIError(c, http.StatusBadRequest, "request.invalid", "invalid test ID")
		return
	}
	var request dto.TestMoveRequest
	if !httpx.BindJSON(c, &request, maxPackageControlBody, "position is required") {
		return
	}
	if err := h.service.ReorderTest(c.Request.Context(), c.Param("id"), id, request.Position); err != nil {
		writeAuthoringError(c, err)
		return
	}
	c.JSON(http.StatusOK, httpx.StatusResponse{Status: "moved"})
}

// ---------- builds ----------

// StartBuild queues a sandboxed package build.
//
//	@Summary	Build the problem package
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id					path		string	true	"Problem ID"
//	@Success	202					{object}	dto.BuildResponse
//	@Failure	400,401,403,404,409	{object}	httpx.ErrorResponse
//	@Param		domain				path		string	true	"Domain slug"
//	@Router		/api/admin/problems/{id}/builds [post]
//	@Router		/api/domains/{domain}/admin/problems/{id}/builds [post]
func (h *PackageHandler) StartBuild(c *gin.Context) {
	build, err := h.service.Build(c.Request.Context(), c.Param("id"), middleware.CurrentUserID(c))
	if errors.Is(err, authoringdomain.ErrBuildRunning) && build != nil {
		c.JSON(http.StatusConflict, dto.FromBuild(*build))
		return
	}
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, dto.FromBuild(*build))
}

// ListBuilds returns recent build attempts, newest first.
//
//	@Summary	List package builds
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Problem ID"
//	@Param		limit		query		int		false	"Maximum builds"
//	@Success	200			{object}	httpx.ListResponse[dto.BuildResponse]
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Param		domain		path		string	true	"Domain slug"
//	@Router		/api/admin/problems/{id}/builds [get]
//	@Router		/api/domains/{domain}/admin/problems/{id}/builds [get]
func (h *PackageHandler) ListBuilds(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	builds, err := h.service.Builds(c.Request.Context(), c.Param("id"), limit)
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	items := dto.FromBuilds(builds)
	c.JSON(http.StatusOK, httpx.ListResponse[dto.BuildResponse]{Items: items, Total: len(items)})
}

// GetBuild returns one build with its log and per-test report.
//
//	@Summary	Get a package build
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Problem ID"
//	@Param		buildId		path		string	true	"Build ID"
//	@Success	200			{object}	dto.BuildResponse
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Param		domain		path		string	true	"Domain slug"
//	@Router		/api/admin/problems/{id}/builds/{buildId} [get]
//	@Router		/api/domains/{domain}/admin/problems/{id}/builds/{buildId} [get]
func (h *PackageHandler) GetBuild(c *gin.Context) {
	build, err := h.service.BuildStatus(c.Request.Context(), c.Param("id"), c.Param("buildId"))
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.FromBuild(*build))
}

// CancelBuild stops a queued or running build.
//
//	@Summary	Cancel a package build
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Problem ID"
//	@Param		buildId		path		string	true	"Build ID"
//	@Success	200			{object}	httpx.StatusResponse
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Param		domain		path		string	true	"Domain slug"
//	@Router		/api/admin/problems/{id}/builds/{buildId}/cancel [post]
//	@Router		/api/domains/{domain}/admin/problems/{id}/builds/{buildId}/cancel [post]
func (h *PackageHandler) CancelBuild(c *gin.Context) {
	if err := h.service.CancelBuild(c.Request.Context(), c.Param("id"), c.Param("buildId")); err != nil {
		writeAuthoringError(c, err)
		return
	}
	c.JSON(http.StatusOK, httpx.StatusResponse{Status: "cancelled"})
}

func pathInt64(c *gin.Context, name string) (int64, error) {
	return strconv.ParseInt(c.Param(name), 10, 64)
}

func writeAPIError(c *gin.Context, status int, code, message string) {
	httpx.WriteError(c, status, code, message)
}

// writeAuthoringError maps domain errors onto one HTTP vocabulary so every
// handler above stays a straight-line function.
func writeAuthoringError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, tenancydomain.ErrForbidden):
		writeAPIError(c, http.StatusForbidden, "authoring.forbidden", "insufficient problem permissions")
	case errors.Is(err, tenancydomain.ErrUnauthenticated):
		writeAPIError(c, http.StatusUnauthorized, "auth.invalid_token", "authentication required")
	case errors.Is(err, authoringdomain.ErrNotFound):
		writeAPIError(c, http.StatusNotFound, "authoring.not_found", "resource not found")
	case errors.Is(err, authoringdomain.ErrInvalidInput):
		writeAPIError(c, http.StatusBadRequest, "request.invalid", err.Error())
	case errors.Is(err, authoringdomain.ErrNotBuildable):
		writeAPIError(c, http.StatusBadRequest, "authoring.not_buildable", err.Error())
	case errors.Is(err, authoringdomain.ErrBuildRunning):
		writeAPIError(c, http.StatusConflict, "authoring.build_running", err.Error())
	case errors.Is(err, authoringdomain.ErrRevisionConflict), errors.Is(err, authoringdomain.ErrNotPublished):
		writeAPIError(c, http.StatusConflict, "authoring.publish_conflict", err.Error())
	case errors.Is(err, authoringdomain.ErrPackageTooBig):
		writeAPIError(c, http.StatusRequestEntityTooLarge, "authoring.package_too_large", err.Error())
	case errors.Is(err, authoringdomain.ErrStaleLease):
		writeAPIError(c, http.StatusConflict, "authoring.stale_lease", err.Error())
	default:
		writeAPIError(c, http.StatusInternalServerError, "authoring.failed", "authoring request failed")
	}
}
