package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/content"
	"github.com/RimuruChan/Vertex/server/internal/content/dto"
	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/gin-gonic/gin"
)

const (
	maxEditorialBody = 1 << 20
	maxVoteBody      = 16 << 10
)

type EditorialListService interface {
	ListEditorials(ctx context.Context, filters content.EditorialFilters) ([]content.EditorialSummary, int, error)
}

type EditorialHandler struct {
	service *content.Service
	list    EditorialListService
}

func NewEditorialHandler(service *content.Service) *EditorialHandler {
	return &EditorialHandler{service: service, list: service}
}

// List returns editorials, optionally scoped to one problem or author.
//
//	@Summary	List editorials
//	@Tags		editorials
//	@Produce	json
//	@Param		problem	query		string	false	"Problem ID"
//	@Param		author	query		string	false	"Author user ID"
//	@Param		keyword	query		string	false	"Search text"
//	@Param		sort	query		string	false	"Ordering"	Enums(recent, votes)
//	@Param		page	query		int		false	"Page"
//	@Param		size	query		int		false	"Page size"
//	@Success	200		{object}	httpx.ListResponse[dto.EditorialSummaryResponse]
//	@Router		/api/editorials [get]
func (h *EditorialHandler) List(c *gin.Context) {
	page, size := pagination(c)
	viewerID := middleware.CurrentUserID(c)
	admin := middleware.CurrentRole(c) == "admin"
	items, total, err := h.list.ListEditorials(c.Request.Context(), content.EditorialFilters{
		ProblemID: c.Query("problem"), AuthorID: c.Query("author"), Keyword: c.Query("keyword"),
		ViewerID: viewerID, Admin: admin, Sort: c.Query("sort"),
		Limit: size, Offset: (page - 1) * size,
	})
	if err != nil {
		writeAPIError(c, http.StatusInternalServerError, "editorial.list_failed", "failed to list editorials")
		return
	}
	c.JSON(http.StatusOK, httpx.ListResponse[dto.EditorialSummaryResponse]{
		Items: dto.FromEditorialSummaries(items, viewerID, admin), Total: total,
	})
}

// Get returns one editorial. A solved-only editorial comes back without its
// body until the reader has solved the problem.
//
//	@Summary	Get an editorial
//	@Tags		editorials
//	@Produce	json
//	@Param		id	path		string	true	"Editorial ID"
//	@Success	200	{object}	dto.EditorialResponse
//	@Failure	404	{object}	httpx.ErrorResponse
//	@Router		/api/editorials/{id} [get]
func (h *EditorialHandler) Get(c *gin.Context) {
	viewerID := middleware.CurrentUserID(c)
	admin := middleware.CurrentRole(c) == "admin"
	item, err := h.service.GetEditorial(c.Request.Context(), c.Param("id"), viewerID, admin)
	if err != nil {
		h.writeError(c, err, "failed to load the editorial")
		return
	}
	c.JSON(http.StatusOK, dto.FromEditorial(*item, item.CanEdit(viewerID, admin)))
}

// Create publishes a new editorial owned by the caller.
//
//	@Summary	Create an editorial
//	@Tags		editorials
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		request		body		dto.EditorialCreateRequest	true	"Editorial"
//	@Success	201			{object}	dto.EditorialResponse
//	@Failure	400,401,413	{object}	httpx.ErrorResponse
//	@Router		/api/editorials [post]
func (h *EditorialHandler) Create(c *gin.Context) {
	var request dto.EditorialCreateRequest
	if !httpx.BindJSON(c, &request, maxEditorialBody, "problemId, title and contentMd are required") {
		return
	}
	created, err := h.service.CreateEditorial(
		c.Request.Context(), middleware.CurrentUserID(c), middleware.CurrentRole(c) == "admin", request.Input(),
	)
	if err != nil {
		h.writeError(c, err, "failed to create the editorial")
		return
	}
	c.JSON(http.StatusCreated, dto.FromEditorial(*created, true))
}

// Update rewrites an editorial the caller owns.
//
//	@Summary	Update an editorial
//	@Tags		editorials
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id					path		string						true	"Editorial ID"
//	@Param		request				body		dto.EditorialUpdateRequest	true	"Editorial"
//	@Success	200					{object}	dto.EditorialResponse
//	@Failure	400,401,403,404,413	{object}	httpx.ErrorResponse
//	@Router		/api/editorials/{id} [put]
func (h *EditorialHandler) Update(c *gin.Context) {
	var request dto.EditorialUpdateRequest
	if !httpx.BindJSON(c, &request, maxEditorialBody, "title and contentMd are required") {
		return
	}
	updated, err := h.service.UpdateEditorial(c.Request.Context(), c.Param("id"),
		middleware.CurrentUserID(c), middleware.CurrentRole(c) == "admin", request.Input())
	if err != nil {
		h.writeError(c, err, "failed to update the editorial")
		return
	}
	c.JSON(http.StatusOK, dto.FromEditorial(*updated, true))
}

// Delete removes an editorial. Administrators may remove any editorial as
// moderation; everyone else only their own.
//
//	@Summary	Delete an editorial
//	@Tags		editorials
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Editorial ID"
//	@Success	200			{object}	httpx.StatusResponse
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/editorials/{id} [delete]
func (h *EditorialHandler) Delete(c *gin.Context) {
	err := h.service.DeleteEditorial(c.Request.Context(), c.Param("id"),
		middleware.CurrentUserID(c), middleware.CurrentRole(c) == "admin")
	if err != nil {
		h.writeError(c, err, "failed to delete the editorial")
		return
	}
	c.JSON(http.StatusOK, httpx.StatusResponse{Status: "deleted"})
}

// Vote records or withdraws the caller's upvote.
//
//	@Summary	Vote for an editorial
//	@Tags		editorials
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id				path		string						true	"Editorial ID"
//	@Param		request			body		dto.EditorialVoteRequest	true	"Vote"
//	@Success	200				{object}	dto.EditorialVoteResponse
//	@Failure	400,401,404,413	{object}	httpx.ErrorResponse
//	@Router		/api/editorials/{id}/vote [post]
func (h *EditorialHandler) Vote(c *gin.Context) {
	var request dto.EditorialVoteRequest
	if !httpx.BindJSON(c, &request, maxVoteBody, "up is required") {
		return
	}
	total, err := h.service.VoteEditorial(c.Request.Context(), c.Param("id"),
		middleware.CurrentUserID(c), middleware.CurrentRole(c) == "admin", request.Up)
	if err != nil {
		h.writeError(c, err, "failed to record the vote")
		return
	}
	c.JSON(http.StatusOK, dto.EditorialVoteResponse{VoteCount: total, Voted: request.Up})
}

func (h *EditorialHandler) writeError(c *gin.Context, err error, fallback string) {
	writeContentError(c, err, fallback)
}

func writeContentError(c *gin.Context, err error, fallback string) {
	var validation *content.ValidationError
	switch {
	case errors.As(err, &validation):
		writeAPIError(c, http.StatusBadRequest, "request.invalid", validation.Message)
	case errors.Is(err, content.ErrNotFound):
		writeAPIError(c, http.StatusNotFound, "content.not_found", "content not found")
	case errors.Is(err, content.ErrForbidden):
		writeAPIError(c, http.StatusForbidden, "content.forbidden", "only the author may change this")
	case errors.Is(err, content.ErrSpoilerLocked):
		writeAPIError(c, http.StatusForbidden, "content.solve_required", err.Error())
	default:
		writeAPIError(c, http.StatusInternalServerError, "content.failed", fallback)
	}
}

func writeAPIError(c *gin.Context, status int, code, message string) {
	httpx.WriteError(c, status, code, message)
}

func pagination(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.Query("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(c.Query("size"))
	if size < 1 || size > 100 {
		size = 20
	}
	return page, size
}
