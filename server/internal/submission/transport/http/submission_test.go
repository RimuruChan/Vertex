package handler

import (
	"bytes"
	"context"
	"encoding/json"
	identityapp "github.com/RimuruChan/Vertex/server/internal/identity/application"
	identitydomain "github.com/RimuruChan/Vertex/server/internal/identity/domain"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	submissiondomain "github.com/RimuruChan/Vertex/server/internal/submission/domain"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var _ = Describe("Submission request body limit", func() {
	BeforeEach(func() { gin.SetMode(gin.TestMode) })

	It("rejects an oversized submission before reaching the service", func() {
		handler := &SubmissionHandler{}
		router := gin.New()
		router.POST("/submissions", handler.Submit)
		body := `{"problemId":"p1","language":"cpp","sourceCode":"` +
			strings.Repeat("x", maxSubmissionBody) + `"}`
		request := httptest.NewRequest(http.MethodPost, "/submissions", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()

		router.ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusRequestEntityTooLarge))
		Expect(response.Body.String()).To(MatchJSON(
			`{"code":"request.too_large","error":"request body is too large"}`,
		))
	})
})

var _ = Describe("Submission progress handler", func() {
	BeforeEach(func() {
		gin.SetMode(gin.TestMode)
	})

	It("returns only the polling DTO and forwards the viewer identity", func() {
		contestID := "contest-1"
		service := &fakeProgressService{item: &submissiondomain.SubmissionProgress{
			ID: "submission-1", UserID: "user-1", ContestID: &contestID,
			Status: submissiondomain.StatusJudging, TotalTimeMs: 12, PeakMemoryKb: 1024,
			JudgedCases: 1, TotalCases: 3,
			CaseResults: []submissiondomain.CaseResult{{CaseIndex: 1, Verdict: submissiondomain.StatusAccepted}},
		}}
		recorder := requestProgress(service)

		Expect(recorder.Code).To(Equal(http.StatusOK))
		var body map[string]any
		Expect(json.Unmarshal(recorder.Body.Bytes(), &body)).To(Succeed())
		Expect(body).To(HaveKeyWithValue("id", "submission-1"))
		Expect(body).To(HaveKeyWithValue("status", submissiondomain.StatusJudging))
		Expect(body).To(HaveKeyWithValue("judgedCases", BeNumerically("==", 1)))
		Expect(body).NotTo(HaveKey("userId"))
		Expect(body).NotTo(HaveKey("contestId"))
		Expect(body).NotTo(HaveKey("sourceCode"))
		Expect(body).NotTo(HaveKey("problemTitle"))
		Expect(service.id).To(Equal("submission-1"))
		Expect(service.userID).To(Equal("user-1"))
		Expect(service.role).To(Equal("user"))
	})

	It("maps an inaccessible submission to not found", func() {
		recorder := requestProgress(&fakeProgressService{err: submissiondomain.ErrNotFound})
		Expect(recorder.Code).To(Equal(http.StatusNotFound))
		Expect(recorder.Body.String()).To(MatchJSON(
			`{"code":"resource.not_found","error":"resource not found"}`,
		))
	})
})

func requestProgress(service *fakeProgressService) *httptest.ResponseRecorder {
	handler := &SubmissionHandler{progress: service}
	auth := middleware.NewAuthMiddleware(progressAuthenticator{})
	router := gin.New()
	router.GET("/api/submissions/:id/progress", auth.Require(), handler.Progress)

	request := httptest.NewRequest(http.MethodGet, "/api/submissions/submission-1/progress", nil)
	request.Header.Set("Authorization", "Bearer test-access")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

type fakeProgressService struct {
	item   *submissiondomain.SubmissionProgress
	err    error
	id     string
	userID string
	role   string
}

func (f *fakeProgressService) Progress(
	_ context.Context, id, userID, role string,
) (*submissiondomain.SubmissionProgress, error) {
	f.id, f.userID, f.role = id, userID, role
	return f.item, f.err
}

type progressAuthenticator struct{}

func (progressAuthenticator) Authenticate(context.Context, string) (*identityapp.Identity, error) {
	return &identityapp.Identity{User: &identitydomain.User{ID: "user-1", Role: "user"}}, nil
}

func TestSubmissionHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Submission Handler Suite")
}
