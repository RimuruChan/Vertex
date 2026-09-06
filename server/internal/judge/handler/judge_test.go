package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/judge"
	judgedto "github.com/RimuruChan/Vertex/server/internal/judge/dto"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("JudgeHandler", func() {
	BeforeEach(func() { gin.SetMode(gin.TestMode) })

	It("returns the immutable claim snapshot", func() {
		service := &fakeJudgeService{job: &judge.Job{
			ID: "job-1", SubmissionID: "submission-1", Generation: 2, Attempt: 3,
			LeaseToken: "lease-1", LeaseExpiresAt: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
			Language: "cpp", SourceCode: "int main(){}", ProblemID: "problem-1",
			TimeLimitMs: 1000, MemoryLimitKB: 262144,
			Testdata: judge.Testdata{
				StoragePath: "problem-data", DataVersion: 7, SHA256: "abc123", CaseCount: 3, Checker: "diff",
			},
		}}
		handler := NewJudgeHandler(service)
		router := gin.New()
		router.POST("/jobs/claim", handler.Claim)
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/jobs/claim", bytes.NewBufferString(`{"workerId":"worker-1","waitSeconds":25}`))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusOK))
		var body judgedto.JobResponse
		Expect(json.Unmarshal(response.Body.Bytes(), &body)).To(Succeed())
		Expect(body.Generation).To(Equal(2))
		Expect(body.Testdata.DataVersion).To(Equal(7))
		Expect(body.Testdata.SHA256).To(Equal("abc123"))
		Expect(service.claimWorker).To(Equal("worker-1"))
		Expect(service.claimWait).To(Equal(25 * time.Second))
	})

	It("maps stale result fencing to conflict", func() {
		service := &fakeJudgeService{completeErr: judge.ErrStaleLease}
		handler := NewJudgeHandler(service)
		router := gin.New()
		router.PUT("/jobs/:jobId/result", handler.Complete)
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, "/jobs/job-1/result", bytes.NewBufferString(`{
			"workerId":"worker-1","submissionId":"submission-1","generation":2,
			"leaseToken":"lease-1","status":"Accepted","score":100
		}`))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusConflict))
		Expect(response.Body.String()).To(MatchJSON(`{"code":"judge.stale_lease","error":"stale judge lease"}`))
		Expect(service.result.JobID).To(Equal("job-1"))
		Expect(service.result.Generation).To(Equal(2))
	})

	It("maps invalid worker identities to a bad request", func() {
		service := &fakeJudgeService{claimErr: judge.ErrInvalidWorker}
		handler := NewJudgeHandler(service)
		router := gin.New()
		router.POST("/jobs/claim", handler.Claim)
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/jobs/claim", bytes.NewBufferString(`{"workerId":"worker-1"}`))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusBadRequest))
		Expect(response.Body.String()).To(MatchJSON(`{"code":"judge.invalid_request","error":"invalid worker ID"}`))
	})

	It("rejects chunked oversized internal requests", func() {
		channels := []struct {
			name    string
			method  string
			route   string
			path    string
			limit   int
			handler gin.HandlerFunc
		}{
			{name: "claim", method: http.MethodPost, route: "/jobs/claim", path: "/jobs/claim", limit: maxJudgeControlBody, handler: NewJudgeHandler(&fakeJudgeService{}).Claim},
			{name: "heartbeat", method: http.MethodPost, route: "/jobs/:jobId/heartbeat", path: "/jobs/job-1/heartbeat", limit: maxJudgeControlBody, handler: NewJudgeHandler(&fakeJudgeService{}).Heartbeat},
			{name: "result", method: http.MethodPut, route: "/jobs/:jobId/result", path: "/jobs/job-1/result", limit: maxJudgeResultBody, handler: NewJudgeHandler(&fakeJudgeService{}).Complete},
		}

		for _, channel := range channels {
			By(channel.name)
			router := gin.New()
			router.Handle(channel.method, channel.route, channel.handler)
			body := `{"workerId":"worker-1","padding":"` + strings.Repeat("x", channel.limit) + `"}`
			request := httptest.NewRequest(channel.method, channel.path, bytes.NewBufferString(body))
			request.ContentLength = -1
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			Expect(response.Code).To(Equal(http.StatusRequestEntityTooLarge))
			Expect(response.Body.String()).To(MatchJSON(
				`{"code":"request.too_large","error":"request body is too large"}`,
			))
		}
	})
})

type fakeJudgeService struct {
	job               *judge.Job
	claimErr          error
	claimWorker       string
	claimWait         time.Duration
	result            judge.Result
	completeErr       error
	heartbeatError    error
	heartbeatProgress int
}

func (f *fakeJudgeService) Claim(_ context.Context, workerID string, wait time.Duration) (*judge.Job, error) {
	f.claimWorker, f.claimWait = workerID, wait
	return f.job, f.claimErr
}

func (f *fakeJudgeService) Heartbeat(_ context.Context, _ string, _ int, _, _ string, judgedCases int) error {
	f.heartbeatProgress = judgedCases
	return f.heartbeatError
}

func (f *fakeJudgeService) Complete(_ context.Context, result judge.Result) error {
	f.result = result
	return f.completeErr
}
