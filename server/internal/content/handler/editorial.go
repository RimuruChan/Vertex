package handler

import (
	"errors"
	"net/http"

	"github.com/RimuruChan/Vertex/server/internal/content"
	"github.com/RimuruChan/Vertex/server/internal/content/dto"
	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/gin-gonic/gin"
)

// EditorialHandler maps Content service results to public DTOs.
type EditorialHandler struct {
	service *content.Service
}

func NewEditorialHandler(service *content.Service) *EditorialHandler {
	return &EditorialHandler{service: service}
}

// ListByProblem requires an explicit problem filter.
//
//	@Summary	List editorials for a problem
//	@Tags		editorials
//	@Produce	json
//	@Param		problem	query		string	true	"Problem ID"
//	@Success	200		{object}	httpx.ListResponse[dto.EditorialResponse]
//	@Failure	400		{object}	httpx.ErrorResponse
//	@Router		/api/editorials [get]
func (h *EditorialHandler) ListByProblem(c *gin.Context) {
	problemID := c.Query("problem")
	if problemID == "" {
		httpx.WriteError(c, http.StatusBadRequest, "request.invalid", "problem query param required")
		return
	}
	list, err := h.service.ListEditorials(c.Request.Context(), problemID)
	if err != nil {
		httpx.WriteError(c, http.StatusInternalServerError, "editorial.list_failed", "failed to list editorials")
		return
	}
	c.JSON(http.StatusOK, httpx.ListResponse[dto.EditorialResponse]{Items: dto.FromEditorials(list), Total: len(list)})
}

// Get returns one published editorial view.
//
//	@Summary	Get editorial
//	@Tags		editorials
//	@Produce	json
//	@Param		id	path		string	true	"Editorial ID"
//	@Success	200	{object}	dto.EditorialResponse
//	@Failure	404	{object}	httpx.ErrorResponse
//	@Router		/api/editorials/{id} [get]
func (h *EditorialHandler) Get(c *gin.Context) {
	e, err := h.service.GetEditorial(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.WriteError(c, http.StatusNotFound, "editorial.not_found", "editorial not found")
		return
	}
	c.JSON(http.StatusOK, dto.FromEditorial(*e))
}

// Create delegates content validation and author ownership to the Content service.
//
//	@Summary	Create editorial
//	@Tags		editorials
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		request	body		dto.EditorialCreateRequest	true	"Editorial"
//	@Success	201		{object}	dto.EditorialResponse
//	@Failure	400,401	{object}	httpx.ErrorResponse
//	@Router		/api/editorials [post]
func (h *EditorialHandler) Create(c *gin.Context) {
	var request dto.EditorialCreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.WriteError(c, http.StatusBadRequest, "request.invalid", "problemId, title and contentMd required")
		return
	}
	e, err := h.service.CreateEditorial(c.Request.Context(), request.ProblemID, middleware.CurrentUserID(c), request.Title, request.ContentMD)
	if err != nil {
		if errors.Is(err, content.ErrInvalidInput) {
			httpx.WriteError(c, http.StatusBadRequest, "request.invalid", err.Error())
			return
		}
		httpx.WriteError(c, http.StatusInternalServerError, "editorial.create_failed", "failed to create editorial")
		return
	}
	c.JSON(http.StatusCreated, dto.FromEditorial(*e))
}
