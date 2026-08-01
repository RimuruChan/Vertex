package api

import (
	"strconv"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vertex-oj/web/internal/store"
)

// DiscussionHandler 评论接口。
type DiscussionHandler struct {
	discussions *store.DiscussionStore
}

func NewDiscussionHandler(d *store.DiscussionStore) *DiscussionHandler {
	return &DiscussionHandler{discussions: d}
}

// ListByProblem 处理 GET /api/problems/:id/discussions。
func (h *DiscussionHandler) ListByProblem(c *gin.Context) {
	list, err := h.discussions.ListByProblem(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list discussions"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": list, "total": len(list)})
}

// CreateProblemPost 处理 POST /api/problems/:id/discussions(需登录)。
func (h *DiscussionHandler) CreateProblemPost(c *gin.Context) {
	var req struct {
		ContentMD string `json:"contentMd" binding:"required"`
		ParentID  *int64 `json:"parentId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "contentMd required"})
		return
	}
	post, err := h.discussions.CreateProblemPost(c.Request.Context(), c.Param("id"), currentUserID(c), req.ContentMD, req.ParentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create post"})
		return
	}
	c.JSON(http.StatusCreated, post)
}

// ListByEditorial 处理 GET /api/editorials/:id/discussions。
func (h *DiscussionHandler) ListByEditorial(c *gin.Context) {
	list, err := h.discussions.ListByEditorial(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list discussions"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": list, "total": len(list)})
}

// CreateEditorialPost 处理 POST /api/editorials/:id/discussions(需登录)。
func (h *DiscussionHandler) CreateEditorialPost(c *gin.Context) {
	var req struct {
		ContentMD string `json:"contentMd" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "contentMd required"})
		return
	}
	post, err := h.discussions.CreateEditorialPost(c.Request.Context(), c.Param("id"), currentUserID(c), req.ContentMD)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create post"})
		return
	}
	c.JSON(http.StatusCreated, post)
}

// Delete 处理 DELETE /api/discussions/:id(作者或 admin)。
func (h *DiscussionHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid post id"})
		return
	}
	if currentRole(c) != "admin" {
		owner, err := h.discussions.IsPostOwner(c.Request.Context(), id, currentUserID(c))
		if err != nil || !owner {
			c.JSON(http.StatusForbidden, gin.H{"error": "not allowed"})
			return
		}
	}
	if err := h.discussions.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
