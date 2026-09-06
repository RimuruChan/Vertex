package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/RimuruChan/Vertex/server/internal/identity"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/RimuruChan/Vertex/server/internal/submission"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Submission progress handler", func() {
	BeforeEach(func() {
		gin.SetMode(gin.TestMode)
	})

	It("returns only the polling DTO and forwards the viewer identity", func() {
		contestID := "contest-1"
		service := &fakeProgressService{item: &submission.SubmissionProgress{
			ID: "submission-1", UserID: "user-1", ContestID: &contestID,
			Status: submission.StatusJudging, TotalTimeMs: 12, PeakMemoryKb: 1024,
			JudgedCases: 1, TotalCases: 3,
			CaseResults: []submission.CaseResult{{CaseIndex: 1, Verdict: submission.StatusAccepted}},
		}}
		recorder := requestProgress(service)

		Expect(recorder.Code).To(Equal(http.StatusOK))
		var body map[string]any
		Expect(json.Unmarshal(recorder.Body.Bytes(), &body)).To(Succeed())
		Expect(body).To(HaveKeyWithValue("id", "submission-1"))
		Expect(body).To(HaveKeyWithValue("status", submission.StatusJudging))
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
		recorder := requestProgress(&fakeProgressService{err: submission.ErrNotFound})
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
	item   *submission.SubmissionProgress
	err    error
	id     string
	userID string
	role   string
}

func (f *fakeProgressService) Progress(
	_ context.Context, id, userID, role string,
) (*submission.SubmissionProgress, error) {
	f.id, f.userID, f.role = id, userID, role
	return f.item, f.err
}

type progressAuthenticator struct{}

func (progressAuthenticator) Authenticate(context.Context, string) (*identity.Identity, error) {
	return &identity.Identity{User: &identity.User{ID: "user-1", Role: "user"}}, nil
}
