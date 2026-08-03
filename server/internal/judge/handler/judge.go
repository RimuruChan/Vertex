package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/RimuruChan/Vertex/server/internal/judge"
	judgedto "github.com/RimuruChan/Vertex/server/internal/judge/dto"
	"github.com/gin-gonic/gin"
)

const (
	maxJudgeControlBody = 64 << 10
	maxJudgeResultBody  = 16 << 20
)

type JudgeService interface {
	Claim(ctx context.Context, workerID string, wait time.Duration) (*judge.Job, error)
	Heartbeat(ctx context.Context, jobID string, generation int, leaseToken, workerID string) error
	Complete(ctx context.Context, result judge.Result) error
}

type JudgeHandler struct{ service JudgeService }

func NewJudgeHandler(service JudgeService) *JudgeHandler { return &JudgeHandler{service: service} }

// Claim long-polls for one atomically leased judge job.
//
//	@Summary	Claim a judge job
//	@Tags		judge-internal
//	@Accept		json
//	@Produce	json
//	@Security	JudgeServiceAuth
//	@Param		request	body		judgedto.ClaimRequest	true	"Worker and long-poll settings"
//	@Success	200		{object}	judgedto.JobResponse
//	@Success	204
//	@Failure	400,401,503	{object}	httpx.ErrorResponse
//	@Router		/internal/judge/v1/jobs/claim [post]
func (h *JudgeHandler) Claim(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxJudgeControlBody)
	var request judgedto.ClaimRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.WriteError(c, http.StatusBadRequest, "judge.invalid_request", "workerId is required")
		return
	}
	job, err := h.service.Claim(c.Request.Context(), request.WorkerID, time.Duration(request.WaitSeconds)*time.Second)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		if errors.Is(err, judge.ErrInvalidWorker) {
			httpx.WriteError(c, http.StatusBadRequest, "judge.invalid_request", err.Error())
			return
		}
		httpx.WriteError(c, http.StatusServiceUnavailable, "judge.claim_failed", "unable to claim a judge job")
		return
	}
	if job == nil {
		c.Status(http.StatusNoContent)
		return
	}
	c.JSON(http.StatusOK, judgedto.JobFromDomain(job))
}

// Heartbeat extends a live fenced lease.
//
//	@Summary	Renew a judge lease
//	@Tags		judge-internal
//	@Accept		json
//	@Security	JudgeServiceAuth
//	@Param		jobId	path	string					true	"Job ID"
//	@Param		request	body	judgedto.LeaseRequest	true	"Lease identity"
//	@Success	204
//	@Failure	400,401,409	{object}	httpx.ErrorResponse
//	@Router		/internal/judge/v1/jobs/{jobId}/heartbeat [post]
func (h *JudgeHandler) Heartbeat(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxJudgeControlBody)
	var request judgedto.LeaseRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.WriteError(c, http.StatusBadRequest, "judge.invalid_request", "workerId, generation and leaseToken are required")
		return
	}
	if err := h.service.Heartbeat(c.Request.Context(), c.Param("jobId"), request.Generation, request.LeaseToken, request.WorkerID); err != nil {
		h.writeJudgeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Complete persists a fenced, idempotent judge result.
//
//	@Summary	Complete a judge job
//	@Tags		judge-internal
//	@Accept		json
//	@Security	JudgeServiceAuth
//	@Param		jobId	path	string					true	"Job ID"
//	@Param		request	body	judgedto.ResultRequest	true	"Judge result"
//	@Success	204
//	@Failure	400,401,409	{object}	httpx.ErrorResponse
//	@Router		/internal/judge/v1/jobs/{jobId}/result [put]
func (h *JudgeHandler) Complete(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxJudgeResultBody)
	var request judgedto.ResultRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.WriteError(c, http.StatusBadRequest, "judge.invalid_request", "invalid judge result")
		return
	}
	result := request.Domain(c.Param("jobId"))
	if err := h.service.Complete(c.Request.Context(), result); err != nil {
		h.writeJudgeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *JudgeHandler) writeJudgeError(c *gin.Context, err error) {
	if errors.Is(err, judge.ErrInvalidResult) {
		httpx.WriteError(c, http.StatusBadRequest, "judge.invalid_result", err.Error())
		return
	}
	if errors.Is(err, judge.ErrStaleLease) {
		httpx.WriteError(c, http.StatusConflict, "judge.stale_lease", judge.ErrStaleLease.Error())
		return
	}
	httpx.WriteError(c, http.StatusInternalServerError, "judge.persistence_failed", "judge result persistence failed")
}
