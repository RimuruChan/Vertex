package httpapi

import (
	"net/http"
	"strconv"

	contentapp "github.com/RimuruChan/Vertex/server/internal/content/application"
	dto "github.com/RimuruChan/Vertex/server/internal/content/transport/http/dto"
	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/gin-gonic/gin"
)

const maxDiscussionBody = 32 << 10

// DiscussionHandler exposes discussion DTOs while ownership stays in the Content service.
type DiscussionHandler struct {
	service *contentapp.Service
}

func NewDiscussionHandler(service *contentapp.Service) *DiscussionHandler {
	return &DiscussionHandler{service: service}
}

// ListByProblem returns the thread attached to a problem.
//
//	@Summary	List problem discussions
//	@Tags		discussions
//	@Produce	json
//	@Param		id		path		string	true	"Problem ID"
//	@Success	200		{object}	dto.DiscussionThreadResponse
//	@Param		domain	path		string	true	"Domain slug"
//	@Router		/api/problems/{id}/discussions [get]
//	@Router		/api/domains/{domain}/problems/{id}/discussions [get]
func (h *DiscussionHandler) ListByProblem(c *gin.Context) {
	list, err := h.service.ListProblemPosts(
		c.Request.Context(), c.Param("id"), middleware.CurrentUserID(c),
	)
	if err != nil {
		writeContentError(c, err, "failed to list discussions")
		return
	}
	c.JSON(http.StatusOK, dto.FromThread(list))
}

// CreateProblemPost creates a top-level post or a validated reply.
//
//	@Summary	Create problem discussion
//	@Tags		discussions
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string								true	"Problem ID"
//	@Param		request		body		dto.ProblemDiscussionCreateRequest	true	"Discussion"
//	@Success	201			{object}	dto.DiscussionResponse
//	@Failure	400,401,413	{object}	httpx.ErrorResponse
//	@Param		domain		path		string	true	"Domain slug"
//	@Router		/api/problems/{id}/discussions [post]
//	@Router		/api/domains/{domain}/problems/{id}/discussions [post]
func (h *DiscussionHandler) CreateProblemPost(c *gin.Context) {
	var request dto.ProblemDiscussionCreateRequest
	if !httpx.BindJSON(c, &request, maxDiscussionBody, "contentMd required") {
		return
	}
	post, err := h.service.CreateProblemPost(
		c.Request.Context(), c.Param("id"), middleware.CurrentUserID(c),
		request.ContentMD, request.ParentID,
	)
	if err != nil {
		writeContentError(c, err, "failed to create post")
		return
	}
	c.JSON(http.StatusCreated, dto.FromDiscussion(*post))
}

// ListByEditorial returns the thread attached to an editorial.
//
//	@Summary	List editorial discussions
//	@Tags		discussions
//	@Produce	json
//	@Param		id		path		string	true	"Editorial ID"
//	@Success	200		{object}	dto.DiscussionThreadResponse
//	@Param		domain	path		string	true	"Domain slug"
//	@Router		/api/editorials/{id}/discussions [get]
//	@Router		/api/domains/{domain}/editorials/{id}/discussions [get]
func (h *DiscussionHandler) ListByEditorial(c *gin.Context) {
	list, err := h.service.ListEditorialPosts(
		c.Request.Context(), c.Param("id"), middleware.CurrentUserID(c),
	)
	if err != nil {
		writeContentError(c, err, "failed to list discussions")
		return
	}
	c.JSON(http.StatusOK, dto.FromThread(list))
}

// CreateEditorialPost creates an authenticated editorial comment.
//
//	@Summary	Create editorial discussion
//	@Tags		discussions
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string									true	"Editorial ID"
//	@Param		request		body		dto.EditorialDiscussionCreateRequest	true	"Discussion"
//	@Success	201			{object}	dto.DiscussionResponse
//	@Failure	400,401,413	{object}	httpx.ErrorResponse
//	@Param		domain		path		string	true	"Domain slug"
//	@Router		/api/editorials/{id}/discussions [post]
//	@Router		/api/domains/{domain}/editorials/{id}/discussions [post]
func (h *DiscussionHandler) CreateEditorialPost(c *gin.Context) {
	var request dto.EditorialDiscussionCreateRequest
	if !httpx.BindJSON(c, &request, maxDiscussionBody, "contentMd required") {
		return
	}
	post, err := h.service.CreateEditorialPost(
		c.Request.Context(), c.Param("id"), middleware.CurrentUserID(c),
		request.ContentMD, request.ParentID,
	)
	if err != nil {
		writeContentError(c, err, "failed to create post")
		return
	}
	c.JSON(http.StatusCreated, dto.FromDiscussion(*post))
}

// Delete authorizes removal within the current parent resource and domain.
//
//	@Summary	Delete discussion
//	@Tags		discussions
//	@Produce	json
//	@Security	BearerAuth
//	@Param		postId		path		int	true	"Discussion ID"
//	@Success	200			{object}	httpx.StatusResponse
//	@Failure	400,401,403	{object}	httpx.ErrorResponse
//	@Param		domain		path		string	true	"Domain slug"
//	@Router		/api/discussions/{postId} [delete]
//	@Router		/api/domains/{domain}/discussions/{postId} [delete]
func (h *DiscussionHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("postId"), 10, 64)
	if err != nil {
		httpx.WriteError(c, http.StatusBadRequest, "request.invalid", "invalid post id")
		return
	}
	if err := h.service.DeletePost(c.Request.Context(), id, middleware.CurrentUserID(c)); err != nil {
		writeContentError(c, err, "failed to delete discussion")
		return
	}
	c.JSON(http.StatusOK, httpx.StatusResponse{Status: "deleted"})
}

// Update rewrites one's own comment. Moderation removes posts instead of
// editing them, so this is author-only even for administrators.
//
//	@Summary	Edit a discussion post
//	@Tags		discussions
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		postId				path		int							true	"Discussion ID"
//	@Param		request				body		dto.DiscussionUpdateRequest	true	"New content"
//	@Success	200					{object}	dto.DiscussionResponse
//	@Failure	400,401,403,404,413	{object}	httpx.ErrorResponse
//	@Param		domain				path		string	true	"Domain slug"
//	@Router		/api/discussions/{postId} [put]
//	@Router		/api/domains/{domain}/discussions/{postId} [put]
func (h *DiscussionHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("postId"), 10, 64)
	if err != nil {
		httpx.WriteError(c, http.StatusBadRequest, "request.invalid", "invalid post id")
		return
	}
	var request dto.DiscussionUpdateRequest
	if !httpx.BindJSON(c, &request, maxDiscussionBody, "contentMd required") {
		return
	}
	post, err := h.service.UpdatePost(c.Request.Context(), id, middleware.CurrentUserID(c), request.ContentMD)
	if err != nil {
		writeContentError(c, err, "failed to update the post")
		return
	}
	c.JSON(http.StatusOK, dto.FromDiscussion(*post))
}
