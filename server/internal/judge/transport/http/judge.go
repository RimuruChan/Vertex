package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/httpx"
	judgeapp "github.com/RimuruChan/Vertex/server/internal/judge/application"
	judgedomain "github.com/RimuruChan/Vertex/server/internal/judge/domain"
	dto "github.com/RimuruChan/Vertex/server/internal/judge/transport/http/dto"
	"github.com/gin-gonic/gin"
)

const (
	maxJudgeControlBody = 64 << 10
	maxJudgeResultBody  = 16 << 20
)

type JudgeService interface {
	Claim(ctx context.Context, workerID string, wait time.Duration) (*judgedomain.Job, error)
	Heartbeat(ctx context.Context, jobID string, generation int, leaseToken, workerID string, judgedCases int) error
	Complete(ctx context.Context, result judgedomain.Result) error
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
//	@Param		request	body		dto.ClaimRequest	true	"Worker and long-poll settings"
//	@Success	200		{object}	dto.JobResponse
//	@Success	204
//	@Failure	400,401,413,503	{object}	httpx.ErrorResponse
//	@Router		/internal/judge/v1/jobs/claim [post]
func (h *JudgeHandler) Claim(c *gin.Context) {
	var request dto.ClaimRequest
	if !httpx.BindJSON(c, &request, maxJudgeControlBody, "workerId is required") {
		return
	}
	job, err := h.service.Claim(c.Request.Context(), request.WorkerID, time.Duration(request.WaitSeconds)*time.Second)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		if errors.Is(err, judgeapp.ErrInvalidWorker) {
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
	c.JSON(http.StatusOK, dto.JobFromDomain(job))
}

// Heartbeat extends a live fenced lease.
//
//	@Summary	Renew a judge lease
//	@Tags		judge-internal
//	@Accept		json
//	@Security	JudgeServiceAuth
//	@Param		jobId	path	string				true	"Job ID"
//	@Param		request	body	dto.LeaseRequest	true	"Lease identity"
//	@Success	204
//	@Failure	400,401,409,413	{object}	httpx.ErrorResponse
//	@Router		/internal/judge/v1/jobs/{jobId}/heartbeat [post]
func (h *JudgeHandler) Heartbeat(c *gin.Context) {
	var request dto.LeaseRequest
	if !httpx.BindJSON(c, &request, maxJudgeControlBody, "workerId, generation and leaseToken are required") {
		return
	}
	if err := h.service.Heartbeat(
		c.Request.Context(), c.Param("jobId"), request.Generation,
		request.LeaseToken, request.WorkerID, request.JudgedCases,
	); err != nil {
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
//	@Param		jobId	path	string				true	"Job ID"
//	@Param		request	body	dto.ResultRequest	true	"Judge result"
//	@Success	204
//	@Failure	400,401,409,413	{object}	httpx.ErrorResponse
//	@Router		/internal/judge/v1/jobs/{jobId}/result [put]
func (h *JudgeHandler) Complete(c *gin.Context) {
	var request dto.ResultRequest
	if !httpx.BindJSON(c, &request, maxJudgeResultBody, "invalid judge result") {
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
	if errors.Is(err, judgedomain.ErrInvalidResult) {
		httpx.WriteError(c, http.StatusBadRequest, "judge.invalid_result", err.Error())
		return
	}
	if errors.Is(err, judgedomain.ErrStaleLease) {
		httpx.WriteError(c, http.StatusConflict, "judge.stale_lease", judgedomain.ErrStaleLease.Error())
		return
	}
	httpx.WriteError(c, http.StatusInternalServerError, "judge.persistence_failed", "judge result persistence failed")
}
