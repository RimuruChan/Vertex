package httpapi_test

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/RimuruChan/Vertex/server/docs"
	contenthandler "github.com/RimuruChan/Vertex/server/internal/content/handler"
	contesthandler "github.com/RimuruChan/Vertex/server/internal/contest/handler"
	domainhandler "github.com/RimuruChan/Vertex/server/internal/domain/handler"
	identityhandler "github.com/RimuruChan/Vertex/server/internal/identity/handler"
	judgehandler "github.com/RimuruChan/Vertex/server/internal/judge/handler"
	problemhandler "github.com/RimuruChan/Vertex/server/internal/problem/handler"
	submissionhandler "github.com/RimuruChan/Vertex/server/internal/submission/handler"
	httpapi "github.com/RimuruChan/Vertex/server/internal/transport/httpapi"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var pathParameter = regexp.MustCompile(`:([A-Za-z][A-Za-z0-9_]*)`)

var _ = Describe("Generated OpenAPI", func() {
	It("covers every business route and both authentication schemes", func() {
		gin.SetMode(gin.TestMode)
		pass := func(c *gin.Context) { c.Next() }
		router := httpapi.Router(httpapi.Dependencies{
			Domains: &domainhandler.Handler{}, ResolveDomain: pass,
			Auth: &identityhandler.AuthHandler{}, Health: &httpapi.HealthHandler{},
			Submissions: &submissionhandler.SubmissionHandler{}, Problems: &problemhandler.ProblemHandler{},
			Contests: &contesthandler.ContestHandler{}, Editorials: &contenthandler.EditorialHandler{},
			Discussions: &contenthandler.DiscussionHandler{}, AdminProblems: &problemhandler.AdminProblemHandler{},
			Judge: &judgehandler.JudgeHandler{}, RequireAuth: pass, OptionalAuth: pass,
			RequireAdmin: pass, RequireJudge: pass,
		})

		var spec struct {
			Paths               map[string]map[string]json.RawMessage `json:"paths"`
			SecurityDefinitions map[string]json.RawMessage            `json:"securityDefinitions"`
		}
		raw := docs.SwaggerInfo.ReadDoc()
		Expect(json.Unmarshal([]byte(raw), &spec)).To(Succeed())
		Expect(spec.SecurityDefinitions).To(HaveKey("BearerAuth"))
		Expect(spec.SecurityDefinitions).To(HaveKey("JudgeServiceAuth"))

		documented := make(map[string]struct{})
		for path, operations := range spec.Paths {
			for method := range operations {
				documented[strings.ToUpper(method)+" "+path] = struct{}{}
			}
		}
		for _, route := range router.Routes() {
			if !strings.HasPrefix(route.Path, "/api/") && !strings.HasPrefix(route.Path, "/internal/judge/") {
				continue
			}
			path := pathParameter.ReplaceAllString(route.Path, `{$1}`)
			Expect(documented).To(HaveKey(route.Method+" "+path), "missing generated contract for %s %s", route.Method, route.Path)
		}
	})

	It("does not expose persistence credential fields", func() {
		raw := docs.SwaggerInfo.ReadDoc()
		Expect(raw).NotTo(ContainSubstring("password_hash"))
		Expect(raw).NotTo(ContainSubstring("refresh_token_hash"))
		Expect(raw).NotTo(ContainSubstring("passwordHash"))
		Expect(raw).NotTo(ContainSubstring("refreshTokenHash"))
	})
})
