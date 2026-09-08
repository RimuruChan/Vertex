package httpapi_test

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/RimuruChan/Vertex/server/docs"
	contenthttp "github.com/RimuruChan/Vertex/server/internal/content/transport/http"
	contesthttp "github.com/RimuruChan/Vertex/server/internal/contest/transport/http"
	identityhttp "github.com/RimuruChan/Vertex/server/internal/identity/transport/http"
	judgehttp "github.com/RimuruChan/Vertex/server/internal/judge/transport/http"
	problemhttp "github.com/RimuruChan/Vertex/server/internal/problem/transport/http"
	submissionhandler "github.com/RimuruChan/Vertex/server/internal/submission/transport/http"
	tenancyhttp "github.com/RimuruChan/Vertex/server/internal/tenancy/transport/http"
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
			Domains: &tenancyhttp.Handler{}, ResolveDomain: pass,
			Auth: &identityhttp.AuthHandler{}, Health: &httpapi.HealthHandler{},
			Submissions: &submissionhandler.SubmissionHandler{}, Problems: &problemhttp.ProblemHandler{},
			Contests: &contesthttp.ContestHandler{}, Editorials: &contenthttp.EditorialHandler{},
			Discussions: &contenthttp.DiscussionHandler{}, AdminProblems: &problemhttp.AdminProblemHandler{},
			Judge: &judgehttp.JudgeHandler{}, RequireAuth: pass, OptionalAuth: pass,
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

	It("declares exactly the path parameters belonging to each scoped or legacy route", func() {
		var spec struct {
			Paths map[string]map[string]struct {
				Parameters []struct {
					Name     string `json:"name"`
					In       string `json:"in"`
					Required bool   `json:"required"`
				} `json:"parameters"`
			} `json:"paths"`
		}
		Expect(json.Unmarshal([]byte(docs.SwaggerInfo.ReadDoc()), &spec)).To(Succeed())
		parameter := regexp.MustCompile(`\{([^}]+)\}`)
		for path, operations := range spec.Paths {
			for _, operation := range operations {
				expected := []string{}
				for _, match := range parameter.FindAllStringSubmatch(path, -1) {
					expected = append(expected, match[1])
				}
				actual := []string{}
				for _, param := range operation.Parameters {
					if param.In == "path" {
						actual = append(actual, param.Name)
						Expect(param.Required).To(BeTrue(), path)
					}
				}
				Expect(actual).To(ConsistOf(expected), path)
			}
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
