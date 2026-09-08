package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	contentdomain "github.com/RimuruChan/Vertex/server/internal/modules/content/domain"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var _ = Describe("Editorial list handler", func() {
	BeforeEach(func() {
		gin.SetMode(gin.TestMode)
	})

	It("returns summary DTOs without editorial bodies", func() {
		service := &fakeEditorialListService{
			items: []contentdomain.EditorialSummary{{
				ID: "editorial-1", ProblemID: "problem-1", ProblemTitle: "A + B",
				Title: "线性做法", Visibility: contentdomain.VisibilityPublic,
				Status: contentdomain.StatusPublished, SolvedOnly: true,
				Locked: true, Voted: true, VoteCount: 7,
			}},
			total: 21,
		}
		handler := &EditorialHandler{list: service}
		router := gin.New()
		router.GET("/api/editorials", handler.List)
		request := httptest.NewRequest(http.MethodGet, "/api/editorials?page=2&size=10&sort=votes", nil)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, request)

		Expect(recorder.Code).To(Equal(http.StatusOK))
		var body struct {
			Items []map[string]any `json:"items"`
			Total int              `json:"total"`
		}
		Expect(json.Unmarshal(recorder.Body.Bytes(), &body)).To(Succeed())
		Expect(body.Total).To(Equal(21))
		Expect(body.Items).To(HaveLen(1))
		Expect(body.Items[0]).NotTo(HaveKey("contentMd"))
		Expect(body.Items[0]).To(HaveKeyWithValue("locked", true))
		Expect(body.Items[0]).To(HaveKeyWithValue("voted", true))
		Expect(body.Items[0]).To(HaveKeyWithValue("canEdit", false))
		Expect(service.filters.Offset).To(Equal(10))
		Expect(service.filters.Limit).To(Equal(10))
		Expect(service.filters.Sort).To(Equal("votes"))
	})
})

type fakeEditorialListService struct {
	items   []contentdomain.EditorialSummary
	total   int
	filters contentdomain.EditorialFilters
}

func (f *fakeEditorialListService) ListEditorials(
	_ context.Context, filters contentdomain.EditorialFilters,
) ([]contentdomain.EditorialSummary, int, error) {
	f.filters = filters
	return f.items, f.total, nil
}

var _ = Describe("Content request body limits", func() {
	BeforeEach(func() { gin.SetMode(gin.TestMode) })

	It("rejects chunked oversized editorial writes", func() {
		channels := []struct {
			name    string
			method  string
			route   string
			path    string
			limit   int
			handler gin.HandlerFunc
		}{
			{name: "editorial create", method: http.MethodPost, route: "/editorials", path: "/editorials", limit: maxEditorialBody, handler: (&EditorialHandler{}).Create},
			{name: "editorial update", method: http.MethodPut, route: "/editorials/:id", path: "/editorials/e1", limit: maxEditorialBody, handler: (&EditorialHandler{}).Update},
		}

		for _, channel := range channels {
			By(channel.name)
			router := gin.New()
			router.Handle(channel.method, channel.route, channel.handler)
			body := `{"problemId":"p1","title":"title","contentMd":"` +
				strings.Repeat("x", channel.limit) + `"}`
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

func TestContentHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Content Handler Suite")
}
