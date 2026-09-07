package domain_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/content"
	contenthandler "github.com/RimuruChan/Vertex/server/internal/content/handler"
	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/identity"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/RimuruChan/Vertex/server/internal/problem"
	"github.com/RimuruChan/Vertex/server/internal/publicid"
	"github.com/RimuruChan/Vertex/server/internal/transport/httpapi"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Content HTTP domain boundaries", func() {
	It("uses parent and domain capabilities instead of stale global role claims", func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		users := map[string]string{}
		for _, name := range []string{"manager", "author", "reader"} {
			u, err := identity.NewUserStore(integrationDB).Create(ctx, name, name+"@example.test", "fixture")
			Expect(err).NotTo(HaveOccurred())
			users[name] = u.ID
		}
		spaces := domain.NewService(domain.NewStore(integrationDB))
		scope, err := spaces.Create(ctx, users["manager"], domain.CreateInput{Slug: "team", Name: "Team"})
		Expect(err).NotTo(HaveOccurred())
		for _, name := range []string{"author", "reader"} {
			Expect(spaces.SetMember(ctx, "team", users["manager"], domain.MemberInput{Username: name, RoleKey: "member", Status: "active"})).To(Succeed())
		}
		as := func(name string) context.Context {
			return domain.WithScope(ctx, domain.Scope{Domain: scope.Domain, UserID: users[name]})
		}
		writer := problem.NewProblemAdminStore(integrationDB, GinkgoT().TempDir())
		task, err := writer.Create(as("manager"), users["manager"], &problem.CreateInput{Title: "HTTP parent", Visibility: "public"})
		Expect(err).NotTo(HaveOccurred())
		editorials := content.NewEditorialStore(integrationDB)
		item, err := editorials.Create(as("author"), users["author"], content.EditorialInput{ProblemID: task.ID, Title: "HTTP solution", ContentMD: "protected solution"})
		Expect(err).NotTo(HaveOccurred())
		service := content.NewService(editorials, content.NewDiscussionStore(integrationDB), content.NewAccessStore(integrationDB))
		auth := middleware.NewAuthMiddleware(staleRoleAuthenticator{users: users})
		router := gin.New()
		contenthandler.RegisterRoutes(router.Group("/api/domains/:domain"), contenthandler.NewEditorialHandler(service), contenthandler.NewDiscussionHandler(service), auth.Optional(), auth.Require(), middleware.ResolveDomain(spaces), httpapi.PublicIDs(publicid.NewStore(integrationDB)))
		request := func(method, path, actor, body string) *httptest.ResponseRecorder {
			r := httptest.NewRequest(method, "/api/domains/"+path, strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			if actor != "" {
				r.Header.Set("Authorization", "Bearer "+actor)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, r)
			return response
		}
		path := "team/editorials/" + item.PublicID
		response := request("GET", path, "manager", "")
		Expect(response.Code).To(Equal(200))
		Expect(response.Body.String()).To(ContainSubstring(`"edit":false`))
		Expect(response.Body.String()).To(ContainSubstring(`"delete":true`))
		Expect(request("PUT", path, "manager", `{"title":"Tampered","contentMd":"no"}`).Code).To(Equal(403))
		Expect(request("DELETE", path, "reader", "").Code).To(Equal(403))
		response = request("GET", "team/editorials", "reader", "")
		Expect(response.Code).To(Equal(200))
		Expect(response.Body.String()).NotTo(ContainSubstring("contentMd"))
		Expect(request("GET", "official/editorials/"+item.ID, "manager", "").Code).To(Equal(404))
		response = request("POST", "team/problems/"+task.PublicID+"/discussions", "author", `{"contentMd":"comment","domainId":"`+domain.OfficialID+`","authorId":"`+users["reader"]+`"}`)
		Expect(response.Code).To(Equal(201))
		var post struct {
			ID       int64  `json:"id"`
			DomainID string `json:"domainId"`
			AuthorID string `json:"authorId"`
		}
		Expect(json.Unmarshal(response.Body.Bytes(), &post)).To(Succeed())
		Expect(post.DomainID).To(Equal(scope.Domain.ID))
		Expect(post.AuthorID).To(Equal(users["author"]))
		Expect(spaces.SetMember(ctx, "team", users["manager"], domain.MemberInput{Username: "reader", RoleKey: "viewer", Status: "active"})).To(Succeed())
		response = request("GET", "team/problems/"+task.PublicID+"/discussions", "reader", "")
		Expect(response.Code).To(Equal(200))
		Expect(response.Body.String()).To(ContainSubstring(`"canPost":false`))
		Expect(request("POST", "team/problems/"+task.PublicID+"/discussions", "reader", `{"contentMd":"not allowed"}`).Code).To(Equal(403))
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE problems SET visibility='private' WHERE id=$1", task.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(request("GET", path, "author", "").Code).To(Equal(404))
		Expect(request("PUT", path, "author", `{"title":"Stale author","contentMd":"no"}`).Code).To(Equal(404))
		Expect(writer.SetGrant(as("manager"), task.ID, problem.GrantInput{Username: "author", Role: problem.AccessReader})).To(Succeed())
		Expect(request("PUT", path, "author", `{"title":"Restored","contentMd":"yes"}`).Code).To(Equal(200))
		for _, method := range []string{"GET", "POST"} {
			Expect(request(method, "team/contests/1/discussions", "manager", `{"contentMd":"use clarifications"}`).Code).To(Equal(404))
		}
	})
})
