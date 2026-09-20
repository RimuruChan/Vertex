package httpapi

import (
	"net/http"

	problemapp "github.com/RimuruChan/Vertex/server/internal/modules/problem/application"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
	dto "github.com/RimuruChan/Vertex/server/internal/modules/problem/transport/http/dto"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

const maxProblemBody = 1 << 20

// AdminProblemHandler retains the legacy URL namespace; authorization is by
// domain and resource ownership, not by a global administrator role.
type AdminProblemHandler struct {
	service *problemapp.Service
}

func NewAdminProblemHandler(service *problemapp.Service) *AdminProblemHandler {
	return &AdminProblemHandler{service: service}
}

// List includes the caller's owned/shared packages and domain-managed resources.
//
//	@Summary	List accessible authoring problems
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
//	@Param		domain		path		string	true	"Domain slug"
//	@Router		/api/domains/{domain}/admin/problems [get]
func (h *AdminProblemHandler) List(c *gin.Context) {
	page, size := pagination(c)
	f := problemdomain.Filters{
		ViewerID:   middleware.CurrentUserID(c),
		Visibility: c.Query("visibility"),
		Tag:        c.Query("tag"),
		Difficulty: parseIntDefault(c.Query("difficulty"), 0),
		Keyword:    c.Query("keyword"),
		Limit:      size,
		Offset:     (page - 1) * size,
	}
	list, total, err := h.service.List(c.Request.Context(), f, true)
	if err != nil {
		writeProblemError(c, err)
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
//	@Param		domain			path		string	true	"Domain slug"
//	@Router		/api/domains/{domain}/admin/problems [post]
func (h *AdminProblemHandler) Create(c *gin.Context) {
	var request dto.ProblemUpsertRequest
	if !httpx.BindJSON(c, &request, maxProblemBody, "title is required") {
		return
	}
	p, err := h.service.Create(c.Request.Context(), middleware.CurrentUserID(c), problemInput(request))
	if err != nil {
		writeProblemError(c, err)
		return
	}
	c.JSON(http.StatusCreated, dto.FromProblem(*p, true))
}

// Get returns problem metadata with the caller's current capabilities.
//
//	@Summary	Get authoring problem
//	@Tags		admin
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Problem ID"
//	@Success	200			{object}	dto.ProblemResponse
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Param		domain		path		string	true	"Domain slug"
//	@Router		/api/domains/{domain}/admin/problems/{id} [get]
func (h *AdminProblemHandler) Get(c *gin.Context) {
	p, err := h.service.GetWorkspace(c.Request.Context(), httpx.ResourceID(c, "id"), middleware.CurrentUserID(c))
	if err != nil {
		writeProblemError(c, err)
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
//	@Param		domain		path		string	true	"Domain slug"
//	@Router		/api/domains/{domain}/admin/problems/{id} [delete]
func (h *AdminProblemHandler) Delete(c *gin.Context) {
	if err := h.service.Delete(c.Request.Context(), httpx.ResourceID(c, "id")); err != nil {
		writeProblemError(c, err)
		return
	}
	c.JSON(http.StatusOK, httpx.StatusResponse{Status: "deleted"})
}

func problemInput(request dto.ProblemUpsertRequest) problemdomain.CreateInput {
	return request.CreateInput()
}
