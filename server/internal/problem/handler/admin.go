package handler

import (
	"errors"
	"io"
	"net/http"

	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/RimuruChan/Vertex/server/internal/problem"
	"github.com/RimuruChan/Vertex/server/internal/problem/dto"
	"github.com/gin-gonic/gin"
)

const maxProblemBody = 1 << 20

// AdminProblemHandler maps administrator workflows without exposing persistence entities.
type AdminProblemHandler struct {
	service *problem.Service
}

func NewAdminProblemHandler(service *problem.Service) *AdminProblemHandler {
	return &AdminProblemHandler{service: service}
}

// List includes draft and private problems for administration.
//
//	@Summary	List all problems
//	@Tags		admin
//	@Produce	json
//	@Security	BearerAuth
//	@Param		visibility	query		string	false	"Visibility"
//	@Param		difficulty	query		int		false	"Difficulty"
//	@Param		tag			query		string	false	"Tag"
//	@Param		keyword		query		string	false	"Search text"
//	@Param		page		query		int		false	"Page"
//	@Param		size		query		int		false	"Page size"
//	@Success	200			{object}	httpx.ListResponse[dto.ProblemResponse]
//	@Failure	401,403		{object}	httpx.ErrorResponse
//	@Router		/api/admin/problems [get]
func (h *AdminProblemHandler) List(c *gin.Context) {
	page, size := pagination(c)
	f := problem.Filters{
		Visibility: c.Query("visibility"),
		Tag:        c.Query("tag"),
		Difficulty: parseIntDefault(c.Query("difficulty"), 0),
		Keyword:    c.Query("keyword"),
		Limit:      size,
		Offset:     (page - 1) * size,
	}
	list, total, err := h.service.List(c.Request.Context(), f, true)
	if err != nil {
		writeAPIError(c, http.StatusInternalServerError, "problem.list_failed", "failed to list problems")
		return
	}
	c.JSON(http.StatusOK, httpx.ListResponse[dto.ProblemResponse]{Items: dto.FromProblems(list, true), Total: total})
}

// Create validates a problem DTO before delegating to the Problem service.
//
//	@Summary	Create problem
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		request			body		dto.ProblemUpsertRequest	true	"Problem"
//	@Success	201				{object}	dto.ProblemResponse
//	@Failure	400,401,403,413	{object}	httpx.ErrorResponse
//	@Router		/api/admin/problems [post]
func (h *AdminProblemHandler) Create(c *gin.Context) {
	var request dto.ProblemUpsertRequest
	if !httpx.BindJSON(c, &request, maxProblemBody, "title is required") {
		return
	}
	p, err := h.service.Create(c.Request.Context(), middleware.CurrentUserID(c), problemInput(request))
	if err != nil {
		if errors.Is(err, problem.ErrInvalidInput) {
			writeAPIError(c, http.StatusBadRequest, "request.invalid", err.Error())
			return
		}
		writeAPIError(c, http.StatusInternalServerError, "problem.create_failed", "failed to create problem")
		return
	}
	c.JSON(http.StatusCreated, dto.FromProblem(*p, true))
}

// Get returns the full administrator problem view.
//
//	@Summary	Get problem as admin
//	@Tags		admin
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Problem ID"
//	@Success	200			{object}	dto.ProblemResponse
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/admin/problems/{id} [get]
func (h *AdminProblemHandler) Get(c *gin.Context) {
	p, err := h.service.Get(c.Request.Context(), c.Param("id"), "", true)
	if err != nil {
		writeAPIError(c, http.StatusNotFound, "problem.not_found", "problem not found")
		return
	}
	c.JSON(http.StatusOK, dto.FromProblem(*p, true))
}

// Update replaces editable problem metadata and tags.
//
//	@Summary	Update problem
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id					path		string						true	"Problem ID"
//	@Param		request				body		dto.ProblemUpsertRequest	true	"Problem"
//	@Success	200					{object}	dto.ProblemResponse
//	@Failure	400,401,403,404,413	{object}	httpx.ErrorResponse
//	@Router		/api/admin/problems/{id} [put]
func (h *AdminProblemHandler) Update(c *gin.Context) {
	var request dto.ProblemUpsertRequest
	if !httpx.BindJSON(c, &request, maxProblemBody, "invalid problem payload") {
		return
	}
	p, err := h.service.Update(c.Request.Context(), c.Param("id"), problem.UpdateInput{CreateInput: problemInput(request)})
	if err != nil {
		if errors.Is(err, problem.ErrInvalidInput) {
			writeAPIError(c, http.StatusBadRequest, "request.invalid", err.Error())
			return
		}
		if errors.Is(err, problem.ErrNotFound) {
			writeAPIError(c, http.StatusNotFound, "problem.not_found", "problem not found")
			return
		}
		writeAPIError(c, http.StatusInternalServerError, "problem.update_failed", "failed to update problem")
		return
	}
	c.JSON(http.StatusOK, dto.FromProblem(*p, true))
}

// Delete removes the problem through the Problem service.
//
//	@Summary	Delete problem
//	@Tags		admin
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Problem ID"
//	@Success	200			{object}	httpx.StatusResponse
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/admin/problems/{id} [delete]
func (h *AdminProblemHandler) Delete(c *gin.Context) {
	if err := h.service.Delete(c.Request.Context(), c.Param("id")); err != nil {
		if errors.Is(err, problem.ErrNotFound) {
			writeAPIError(c, http.StatusNotFound, "problem.not_found", "problem not found")
			return
		}
		writeAPIError(c, http.StatusInternalServerError, "problem.delete_failed", "failed to delete problem")
		return
	}
	c.JSON(http.StatusOK, httpx.StatusResponse{Status: "deleted"})
}

// UploadTestdata bounds the archive before it reaches testdata extraction and persistence.
//
//	@Summary	Upload problem testdata
//	@Tags		admin
//	@Accept		mpfd
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Problem ID"
//	@Param		file		formData	file	true	"Testdata zip"
//	@Param		checker		formData	string	false	"Checker type"
//	@Success	200			{object}	dto.TestdataUploadResponse
//	@Failure	400,401,403	{object}	httpx.ErrorResponse
//	@Router		/api/admin/problems/{id}/testdata [post]
func (h *AdminProblemHandler) UploadTestdata(c *gin.Context) {
	// 先确认题目存在
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		writeAPIError(c, http.StatusBadRequest, "request.invalid", "file field required")
		return
	}
	defer file.Close()

	const maxZip = 64 * 1024 * 1024 // 64MB 上限
	data, err := io.ReadAll(io.LimitReader(file, maxZip+1))
	if err != nil {
		writeAPIError(c, http.StatusInternalServerError, "problem.upload_failed", "failed to read upload")
		return
	}
	if len(data) > maxZip {
		writeAPIError(c, http.StatusBadRequest, "request.too_large", "zip too large (max 64MB)")
		return
	}

	checker := c.PostForm("checker")
	if checker == "" {
		checker = "diff"
	}

	count, hash, err := h.service.SaveTestdata(c.Request.Context(), c.Param("id"), data, checker)
	if err != nil {
		if errors.Is(err, problem.ErrInvalidInput) {
			writeAPIError(c, http.StatusBadRequest, "problem.invalid_testdata", err.Error())
			return
		}
		writeAPIError(c, http.StatusInternalServerError, "problem.upload_failed", "failed to save testdata")
		return
	}
	c.JSON(http.StatusOK, dto.TestdataUploadResponse{CaseCount: count, SHA256: hash, Checker: checker})
}

func problemInput(request dto.ProblemUpsertRequest) problem.CreateInput {
	return request.CreateInput()
}
