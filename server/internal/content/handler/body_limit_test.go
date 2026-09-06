package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Content request body limits", func() {
	BeforeEach(func() { gin.SetMode(gin.TestMode) })

	It("rejects chunked oversized editorial and discussion writes", func() {
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
			{name: "problem discussion create", method: http.MethodPost, route: "/problems/:id/discussions", path: "/problems/p1/discussions", limit: maxDiscussionBody, handler: (&DiscussionHandler{}).CreateProblemPost},
			{name: "editorial discussion create", method: http.MethodPost, route: "/editorials/:id/discussions", path: "/editorials/e1/discussions", limit: maxDiscussionBody, handler: (&DiscussionHandler{}).CreateEditorialPost},
			{name: "contest discussion create", method: http.MethodPost, route: "/contests/:id/discussions", path: "/contests/c1/discussions", limit: maxDiscussionBody, handler: (&DiscussionHandler{}).CreateContestPost},
			{name: "discussion update", method: http.MethodPut, route: "/discussions/:postId", path: "/discussions/1", limit: maxDiscussionBody, handler: (&DiscussionHandler{}).Update},
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

	It("keeps ordinary bind failures as request.invalid", func() {
		router := gin.New()
		router.POST("/problems/p1/discussions", (&DiscussionHandler{}).CreateProblemPost)
		request := httptest.NewRequest(http.MethodPost, "/problems/p1/discussions", bytes.NewBufferString(`{"contentMd":`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()

		router.ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusBadRequest))
		Expect(response.Body.String()).To(MatchJSON(
			`{"code":"request.invalid","error":"contentMd required"}`,
		))
	})
})
