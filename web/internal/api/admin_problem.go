package api

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vertex-oj/web/internal/store"
)

// AdminProblemHandler 出题管理后台接口。
type AdminProblemHandler struct {
	admin *store.ProblemAdminStore
	view  *store.ProblemStore
}

func NewAdminProblemHandler(admin *store.ProblemAdminStore, view *store.ProblemStore) *AdminProblemHandler {
	return &AdminProblemHandler{admin: admin, view: view}
}

// List 处理 GET /api/admin/problems:管理员视角题目列表(含草稿/私有)。
func (h *AdminProblemHandler) List(c *gin.Context) {
	f := store.ProblemFilters{
		Visibility: c.Query("visibility"),
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
	c.JSON(http.StatusOK, gin.H{"items": list, "total": total})
}

// Create 处理 POST /api/admin/problems。
func (h *AdminProblemHandler) Create(c *gin.Context) {
	var in store.CreateProblemInput
	if err := c.ShouldBindJSON(&in); err != nil || in.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}
	p, err := h.admin.Create(c.Request.Context(), currentUserID(c), &in)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create problem"})
		return
	}
	c.JSON(http.StatusCreated, p)
}

// Get 处理 GET /api/admin/problems/:id。
func (h *AdminProblemHandler) Get(c *gin.Context) {
	p, err := h.view.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "problem not found"})
		return
	}
	c.JSON(http.StatusOK, p)
}

// Update 处理 PUT /api/admin/problems/:id。
func (h *AdminProblemHandler) Update(c *gin.Context) {
	var in store.UpdateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	p, err := h.admin.Update(c.Request.Context(), c.Param("id"), &in)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "problem not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update problem"})
		return
	}
	c.JSON(http.StatusOK, p)
}

// Delete 处理 DELETE /api/admin/problems/:id。
func (h *AdminProblemHandler) Delete(c *gin.Context) {
	if err := h.admin.Delete(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete problem"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// UploadTestdata 处理 POST /api/admin/problems/:id/testdata(multipart 表单,字段 file)。
func (h *AdminProblemHandler) UploadTestdata(c *gin.Context) {
	// 先确认题目存在
	if _, err := h.view.Get(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "problem not found"})
		return
	}

	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file field required"})
		return
	}
	defer file.Close()

	const maxZip = 64 * 1024 * 1024 // 64MB 上限
	data, err := io.ReadAll(io.LimitReader(file, maxZip+1))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read upload"})
		return
	}
	if len(data) > maxZip {
		c.JSON(http.StatusBadRequest, gin.H{"error": "zip too large (max 64MB)"})
		return
	}

	checker := c.PostForm("checker")
	if checker == "" {
		checker = "diff"
	}

	count, hash, err := h.admin.SaveTestdata(c.Request.Context(), c.Param("id"), data, checker)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"caseCount": count, "sha256": hash, "checker": checker})
}
