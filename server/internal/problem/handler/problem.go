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

// List defaults to the public library. The available view is a permission-filtered reuse picker.
//
//	@Summary	List published problems
//	@Tags		problems
//	@Produce	json
//	@Param		difficulty	query		int		false	"Difficulty"
//	@Param		tag			query		string	false	"Tag"
//	@Param		keyword		query		string	false	"Search text"
//	@Param		status		query		string	false	"Viewer progress filter"						Enums(solved, attempted, none)
//
//	@Param		view		query		string	false	"Public library or authorized reuse candidates"	Enums(public,available)
//
//	@Param		page		query		int		false	"Page"
//	@Param		size		query		int		false	"Page size"
//	@Success	200			{object}	httpx.ListResponse[dto.ProblemResponse]
//	@Param		domain		path		string	true	"Domain slug"
//	@Router		/api/problems [get]
//	@Router		/api/domains/{domain}/problems [get]
func (h *ProblemHandler) List(c *gin.Context) {
	view := c.Query("view")
	if view != "" && view != "public" && view != "available" {
		writeAPIError(c, http.StatusBadRequest, "request.invalid", "invalid problem view")
		return
	}
	page, size := pagination(c)
	f := problem.Filters{
		Available:  view == "available",
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
		writeProblemError(c, err)
		return
	}
	c.JSON(http.StatusOK, httpx.ListResponse[dto.ProblemResponse]{Items: dto.FromProblems(list, false), Total: total})
}

// Get exposes unpublished statements only to the owner, collaborators and
// domain resource managers. Role claims alone do not grant access.
//
//	@Summary	Get problem
//	@Tags		problems
//	@Produce	json
//	@Param		id		path		string	true	"Problem ID"
//	@Success	200		{object}	dto.ProblemResponse
//	@Failure	404		{object}	httpx.ErrorResponse
//	@Param		domain	path		string	true	"Domain slug"
//	@Router		/api/problems/{id} [get]
//	@Router		/api/domains/{domain}/problems/{id} [get]
func (h *ProblemHandler) Get(c *gin.Context) {
	p, err := h.service.Get(
		c.Request.Context(), c.Param("id"),
		middleware.CurrentUserID(c), middleware.CurrentRole(c) == "admin",
	)
	if err != nil {
		writeProblemError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.FromProblem(*p, true))
}

// Tags lists the public tag catalogue used by the problem-set filters.
//
//	@Summary	List problem tags
//	@Tags		problems
//	@Produce	json
//	@Success	200		{object}	httpx.ListResponse[dto.TagResponse]
//	@Param		domain	path		string	true	"Domain slug"
//	@Router		/api/tags [get]
//	@Router		/api/domains/{domain}/tags [get]
func (h *ProblemHandler) Tags(c *gin.Context) {
	tags, err := h.service.Tags(c.Request.Context())
	if err != nil {
		writeProblemError(c, err)
		return
	}
	c.JSON(http.StatusOK, httpx.ListResponse[dto.TagResponse]{Items: dto.FromTags(tags), Total: len(tags)})
}
