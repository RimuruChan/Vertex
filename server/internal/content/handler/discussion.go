package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/content"
	"github.com/RimuruChan/Vertex/server/internal/content/dto"
	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/gin-gonic/gin"
)

const maxDiscussionBody = 32 << 10

// DiscussionHandler exposes discussion DTOs while ownership stays in the Content service.
type DiscussionHandler struct {
	service *content.Service
}

func NewDiscussionHandler(service *content.Service) *DiscussionHandler {
	return &DiscussionHandler{service: service}
}

// ListByProblem returns the thread attached to a problem.
//
//	@Summary	List problem discussions
//	@Tags		discussions
//	@Produce	json
//	@Param		id	path		string	true	"Problem ID"
//	@Success	200	{object}	httpx.ListResponse[dto.DiscussionResponse]
//	@Router		/api/problems/{id}/discussions [get]
func (h *DiscussionHandler) ListByProblem(c *gin.Context) {
	list, err := h.service.ListProblemPosts(
		c.Request.Context(), c.Param("id"), middleware.CurrentUserID(c), middleware.CurrentRole(c) == "admin",
	)
	if err != nil {
		writeContentError(c, err, "failed to list discussions")
		return
	}
	c.JSON(http.StatusOK, httpx.ListResponse[dto.DiscussionResponse]{Items: dto.FromDiscussions(list), Total: len(list)})
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
//	@Router		/api/problems/{id}/discussions [post]
func (h *DiscussionHandler) CreateProblemPost(c *gin.Context) {
	var request dto.ProblemDiscussionCreateRequest
	if !httpx.BindJSON(c, &request, maxDiscussionBody, "contentMd required") {
		return
	}
	post, err := h.service.CreateProblemPost(
		c.Request.Context(), c.Param("id"), middleware.CurrentUserID(c),
		middleware.CurrentRole(c) == "admin", request.ContentMD, request.ParentID,
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
//	@Param		id	path		string	true	"Editorial ID"
//	@Success	200	{object}	httpx.ListResponse[dto.DiscussionResponse]
//	@Router		/api/editorials/{id}/discussions [get]
func (h *DiscussionHandler) ListByEditorial(c *gin.Context) {
	list, err := h.service.ListEditorialPosts(
		c.Request.Context(), c.Param("id"), middleware.CurrentUserID(c), middleware.CurrentRole(c) == "admin",
	)
	if err != nil {
		writeContentError(c, err, "failed to list discussions")
		return
	}
	c.JSON(http.StatusOK, httpx.ListResponse[dto.DiscussionResponse]{Items: dto.FromDiscussions(list), Total: len(list)})
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
//	@Router		/api/editorials/{id}/discussions [post]
func (h *DiscussionHandler) CreateEditorialPost(c *gin.Context) {
	var request dto.EditorialDiscussionCreateRequest
	if !httpx.BindJSON(c, &request, maxDiscussionBody, "contentMd required") {
		return
	}
	post, err := h.service.CreateEditorialPost(
		c.Request.Context(), c.Param("id"), middleware.CurrentUserID(c),
		middleware.CurrentRole(c) == "admin", request.ContentMD,
	)
	if err != nil {
		writeContentError(c, err, "failed to create post")
		return
	}
	c.JSON(http.StatusCreated, dto.FromDiscussion(*post))
}

// Delete relies on the Content service for author-or-admin authorization.
//
//	@Summary	Delete discussion
//	@Tags		discussions
//	@Produce	json
//	@Security	BearerAuth
//	@Param		postId		path		int	true	"Discussion ID"
//	@Success	200			{object}	httpx.StatusResponse
//	@Failure	400,401,403	{object}	httpx.ErrorResponse
//	@Router		/api/discussions/{postId} [delete]
func (h *DiscussionHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("postId"), 10, 64)
	if err != nil {
		httpx.WriteError(c, http.StatusBadRequest, "request.invalid", "invalid post id")
		return
	}
	if err := h.service.DeletePost(c.Request.Context(), id, middleware.CurrentUserID(c), middleware.CurrentRole(c) == "admin"); err != nil {
		if errors.Is(err, content.ErrForbidden) {
			httpx.WriteError(c, http.StatusForbidden, "discussion.forbidden", "not allowed")
			return
		}
		httpx.WriteError(c, http.StatusInternalServerError, "discussion.delete_failed", "failed to delete")
		return
	}
	c.JSON(http.StatusOK, httpx.StatusResponse{Status: "deleted"})
}

// ListByContest returns the thread attached to a contest.
//
//	@Summary	List contest discussions
//	@Tags		discussions
//	@Produce	json
//	@Param		id	path		string	true	"Contest ID"
//	@Success	200	{object}	httpx.ListResponse[dto.DiscussionResponse]
//	@Router		/api/contests/{id}/discussions [get]
func (h *DiscussionHandler) ListByContest(c *gin.Context) {
	list, err := h.service.ListContestPosts(
		c.Request.Context(), c.Param("id"), middleware.CurrentUserID(c), middleware.CurrentRole(c) == "admin",
	)
	if err != nil {
		writeContentError(c, err, "failed to list discussions")
		return
	}
	c.JSON(http.StatusOK, httpx.ListResponse[dto.DiscussionResponse]{Items: dto.FromDiscussions(list), Total: len(list)})
}

// CreateContestPost adds a comment to a contest's public thread.
//
//	@Summary	Create contest discussion
//	@Tags		discussions
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string								true	"Contest ID"
//	@Param		request		body		dto.ProblemDiscussionCreateRequest	true	"Discussion"
//	@Success	201			{object}	dto.DiscussionResponse
//	@Failure	400,401,413	{object}	httpx.ErrorResponse
//	@Router		/api/contests/{id}/discussions [post]
func (h *DiscussionHandler) CreateContestPost(c *gin.Context) {
	var request dto.ProblemDiscussionCreateRequest
	if !httpx.BindJSON(c, &request, maxDiscussionBody, "contentMd required") {
		return
	}
	post, err := h.service.CreateContestPost(c.Request.Context(), c.Param("id"),
		middleware.CurrentUserID(c), middleware.CurrentRole(c) == "admin", request.ContentMD, request.ParentID)
	if err != nil {
		writeContentError(c, err, "failed to create post")
		return
	}
	c.JSON(http.StatusCreated, dto.FromDiscussion(*post))
}

// Update rewrites one's own comment. Moderation removes posts instead of
// editing them, so this is author-only even for administrators.
//
//	@Summary	Edit a discussion post
//	@Tags		discussions
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		postId				path		int										true	"Discussion ID"
//	@Param		request				body		dto.EditorialDiscussionCreateRequest	true	"New content"
//	@Success	200					{object}	dto.DiscussionResponse
//	@Failure	400,401,403,404,413	{object}	httpx.ErrorResponse
//	@Router		/api/discussions/{postId} [put]
func (h *DiscussionHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("postId"), 10, 64)
	if err != nil {
		httpx.WriteError(c, http.StatusBadRequest, "request.invalid", "invalid post id")
		return
	}
	var request dto.EditorialDiscussionCreateRequest
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
