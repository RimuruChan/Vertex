package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/RimuruChan/Vertex/server/internal/submission"
	"github.com/RimuruChan/Vertex/server/internal/submission/dto"
	"github.com/gin-gonic/gin"
)

const maxRejudgingBody = 1 << 20

// CreateRejudging queues a batch re-judge from a selector.
//
//	@Summary	Create a rejudging batch
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		request			body		dto.RejudgingCreateRequest	true	"Selector"
//	@Success	202				{object}	dto.RejudgingResponse
//	@Failure	400,401,403,413	{object}	httpx.ErrorResponse
//	@Router		/api/admin/rejudgings [post]
func (h *SubmissionHandler) CreateRejudging(c *gin.Context) {
	var request dto.RejudgingCreateRequest
	if !httpx.BindJSON(c, &request, maxRejudgingBody, "invalid rejudge selector") {
		return
	}
	batch, err := h.service.CreateRejudging(
		c.Request.Context(), request.Selector(), middleware.CurrentUserID(c))
	if err != nil {
		h.writeRejudgeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, dto.FromRejudging(*batch))
}

// ListRejudgings returns recent batches, newest first.
//
//	@Summary	List rejudging batches
//	@Tags		admin
//	@Produce	json
//	@Security	BearerAuth
//	@Param		contest	query		string	false	"Contest ID"
//	@Param		limit	query		int		false	"Maximum batches"
//	@Success	200		{object}	httpx.ListResponse[dto.RejudgingResponse]
//	@Failure	401,403	{object}	httpx.ErrorResponse
//	@Router		/api/admin/rejudgings [get]
func (h *SubmissionHandler) ListRejudgings(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	items, err := h.service.ListRejudgings(c.Request.Context(), c.Query("contest"), limit)
	if err != nil {
		writeAPIError(c, http.StatusInternalServerError, "rejudging.list_failed", "failed to list rejudgings")
		return
	}
	responses := dto.FromRejudgings(items)
	c.JSON(http.StatusOK, httpx.ListResponse[dto.RejudgingResponse]{
		Items: responses, Total: len(responses),
	})
}

// GetRejudging returns one batch with live progress.
//
//	@Summary	Get a rejudging batch
//	@Tags		admin
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Rejudging ID"
//	@Success	200			{object}	dto.RejudgingResponse
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/admin/rejudgings/{id} [get]
func (h *SubmissionHandler) GetRejudging(c *gin.Context) {
	batch, err := h.service.Rejudging(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeRejudgeError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.FromRejudging(*batch))
}

// RejudgingChanges lists the submissions whose verdict moved.
//
//	@Summary	List verdict changes of a rejudging
//	@Tags		admin
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Rejudging ID"
//	@Param		limit		query		int		false	"Maximum rows"
//	@Success	200			{object}	httpx.ListResponse[dto.RejudgingChangeResponse]
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/admin/rejudgings/{id}/changes [get]
func (h *SubmissionHandler) RejudgingChanges(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	changes, err := h.service.RejudgingChanges(c.Request.Context(), c.Param("id"), limit)
	if err != nil {
		h.writeRejudgeError(c, err)
		return
	}
	responses := dto.FromRejudgingChanges(changes)
	c.JSON(http.StatusOK, httpx.ListResponse[dto.RejudgingChangeResponse]{
		Items: responses, Total: len(responses),
	})
}

// CancelRejudging withdraws queued work and restores its prior results.
//
//	@Summary	Cancel a rejudging batch
//	@Tags		admin
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id				path		string	true	"Rejudging ID"
//	@Success	200				{object}	httpx.StatusResponse
//	@Failure	400,401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/admin/rejudgings/{id}/cancel [post]
func (h *SubmissionHandler) CancelRejudging(c *gin.Context) {
	if err := h.service.CancelRejudging(c.Request.Context(), c.Param("id")); err != nil {
		h.writeRejudgeError(c, err)
		return
	}
	c.JSON(http.StatusOK, httpx.StatusResponse{Status: "cancelled"})
}

func (h *SubmissionHandler) writeRejudgeError(c *gin.Context, err error) {
	var validation *submission.ValidationError
	switch {
	case errors.As(err, &validation):
		writeAPIError(c, http.StatusBadRequest, "request.invalid", validation.Message)
	case errors.Is(err, submission.ErrRejudgeEmpty):
		writeAPIError(c, http.StatusBadRequest, "rejudging.empty",
			"the selector matched no submissions; a rejudge must be restricted")
	case errors.Is(err, submission.ErrRejudgeNotFound):
		writeAPIError(c, http.StatusNotFound, "rejudging.not_found", "rejudging not found")
	case errors.Is(err, submission.ErrRejudgeClosed):
		writeAPIError(c, http.StatusBadRequest, "rejudging.closed", "rejudging is no longer running")
	default:
		writeAPIError(c, http.StatusInternalServerError, "rejudging.failed", "rejudge request failed")
	}
}
