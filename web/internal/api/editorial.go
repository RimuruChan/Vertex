package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vertex-oj/web/internal/store"
)

// EditorialHandler 题解接口。
type EditorialHandler struct {
	editorials *store.EditorialStore
}

func NewEditorialHandler(e *store.EditorialStore) *EditorialHandler {
	return &EditorialHandler{editorials: e}
}

// ListByProblem 处理 GET /api/editorials?problem=:id。
func (h *EditorialHandler) ListByProblem(c *gin.Context) {
	problemID := c.Query("problem")
	if problemID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "problem query param required"})
		return
	}
	list, err := h.editorials.ListByProblem(c.Request.Context(), problemID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list editorials"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": list, "total": len(list)})
}

// Get 处理 GET /api/editorials/:id。
func (h *EditorialHandler) Get(c *gin.Context) {
	e, err := h.editorials.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "editorial not found"})
		return
	}
	c.JSON(http.StatusOK, e)
}

// Create 处理 POST /api/editorials(需登录)。
func (h *EditorialHandler) Create(c *gin.Context) {
	var req struct {
		ProblemID string `json:"problemId" binding:"required"`
		Title     string `json:"title" binding:"required"`
		ContentMD string `json:"contentMd" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "problemId, title and contentMd required"})
		return
	}
	e, err := h.editorials.Create(c.Request.Context(), req.ProblemID, currentUserID(c), req.Title, req.ContentMD)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create editorial"})
		return
	}
	c.JSON(http.StatusCreated, e)
}
