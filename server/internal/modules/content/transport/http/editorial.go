package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	contentapp "github.com/RimuruChan/Vertex/server/internal/modules/content/application"
	contentdomain "github.com/RimuruChan/Vertex/server/internal/modules/content/domain"
	dto "github.com/RimuruChan/Vertex/server/internal/modules/content/transport/http/dto"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

const (
	maxEditorialBody = 1 << 20
	maxVoteBody      = 16 << 10
)

type EditorialListService interface {
	ListEditorials(ctx context.Context, filters contentdomain.EditorialFilters) ([]contentdomain.EditorialSummary, int, error)
}

type EditorialHandler struct {
	service *contentapp.Service
	list    EditorialListService
}

func NewEditorialHandler(service *contentapp.Service) *EditorialHandler {
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
//	@Param		domain	path		string	true	"Domain slug"
//	@Router		/api/editorials [get]
//	@Router		/api/domains/{domain}/editorials [get]
func (h *EditorialHandler) List(c *gin.Context) {
	page, size := pagination(c)
	viewerID := middleware.CurrentUserID(c)
	items, total, err := h.list.ListEditorials(c.Request.Context(), contentdomain.EditorialFilters{
		ProblemID: c.Query("problem"), AuthorID: c.Query("author"), Keyword: c.Query("keyword"),
		ViewerID: viewerID, Sort: c.Query("sort"),
		Limit: size, Offset: (page - 1) * size,
	})
	if err != nil {
		writeContentError(c, err, "failed to list editorials")
		return
	}
	c.JSON(http.StatusOK, httpx.ListResponse[dto.EditorialSummaryResponse]{
		Items: dto.FromEditorialSummaries(items), Total: total,
	})
}

// Get returns one editorial. A solved-only editorial comes back without its
// body until the reader has solved the problem.
//
//	@Summary	Get an editorial
//	@Tags		editorials
//	@Produce	json
//	@Param		id		path		string	true	"Editorial ID"
//	@Success	200		{object}	dto.EditorialResponse
//	@Failure	404		{object}	httpx.ErrorResponse
//	@Param		domain	path		string	true	"Domain slug"
//	@Router		/api/editorials/{id} [get]
//	@Router		/api/domains/{domain}/editorials/{id} [get]
func (h *EditorialHandler) Get(c *gin.Context) {
	viewerID := middleware.CurrentUserID(c)
	item, err := h.service.GetEditorial(c.Request.Context(), c.Param("id"), viewerID)
	if err != nil {
		h.writeError(c, err, "failed to load the editorial")
		return
	}
	c.JSON(http.StatusOK, dto.FromEditorial(*item))
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
//	@Param		domain		path		string	true	"Domain slug"
//	@Router		/api/editorials [post]
//	@Router		/api/domains/{domain}/editorials [post]
func (h *EditorialHandler) Create(c *gin.Context) {
	var request dto.EditorialCreateRequest
	if !httpx.BindJSON(c, &request, maxEditorialBody, "problemId, title and contentMd are required") {
		return
	}
	created, err := h.service.CreateEditorial(
		c.Request.Context(), middleware.CurrentUserID(c), request.Input(),
	)
	if err != nil {
		h.writeError(c, err, "failed to create the editorial")
		return
	}
	c.JSON(http.StatusCreated, dto.FromEditorial(*created))
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
//	@Param		domain				path		string	true	"Domain slug"
//	@Router		/api/editorials/{id} [put]
//	@Router		/api/domains/{domain}/editorials/{id} [put]
func (h *EditorialHandler) Update(c *gin.Context) {
	var request dto.EditorialUpdateRequest
	if !httpx.BindJSON(c, &request, maxEditorialBody, "title and contentMd are required") {
		return
	}
	updated, err := h.service.UpdateEditorial(c.Request.Context(), c.Param("id"),
		middleware.CurrentUserID(c), request.Input())
	if err != nil {
		h.writeError(c, err, "failed to update the editorial")
		return
	}
	c.JSON(http.StatusOK, dto.FromEditorial(*updated))
}

// Delete checks authorship or resource moderation within the parent boundary.
//
//	@Summary	Delete an editorial
//	@Tags		editorials
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Editorial ID"
//	@Success	200			{object}	httpx.StatusResponse
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Param		domain		path		string	true	"Domain slug"
//	@Router		/api/editorials/{id} [delete]
//	@Router		/api/domains/{domain}/editorials/{id} [delete]
func (h *EditorialHandler) Delete(c *gin.Context) {
	err := h.service.DeleteEditorial(c.Request.Context(), c.Param("id"),
		middleware.CurrentUserID(c))
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
//	@Param		domain			path		string	true	"Domain slug"
//	@Router		/api/editorials/{id}/vote [post]
//	@Router		/api/domains/{domain}/editorials/{id}/vote [post]
func (h *EditorialHandler) Vote(c *gin.Context) {
	var request dto.EditorialVoteRequest
	if !httpx.BindJSON(c, &request, maxVoteBody, "up is required") {
		return
	}
	total, err := h.service.VoteEditorial(c.Request.Context(), c.Param("id"),
		middleware.CurrentUserID(c), request.Up)
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
	var validation *contentdomain.ValidationError
	switch {
	case errors.Is(err, tenancydomain.ErrUnauthenticated):
		writeAPIError(c, http.StatusUnauthorized, "auth.invalid_token", "authentication required")
	case errors.As(err, &validation):
		writeAPIError(c, http.StatusBadRequest, "request.invalid", validation.Message)
	case errors.Is(err, contentdomain.ErrNotFound), errors.Is(err, tenancydomain.ErrNotFound):
		writeAPIError(c, http.StatusNotFound, "content.not_found", "content not found")
	case errors.Is(err, contentdomain.ErrForbidden), errors.Is(err, tenancydomain.ErrForbidden):
		writeAPIError(c, http.StatusForbidden, "content.forbidden", "insufficient content permissions")
	case errors.Is(err, contentdomain.ErrSpoilerLocked):
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
