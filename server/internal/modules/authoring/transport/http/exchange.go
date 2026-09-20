package handler

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/transport/http/dto"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/gin-gonic/gin"
)

// PreviewImport stages source material without changing the private copy.
//
//	@Summary	Preview a problem package import
//	@Tags		authoring
//	@Accept		multipart/form-data
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain					path		string	true	"Domain slug"
//	@Param		id						path		string	true	"Problem number"
//	@Param		file					formData	file	true	"ZIP or KPP package"
//	@Param		etag					formData	string	true	"Expected working copy token"
//	@Param		format					formData	string	false	"Expected format"
//	@Param		timeLimitMs				formData	integer	false	"Explicit time limit when absent from the package"
//	@Success	200						{object}	domain.ImportReceipt
//	@Failure	400,401,403,404,409,413	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/imports [post]
func (handler *WorkbenchHandler) PreviewImport(c *gin.Context) {
	if err := handler.service.AuthorizeEdit(c.Request.Context(), httpx.ResourceID(c, "id")); err != nil {
		writeAuthoringError(c, err)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, (64<<20)+(64<<10))
	err := c.Request.ParseMultipartForm(1 << 20)
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeAuthoringError(c, domain.ErrPackageTooBig)
			return
		}
		httpx.WriteError(c, 400, "request.invalid", "invalid package upload")
		return
	}
	files := c.Request.MultipartForm.File["file"]
	if len(files) != 1 || len(c.Request.MultipartForm.File) != 1 {
		writeAuthoringError(c, domain.InvalidInput("upload exactly one package"))
		return
	}
	etag := c.Request.FormValue("etag")
	if etag == "" {
		writeAuthoringError(c, domain.InvalidInput("working copy token required"))
		return
	}
	limit := 0
	if raw := c.Request.FormValue("timeLimitMs"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit <= 0 || limit > 3600000 {
			writeAuthoringError(c, domain.InvalidInput("invalid import time limit"))
			return
		}
	}
	file, err := files[0].Open()
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (64<<20)+1))
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	if len(data) > 64<<20 {
		writeAuthoringError(c, domain.ErrPackageTooBig)
		return
	}
	value, err := handler.service.PreviewImport(c.Request.Context(), httpx.ResourceID(c, "id"), etag, data, domain.ImportOptions{Format: c.Request.FormValue("format"), TimeLimitMs: limit})
	workbenchResult(c, value, err)
}

// Import returns only the authenticated actor's staged package.
//
//	@Summary	Read my package import preview
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain		path		string	true	"Domain slug"
//	@Param		id			path		string	true	"Problem number"
//	@Param		importId	path		string	true	"Import ID"
//	@Success	200			{object}	domain.ImportReceipt
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/imports/{importId} [get]
func (handler *WorkbenchHandler) Import(c *gin.Context) {
	value, err := handler.service.Import(c.Request.Context(), httpx.ResourceID(c, "id"), c.Param("importId"))
	workbenchResult(c, value, err)
}

// ApplyImport atomically replaces my copy with the previewed material tree.
//
//	@Summary	Apply a reviewed package import
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain				path		string						true	"Domain slug"
//	@Param		id					path		string						true	"Problem number"
//	@Param		importId			path		string						true	"Import ID"
//	@Param		request				body		dto.WorkingCopyTokenRequest	true	"Expected working copy token"
//	@Success	200					{object}	domain.WorkingCopy
//	@Failure	400,401,403,404,409	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/imports/{importId}/apply [post]
func (handler *WorkbenchHandler) ApplyImport(c *gin.Context) {
	var input dto.WorkingCopyTokenRequest
	if !httpx.BindJSON(c, &input, maxPackageControlBody, "working copy token required") {
		return
	}
	value, err := handler.service.ApplyImport(c.Request.Context(), httpx.ResourceID(c, "id"), c.Param("importId"), input.ETag)
	workbenchResult(c, value, err)
}

// ExportPackage returns compatibility notes and a private download reference.
//
//	@Summary	Export authoring source material
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		domain				path		string						true	"Domain slug"
//	@Param		id					path		string						true	"Problem number"
//	@Param		request				body		dto.PackageExportRequest	true	"Format and optional immutable revision"
//	@Success	200					{object}	domain.PackageExport
//	@Failure	400,401,403,404,413	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/authoring/problems/{id}/exports [post]
func (handler *WorkbenchHandler) ExportPackage(c *gin.Context) {
	var input dto.PackageExportRequest
	if !httpx.BindJSON(c, &input, maxPackageControlBody, "export format required") {
		return
	}
	value, err := handler.service.ExportPackage(c.Request.Context(), httpx.ResourceID(c, "id"), input.Revision, input.Format)
	workbenchResult(c, value, err)
}
