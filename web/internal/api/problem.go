package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vertex-oj/web/internal/store"
)

// ProblemHandler 公开题目接口。
type ProblemHandler struct {
	view *store.ProblemStore
}

func NewProblemHandler(view *store.ProblemStore) *ProblemHandler {
	return &ProblemHandler{view: view}
}

// List 处理 GET /api/problems?difficulty=&tag=&keyword=&page=&size=。
// 只返回 public 题目。
func (h *ProblemHandler) List(c *gin.Context) {
	f := store.ProblemFilters{
		Visibility: "public",
		Tag:        c.Query("tag"),
		Difficulty: parseIntDefault(c.Query("difficulty"), 0),
		Keyword:    c.Query("keyword"),
		Limit:      parseIntDefault(c.Query("size"), 20),
		Offset:     parseIntDefault(c.Query("page"), 1) - 1,
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	list, total, err := h.view.List(c.Request.Context(), f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list problems"})
		return
	}
	// 公开列表隐藏题面内容,只留元信息
	for i := range list {
		list[i].StatementMD = ""
	}
	c.JSON(http.StatusOK, gin.H{"items": list, "total": total})
}

// Get 处理 GET /api/problems/:id。
// public 题目可公开访问;admin 可访问 private/draft。
func (h *ProblemHandler) Get(c *gin.Context) {
	p, err := h.view.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "problem not found"})
		return
	}
	if p.Visibility != "public" && currentRole(c) != "admin" {
		c.JSON(http.StatusNotFound, gin.H{"error": "problem not found"})
		return
	}
	c.JSON(http.StatusOK, p)
}
