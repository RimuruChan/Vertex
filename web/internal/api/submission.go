package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/vertex-oj/web/internal/model"
	"github.com/vertex-oj/web/internal/store"
)

// SubmissionHandler 处理提交相关接口。
type SubmissionHandler struct {
	submissions *store.SubmissionStore
	problems    *store.ProblemStore
	notify      func(subID string) // 入队后唤醒 judge worker(Redis 信号或直接通知)
	rateLimit   *rateLimiter       // 每用户提交限流
}

func NewSubmissionHandler(subs *store.SubmissionStore, problems *store.ProblemStore, notify func(string)) *SubmissionHandler {
	return &SubmissionHandler{
		submissions: subs,
		problems:    problems,
		notify:      notify,
		rateLimit:   newRateLimiter(),
	}
}

// 判题支持的语言(扩展:加编译/运行指令,判题 worker 侧对应实现)。
var supportedLanguages = map[string]bool{
	"cpp":  true,
	"c":    true,
	"python": true,
}

type submitReq struct {
	ProblemID  string `json:"problemId" binding:"required"`
	Language   string `json:"language" binding:"required"`
	SourceCode string `json:"sourceCode" binding:"required"`
}

// Submit 处理 POST /api/submissions。
func (h *SubmissionHandler) Submit(c *gin.Context) {
	var req submitReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "problemId, language and sourceCode are required"})
		return
	}
	if !supportedLanguages[req.Language] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported language: " + req.Language})
		return
	}
	if len(req.SourceCode) > 256*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source code too large"})
		return
	}

	// 提交限流:每用户 10 次/分钟(防刷爆判题队列)
	if !h.rateLimit.Allow(currentUserID(c)) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "submission rate limit exceeded, slow down"})
		return
	}

	// 校验题目存在且可见(非 draft / private;admin 可见私有,但不处理 draft 提交)
	p, err := h.problems.Get(c.Request.Context(), req.ProblemID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "problem not found"})
		return
	}
	if p.Visibility != "public" && currentRole(c) != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "problem not accessible"})
		return
	}

	sub := &model.Submission{
		UserID:     currentUserID(c),
		ProblemID:  req.ProblemID,
		Language:   req.Language,
		SourceCode: req.SourceCode,
	}
	created, err := h.submissions.Create(c.Request.Context(), sub)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create submission"})
		return
	}

	// 唤醒 judge worker(非阻塞;Postgres 行是权威,信号丢失由 worker 轮询兜底)
	if h.notify != nil {
		h.notify(created.ID)
	}

	c.JSON(http.StatusAccepted, created)
}

// List 处理 GET /api/submissions?user=&problem=&status=&language=&page=&size=。
func (h *SubmissionHandler) List(c *gin.Context) {
	userID := c.Query("user")
	if userID == "" {
		userID = c.Query("username") // 兼容前端按用户名查
	}
	f := store.SubmissionFilters{
		UserID:    userID,
		ProblemID: c.Query("problem"),
		ContestID: c.Query("contest"),
		Language:  c.Query("language"),
		Status:    c.Query("status"),
		Limit:     parseIntDefault(c.Query("size"), 20),
		Offset:    parseIntDefault(c.Query("page"), 1) - 1,
	}
	if f.Offset < 0 {
		f.Offset = 0
	}

	list, total, err := h.submissions.List(c.Request.Context(), f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list submissions"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": list, "total": total})
}

// Get 处理 GET /api/submissions/:id。
func (h *SubmissionHandler) Get(c *gin.Context) {
	sub, err := h.submissions.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "submission not found"})
		return
	}
	// 非本人、非 admin 不暴露源码
	if sub.UserID != currentUserID(c) && currentRole(c) != "admin" {
		sub.SourceCode = ""
	}
	c.JSON(http.StatusOK, sub)
}

// Rejudge 处理 POST /api/submissions/:id/rejudge(仅 admin)。
func (h *SubmissionHandler) Rejudge(c *gin.Context) {
	id := c.Param("id")
	if err := h.submissions.Rejudge(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to schedule rejudge"})
		return
	}
	if h.notify != nil {
		h.notify(id)
	}
	c.JSON(http.StatusOK, gin.H{"status": "rejudge scheduled"})
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
