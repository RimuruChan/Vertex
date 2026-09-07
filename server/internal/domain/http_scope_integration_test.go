package domain_test

import (
	"net/http/httptest"

	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/identity"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/RimuruChan/Vertex/server/internal/problem"
	problemhandler "github.com/RimuruChan/Vertex/server/internal/problem/handler"
	"github.com/RimuruChan/Vertex/server/internal/publicid"
	"github.com/RimuruChan/Vertex/server/internal/transport/httpapi"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("HTTP resource scope", func() {
	It("binds the path domain before number resolution and rechecks suspended membership", func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		users := map[string]string{}
		for _, name := range []string{"owner", "member", "outsider"} {
			user, err := identity.NewUserStore(integrationDB).Create(ctx, name, name+"@example.test", "fixture-hash")
			Expect(err).NotTo(HaveOccurred())
			users[name] = user.ID
		}
		service := domain.NewService(domain.NewStore(integrationDB))
		_, err := integrationDB.Pool.ExecContext(ctx, "UPDATE domain_members SET role_key='author' WHERE user_id=$1 AND domain_id=$2", users["owner"], domain.OfficialID)
		Expect(err).NotTo(HaveOccurred())
		scope, err := service.Create(ctx, users["owner"], domain.CreateInput{Slug: "training", Name: "Training"})
		Expect(err).NotTo(HaveOccurred())
		Expect(service.SetMember(ctx, "training", users["owner"], domain.MemberInput{Username: "member", RoleKey: "member", Status: "active"})).To(Succeed())
		writer := problem.NewProblemAdminStore(integrationDB, GinkgoT().TempDir())
		local, err := writer.Create(ctx, users["owner"], &problem.CreateInput{Title: "Official exercise", Visibility: "public"})
		Expect(err).NotTo(HaveOccurred())
		foreign, err := writer.Create(domain.WithScope(ctx, scope), users["owner"], &problem.CreateInput{Title: "Training exercise", Visibility: "public"})
		Expect(err).NotTo(HaveOccurred())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, local.ID, foreign.ID)).To(Succeed())
		handler := problemhandler.NewProblemHandler(problem.NewService(problem.NewProblemStore(integrationDB), writer))
		auth := middleware.NewAuthMiddleware(staleRoleAuthenticator{users: users})
		router := gin.New()
		// The resource registrations are deliberately identical; only their
		// parent group supplies the domain parameter.
		for _, prefix := range []string{"/api", "/api/domains/:domain"} {
			group := router.Group(prefix, auth.Optional(), middleware.ResolveDomain(service), httpapi.PublicIDs(publicid.NewStore(integrationDB)))
			group.GET("/problems/:id", handler.Get)
			group.GET("/tags", handler.Tags)
		}
		request := func(path, token string) *httptest.ResponseRecorder {
			r := httptest.NewRequest("GET", path, nil)
			if token != "" {
				r.Header.Set("Authorization", "Bearer "+token)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, r)
			return response
		}
		response := request("/api/problems/"+local.PublicID+"?domain=training", "owner")
		Expect(response.Code).To(Equal(200))
		Expect(response.Body.String()).To(ContainSubstring("Official exercise"))
		response = request("/api/domains/training/problems/"+foreign.PublicID, "member")
		Expect(response.Code).To(Equal(200))
		Expect(response.Body.String()).To(ContainSubstring("Training exercise"))
		for _, path := range []string{
			"/api/problems/" + foreign.ID,
			"/api/domains/official/problems/" + foreign.ID,
			"/api/domains/training/problems/" + local.ID,
		} {
			Expect(request(path, "owner").Code).To(Equal(404), path)
		}
		for _, token := range []string{"", "outsider"} {
			Expect(request("/api/domains/training/problems/"+foreign.PublicID, token).Code).To(Equal(404))
			Expect(request("/api/domains/training/tags", token).Code).To(Equal(404))
		}
		Expect(service.SetMember(ctx, "training", users["owner"], domain.MemberInput{Username: "member", RoleKey: "member", Status: "suspended"})).To(Succeed())
		Expect(request("/api/domains/training/problems/"+foreign.PublicID, "member").Code).To(Equal(404))
		// The path also determines the scope for non-resource-number endpoints.
		Expect(request("/api/domains/training/tags", "member").Code).To(Equal(404))
	})
})
