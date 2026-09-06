package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/RimuruChan/Vertex/server/internal/content"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Editorial list handler", func() {
	BeforeEach(func() {
		gin.SetMode(gin.TestMode)
	})

	It("returns summary DTOs without editorial bodies", func() {
		service := &fakeEditorialListService{
			items: []content.EditorialSummary{{
				ID: "editorial-1", ProblemID: "problem-1", ProblemTitle: "A + B",
				Title: "线性做法", Visibility: content.VisibilityPublic,
				Status: content.StatusPublished, SolvedOnly: true,
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
	items   []content.EditorialSummary
	total   int
	filters content.EditorialFilters
}

func (f *fakeEditorialListService) ListEditorials(
	_ context.Context, filters content.EditorialFilters,
) ([]content.EditorialSummary, int, error) {
	f.filters = filters
	return f.items, f.total, nil
}
