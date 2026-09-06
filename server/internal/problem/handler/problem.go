package handler

import (
	"net/http"

	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/RimuruChan/Vertex/server/internal/problem"
	"github.com/RimuruChan/Vertex/server/internal/problem/dto"
	"github.com/gin-gonic/gin"
)

// ProblemHandler exposes the public, visibility-filtered problem view.
type ProblemHandler struct {
	service *problem.Service
}

func NewProblemHandler(service *problem.Service) *ProblemHandler {
	return &ProblemHandler{service: service}
}

// List forces public visibility regardless of caller-provided filters.
//
//	@Summary	List public problems
//	@Tags		problems
//	@Produce	json
//	@Param		difficulty	query		int		false	"Difficulty"
//	@Param		tag			query		string	false	"Tag"
//	@Param		keyword		query		string	false	"Search text"
//	@Param		status		query		string	false	"Viewer progress filter"	Enums(solved, attempted, none)
//	@Param		page		query		int		false	"Page"
//	@Param		size		query		int		false	"Page size"
//	@Success	200			{object}	httpx.ListResponse[dto.ProblemResponse]
//	@Router		/api/problems [get]
func (h *ProblemHandler) List(c *gin.Context) {
	page, size := pagination(c)
	f := problem.Filters{
		Tag:        c.Query("tag"),
		Difficulty: parseIntDefault(c.Query("difficulty"), 0),
		Keyword:    c.Query("keyword"),
		ViewerID:   middleware.CurrentUserID(c),
		Status:     c.Query("status"),
		Limit:      size,
		Offset:     (page - 1) * size,
	}
	list, total, err := h.service.List(c.Request.Context(), f, false)
	if err != nil {
		writeAPIError(c, http.StatusInternalServerError, "problem.list_failed", "failed to list problems")
		return
	}
	c.JSON(http.StatusOK, httpx.ListResponse[dto.ProblemResponse]{Items: dto.FromProblems(list, false), Total: total})
}

// Get hides unpublished problems from non-administrators.
//
//	@Summary	Get problem
//	@Tags		problems
//	@Produce	json
//	@Param		id	path		string	true	"Problem ID"
//	@Success	200	{object}	dto.ProblemResponse
//	@Failure	404	{object}	httpx.ErrorResponse
//	@Router		/api/problems/{id} [get]
func (h *ProblemHandler) Get(c *gin.Context) {
	p, err := h.service.Get(
		c.Request.Context(), c.Param("id"),
		middleware.CurrentUserID(c), middleware.CurrentRole(c) == "admin",
	)
	if err != nil {
		writeAPIError(c, http.StatusNotFound, "problem.not_found", "problem not found")
		return
	}
	c.JSON(http.StatusOK, dto.FromProblem(*p, true))
}

// Tags lists the public tag catalogue used by the problem-set filters.
//
//	@Summary	List problem tags
//	@Tags		problems
//	@Produce	json
//	@Success	200	{object}	httpx.ListResponse[dto.TagResponse]
//	@Router		/api/tags [get]
func (h *ProblemHandler) Tags(c *gin.Context) {
	tags, err := h.service.Tags(c.Request.Context())
	if err != nil {
		writeAPIError(c, http.StatusInternalServerError, "problem.tags_failed", "failed to list tags")
		return
	}
	c.JSON(http.StatusOK, httpx.ListResponse[dto.TagResponse]{Items: dto.FromTags(tags), Total: len(tags)})
}
