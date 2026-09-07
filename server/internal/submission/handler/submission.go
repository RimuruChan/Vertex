package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/contest"
	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/RimuruChan/Vertex/server/internal/problem"
	"github.com/RimuruChan/Vertex/server/internal/submission"
	"github.com/RimuruChan/Vertex/server/internal/submission/dto"
	"github.com/gin-gonic/gin"
)

const maxSubmissionBody = submission.MaxSourceBytes + (8 << 10)

type ProgressService interface {
	Progress(ctx context.Context, id, userID, role string) (*submission.SubmissionProgress, error)
}

type SubmissionHandler struct {
	service  *submission.Service
	progress ProgressService
}

func NewSubmissionHandler(service *submission.Service) *SubmissionHandler {
	return &SubmissionHandler{service: service, progress: service}
}

// Submit creates a queued judge job together with the submission.
//
//	@Summary	Submit source code
//	@Tags		submissions
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		request					body		dto.SubmissionCreateRequest	true	"Submission"
//	@Success	202						{object}	dto.SubmissionResponse
//	@Failure	400,401,403,404,413,429	{object}	httpx.ErrorResponse
//	@Router		/api/submissions [post]
func (h *SubmissionHandler) Submit(c *gin.Context) {
	var request dto.SubmissionCreateRequest
	if !httpx.BindJSON(c, &request, maxSubmissionBody, "problemId, language and sourceCode are required") {
		return
	}
	created, err := h.service.Submit(c.Request.Context(), middleware.CurrentUserID(c), middleware.CurrentRole(c), request.CreateInput())
	if err != nil {
		h.writeError(c, err, "failed to create submission")
		return
	}
	c.JSON(http.StatusAccepted, dto.FromSubmission(*created, true))
}

// List returns submissions matching optional filters.
//
//	@Summary	List submissions
//	@Tags		submissions
//	@Produce	json
//	@Security	BearerAuth
//	@Param		user		query		string	false	"User ID or username"
//	@Param		problem		query		string	false	"Problem ID"
//	@Param		contest		query		string	false	"Contest ID"
//	@Param		language	query		string	false	"Language"
//	@Param		status		query		string	false	"Verdict"
//	@Param		page		query		int		false	"Page"
//	@Param		size		query		int		false	"Page size"
//	@Success	200			{object}	httpx.ListResponse[dto.SubmissionResponse]
//	@Failure	401			{object}	httpx.ErrorResponse
//	@Router		/api/submissions [get]
func (h *SubmissionHandler) List(c *gin.Context) {
	page, size := pagination(c)
	userID := c.Query("user")
	if userID == "" {
		userID = c.Query("username")
	}
	items, total, err := h.service.List(c.Request.Context(), submission.Filters{
		UserID: userID, ProblemID: c.Query("problem"), ContestID: c.Query("contest"),
		Language: c.Query("language"), Status: c.Query("status"),
		Limit: size, Offset: (page - 1) * size,
	}, middleware.CurrentUserID(c), middleware.CurrentRole(c))
	if err != nil {
		writeAPIError(c, http.StatusInternalServerError, "submission.list_failed", "failed to list submissions")
		return
	}
	c.JSON(http.StatusOK, httpx.ListResponse[dto.SubmissionResponse]{Items: dto.FromSubmissions(items), Total: total})
}

// Get returns one submission; source is visible only to its owner or an admin.
//
//	@Summary	Get submission
//	@Tags		submissions
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string	true	"Submission ID"
//	@Success	200		{object}	dto.SubmissionResponse
//	@Failure	401,404	{object}	httpx.ErrorResponse
//	@Router		/api/submissions/{id} [get]
func (h *SubmissionHandler) Get(c *gin.Context) {
	item, includeSource, err := h.service.Get(
		c.Request.Context(), c.Param("id"), middleware.CurrentUserID(c), middleware.CurrentRole(c),
	)
	if err != nil {
		h.writeError(c, err, "failed to load submission")
		return
	}
	c.JSON(http.StatusOK, dto.FromSubmission(*item, includeSource))
}

// Progress returns the lightweight view used while judging. It follows the
// same visibility rules as submission detail and never includes source.
//
//	@Summary	Get submission judging progress
//	@Tags		submissions
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Submission ID"
//	@Success	200			{object}	dto.SubmissionProgressResponse
//	@Failure	401,404,500	{object}	httpx.ErrorResponse
//	@Router		/api/submissions/{id}/progress [get]
func (h *SubmissionHandler) Progress(c *gin.Context) {
	item, err := h.progress.Progress(
		c.Request.Context(), c.Param("id"), middleware.CurrentUserID(c), middleware.CurrentRole(c),
	)
	if err != nil {
		h.writeError(c, err, "failed to load submission progress")
		return
	}
	c.JSON(http.StatusOK, dto.FromProgress(*item))
}

// Rejudge creates a new fenced generation for a submission.
//
//	@Summary	Rejudge submission
//	@Tags		admin
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Submission ID"
//	@Success	200			{object}	httpx.StatusResponse
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/admin/submissions/{id}/rejudge [post]
func (h *SubmissionHandler) Rejudge(c *gin.Context) {
	if err := h.service.Rejudge(c.Request.Context(), c.Param("id")); err != nil {
		h.writeError(c, err, "failed to schedule rejudge")
		return
	}
	c.JSON(http.StatusOK, httpx.StatusResponse{Status: "rejudge scheduled"})
}

func (h *SubmissionHandler) writeError(c *gin.Context, err error, fallback string) {
	var validation *submission.ValidationError
	switch {
	case errors.Is(err, domain.ErrUnauthenticated):
		writeAPIError(c, http.StatusUnauthorized, "auth.invalid_token", "authentication required")
	case errors.Is(err, domain.ErrForbidden), errors.Is(err, contest.ErrForbidden):
		writeAPIError(c, http.StatusForbidden, "submission.forbidden", "insufficient resource permissions")
	case errors.As(err, &validation):
		writeAPIError(c, http.StatusBadRequest, "request.invalid", validation.Message)
	case errors.Is(err, submission.ErrUnsupportedLanguage):
		writeAPIError(c, http.StatusBadRequest, "submission.unsupported_language", err.Error())
	case errors.Is(err, submission.ErrSourceTooLarge):
		writeAPIError(c, http.StatusBadRequest, "submission.source_too_large", err.Error())
	case errors.Is(err, submission.ErrRateLimited):
		writeAPIError(c, http.StatusTooManyRequests, "submission.rate_limited", "submission rate limit exceeded, slow down")
	case errors.Is(err, submission.ErrProblemForbidden):
		writeAPIError(c, http.StatusForbidden, "problem.forbidden", err.Error())
	case errors.Is(err, submission.ErrProblemUnpublished):
		writeAPIError(c, http.StatusConflict, "problem.not_published", err.Error())
	case submission.IsContestRuleError(err):
		writeAPIError(c, http.StatusBadRequest, "contest.submission_rejected", err.Error())
	case errors.Is(err, submission.ErrNotFound), errors.Is(err, domain.ErrNotFound), errors.Is(err, contest.ErrNotFound), errors.Is(err, problem.ErrNotFound):
		writeAPIError(c, http.StatusNotFound, "resource.not_found", "resource not found")
	case errors.Is(err, submission.ErrContestUnavailable):
		writeAPIError(c, http.StatusServiceUnavailable, "contest.unavailable", "contest service unavailable")
	default:
		writeAPIError(c, http.StatusInternalServerError, "submission.operation_failed", fallback)
	}
}

func parseIntDefault(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func pagination(c *gin.Context) (int, int) {
	page := parseIntDefault(c.Query("page"), 1)
	size := parseIntDefault(c.Query("size"), 20)
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return page, size
}

func writeAPIError(c *gin.Context, status int, code, message string) {
	httpx.WriteError(c, status, code, message)
}
