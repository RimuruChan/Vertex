package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vertex-oj/web/internal/store"
)

// ContestHandler 比赛接口。
type ContestHandler struct {
	contests *store.ContestStore
}

func NewContestHandler(contests *store.ContestStore) *ContestHandler {
	return &ContestHandler{contests: contests}
}

// List 处理 GET /api/contests。
func (h *ContestHandler) List(c *gin.Context) {
	page := parseIntDefault(c.Query("page"), 1)
	size := parseIntDefault(c.Query("size"), 20)
	if page < 1 {
		page = 1
	}
	list, total, err := h.contests.List(c.Request.Context(), size, (page-1)*size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list contests"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": list, "total": total})
}

// Get 处理 GET /api/contests/:id。
func (h *ContestHandler) Get(c *gin.Context) {
	contest, err := h.contests.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "contest not found"})
		return
	}
	problems, err := h.contests.Problems(c.Request.Context(), contest.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load problems"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"contest": contest, "problems": problems})
}

// Create 处理 POST /api/contests(admin)。
func (h *ContestHandler) Create(c *gin.Context) {
	var in store.CreateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if in.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title required"})
		return
	}
	if !in.EndAt.After(in.BeginAt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "end time must be after begin time"})
		return
	}

	contest, err := h.contests.Create(c.Request.Context(), currentUserID(c), &in)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create contest"})
		return
	}
	c.JSON(http.StatusCreated, contest)
}

// SetProblems 处理 PUT /api/contests/:id/problems(admin)。
func (h *ContestHandler) SetProblems(c *gin.Context) {
	var req struct {
		ProblemIDs []string `json:"problemIds"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "problemIds required"})
		return
	}
	if err := h.contests.SetProblems(c.Request.Context(), c.Param("id"), req.ProblemIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set problems"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Register 处理 POST /api/contests/:id/register。
func (h *ContestHandler) Register(c *gin.Context) {
	// 检查比赛是否开始前
	contest, err := h.contests.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "contest not found"})
		return
	}
	if time.Now().After(contest.BeginAt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "contest already started"})
		return
	}
	if err := h.contests.Register(c.Request.Context(), contest.ID, currentUserID(c)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to register"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "registered"})
}

// Rankboard 处理 GET /api/contests/:id/rankboard。
// frozen=true 查询封榜视图;默认根据当前时间自动决定是否封榜。
func (h *ContestHandler) Rankboard(c *gin.Context) {
	contest, err := h.contests.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "contest not found"})
		return
	}

	frozen := false
	if contest.FreezeAt != nil {
		frozen = time.Now().After(*contest.FreezeAt)
	}
	if q := c.Query("frozen"); q == "true" {
		frozen = true
	} else if q == "false" {
		frozen = false
	}

	board, err := h.contests.Rankboard(c.Request.Context(), contest.ID, frozen)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute rankboard"})
		return
	}
	c.JSON(http.StatusOK, board)
}
