// Package handler exposes curated problem sets over HTTP.
package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/RimuruChan/Vertex/server/internal/problemset"
	"github.com/RimuruChan/Vertex/server/internal/problemset/dto"
	"github.com/gin-gonic/gin"
)

const maxSetBody = 1 << 20

type SetHandler struct{ service *problemset.Service }

func NewSetHandler(service *problemset.Service) *SetHandler { return &SetHandler{service: service} }

// List returns the sets the caller may see, newest first.
//
//	@Summary	List problem sets
//	@Tags		problem-sets
//	@Produce	json
//	@Param		author	query		string	false	"Author user ID"
//	@Param		keyword	query		string	false	"Search text"
//	@Param		page	query		int		false	"Page"
//	@Param		size	query		int		false	"Page size"
//	@Success	200		{object}	httpx.ListResponse[dto.SetResponse]
//	@Router		/api/problem-sets [get]
func (h *SetHandler) List(c *gin.Context) {
	page, size := pagination(c)
	viewerID := middleware.CurrentUserID(c)
	admin := middleware.CurrentRole(c) == "admin"
	items, total, err := h.service.List(c.Request.Context(), problemset.Filters{
		AuthorID: c.Query("author"), Keyword: c.Query("keyword"),
		ViewerID: viewerID, Admin: admin, Limit: size, Offset: (page - 1) * size,
	})
	if err != nil {
		writeAPIError(c, http.StatusInternalServerError, "problemset.list_failed", "failed to list problem sets")
		return
	}
	c.JSON(http.StatusOK, httpx.ListResponse[dto.SetResponse]{
		Items: dto.FromSets(items, viewerID, admin), Total: total,
	})
}

// Get returns one set with its ordered problems and the viewer's progress.
//
//	@Summary	Get a problem set
//	@Tags		problem-sets
//	@Produce	json
//	@Param		id	path		string	true	"Problem set ID"
//	@Success	200	{object}	dto.SetResponse
//	@Failure	404	{object}	httpx.ErrorResponse
//	@Router		/api/problem-sets/{id} [get]
func (h *SetHandler) Get(c *gin.Context) {
	viewerID := middleware.CurrentUserID(c)
	admin := middleware.CurrentRole(c) == "admin"
	item, err := h.service.Get(c.Request.Context(), c.Param("id"), viewerID, admin)
	if err != nil {
		h.writeError(c, err, "failed to load the problem set")
		return
	}
	c.JSON(http.StatusOK, dto.FromSet(*item, item.CanEdit(viewerID, admin), true))
}

// Create opens a new set owned by the caller. Any signed-in user may curate.
//
//	@Summary	Create a problem set
//	@Tags		problem-sets
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		request		body		dto.SetUpsertRequest	true	"Problem set"
//	@Success	201			{object}	dto.SetResponse
//	@Failure	400,401,413	{object}	httpx.ErrorResponse
//	@Router		/api/problem-sets [post]
func (h *SetHandler) Create(c *gin.Context) {
	var request dto.SetUpsertRequest
	if !httpx.BindJSON(c, &request, maxSetBody, "title is required") {
		return
	}
	created, err := h.service.Create(c.Request.Context(), middleware.CurrentUserID(c), request.Input())
	if err != nil {
		h.writeError(c, err, "failed to create the problem set")
		return
	}
	c.JSON(http.StatusCreated, dto.FromSet(*created, true, true))
}

// Update replaces the editable fields of a set the caller owns.
//
//	@Summary	Update a problem set
//	@Tags		problem-sets
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id					path		string					true	"Problem set ID"
//	@Param		request				body		dto.SetUpsertRequest	true	"Problem set"
//	@Success	200					{object}	dto.SetResponse
//	@Failure	400,401,403,404,413	{object}	httpx.ErrorResponse
//	@Router		/api/problem-sets/{id} [put]
func (h *SetHandler) Update(c *gin.Context) {
	var request dto.SetUpsertRequest
	if !httpx.BindJSON(c, &request, maxSetBody, "title is required") {
		return
	}
	updated, err := h.service.Update(c.Request.Context(), c.Param("id"),
		middleware.CurrentUserID(c), middleware.CurrentRole(c) == "admin", request.Input())
	if err != nil {
		h.writeError(c, err, "failed to update the problem set")
		return
	}
	c.JSON(http.StatusOK, dto.FromSet(*updated, true, true))
}

// Delete removes a set. The problems it pointed at are untouched.
//
//	@Summary	Delete a problem set
//	@Tags		problem-sets
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Problem set ID"
//	@Success	200			{object}	httpx.StatusResponse
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/problem-sets/{id} [delete]
func (h *SetHandler) Delete(c *gin.Context) {
	err := h.service.Delete(c.Request.Context(), c.Param("id"),
		middleware.CurrentUserID(c), middleware.CurrentRole(c) == "admin")
	if err != nil {
		h.writeError(c, err, "failed to delete the problem set")
		return
	}
	c.JSON(http.StatusOK, httpx.StatusResponse{Status: "deleted"})
}

// SetItems replaces the curated problem list in the given order.
//
//	@Summary	Replace problem set contents
//	@Tags		problem-sets
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id					path		string				true	"Problem set ID"
//	@Param		request				body		dto.SetItemsRequest	true	"Ordered problems"
//	@Success	200					{object}	dto.SetResponse
//	@Failure	400,401,403,404,413	{object}	httpx.ErrorResponse
//	@Router		/api/problem-sets/{id}/items [put]
func (h *SetHandler) SetItems(c *gin.Context) {
	var request dto.SetItemsRequest
	if !httpx.BindJSON(c, &request, maxSetBody, "items are required") {
		return
	}
	viewerID := middleware.CurrentUserID(c)
	admin := middleware.CurrentRole(c) == "admin"
	if err := h.service.SetItems(c.Request.Context(), c.Param("id"), viewerID, admin, request.Input()); err != nil {
		h.writeError(c, err, "failed to update the problem set contents")
		return
	}
	updated, err := h.service.Get(c.Request.Context(), c.Param("id"), viewerID, admin)
	if err != nil {
		h.writeError(c, err, "failed to reload the problem set")
		return
	}
	c.JSON(http.StatusOK, dto.FromSet(*updated, true, true))
}

func (h *SetHandler) writeError(c *gin.Context, err error, fallback string) {
	var validation *problemset.ValidationError
	switch {
	case errors.As(err, &validation):
		writeAPIError(c, http.StatusBadRequest, "request.invalid", validation.Message)
	case errors.Is(err, problemset.ErrNotFound):
		writeAPIError(c, http.StatusNotFound, "problemset.not_found", "problem set not found")
	case errors.Is(err, problemset.ErrForbidden):
		writeAPIError(c, http.StatusForbidden, "problemset.forbidden", "only the curator may change this set")
	default:
		writeAPIError(c, http.StatusInternalServerError, "problemset.failed", fallback)
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
