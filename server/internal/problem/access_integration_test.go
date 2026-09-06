package problem_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/authoring"
	"github.com/RimuruChan/Vertex/server/internal/content"
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

type problemTestAuthenticator map[string]string

func (a problemTestAuthenticator) Authenticate(_ context.Context, token string) (*identity.Identity, error) {
	id, ok := a[token]
	if !ok {
		return nil, errors.New("unknown fixture actor")
	}
	// A stale upstream role must not override current domain/resource policy.
	return &identity.Identity{User: &identity.User{ID: id, Username: token, Role: "admin"}}, nil
}

type unreadUpload struct{ read bool }

func (u *unreadUpload) Read([]byte) (int, error) {
	u.read = true
	return 0, errors.New("unauthorized upload body was read")
}

var _ = Describe("Problem ownership and collaboration against PostgreSQL", func() {
	var domains *domain.Service
	var reader *problem.ProblemStore
	var writer *problem.ProblemAdminStore
	var users map[string]string
	var scope domain.Scope
	var item *problem.Problem
	as := func(ctx context.Context, username string) context.Context {
		return domain.WithScope(ctx, domain.Scope{Domain: scope.Domain, UserID: users[username]})
	}
	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		domains = domain.NewService(domain.NewStore(integrationDB))
		reader = problem.NewProblemStore(integrationDB)
		writer = problem.NewProblemAdminStore(integrationDB, GinkgoT().TempDir())
		users = map[string]string{}
		for _, name := range []string{"manager", "setter", "editor", "reader", "outsider"} {
			user, err := identity.NewUserStore(integrationDB).Create(ctx, name, name+"@example.test", "fixture")
			Expect(err).NotTo(HaveOccurred())
			users[name] = user.ID
		}
		var err error
		scope, err = domains.Create(ctx, users["manager"], domain.CreateInput{Slug: "team", Name: "Team"})
		Expect(err).NotTo(HaveOccurred())
		for _, name := range []string{"setter", "editor", "reader"} {
			role := "member"
			if name == "setter" {
				role = "author"
			}
			Expect(domains.SetMember(ctx, "team", users["manager"], domain.MemberInput{Username: name, RoleKey: role, Status: "active"})).To(Succeed())
		}
		item, err = writer.Create(as(ctx, "setter"), users["setter"], &problem.CreateInput{Title: "Unreleased", Visibility: "draft"})
		Expect(err).NotTo(HaveOccurred())
	})

	It("keeps creation, package editing and owner-only actions distinct", func(ctx SpecContext) {
		_, err := writer.Create(as(ctx, "reader"), users["reader"], &problem.CreateInput{Title: "Unauthorized"})
		Expect(err).To(MatchError(domain.ErrForbidden))
		Expect(writer.SetGrant(as(ctx, "setter"), item.ID, problem.GrantInput{Username: "editor", Role: problem.AccessEditor})).To(Succeed())
		Expect(writer.SetGrant(as(ctx, "setter"), item.ID, problem.GrantInput{Username: "reader", Role: problem.AccessReader})).To(Succeed())
		packages := authoring.NewPackageStore(integrationDB)
		_, err = packages.SaveStatement(as(ctx, "reader"), authoring.Statement{ProblemID: item.ID, Language: "zh", Name: "Denied"})
		Expect(err).To(MatchError(domain.ErrForbidden))
		_, err = packages.SaveStatement(as(ctx, "editor"), authoring.Statement{ProblemID: item.ID, Language: "zh", Name: "Edited"})
		Expect(err).NotTo(HaveOccurred())
		statements, err := packages.Statements(as(ctx, "reader"), item.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(statements).To(HaveLen(1))
		Expect(statements[0].Name).To(Equal("Edited"))
		Expect(writer.Delete(as(ctx, "editor"), item.ID)).To(MatchError(domain.ErrForbidden))
		Expect(writer.Transfer(as(ctx, "editor"), item.ID, "reader")).To(MatchError(domain.ErrForbidden))
		_, err = writer.Update(as(ctx, "editor"), item.ID, &problem.UpdateInput{CreateInput: problem.CreateInput{Title: "Publish", TimeLimitMs: 1000, MemoryLimitKb: 65536, Visibility: "public"}})
		Expect(err).To(MatchError(domain.ErrForbidden))
		Expect(domains.SetMember(ctx, "team", users["manager"], domain.MemberInput{Username: "setter", RoleKey: "viewer", Status: "active"})).To(Succeed())
		access, err := reader.Access(as(ctx, "setter"), item.ID, users["setter"])
		Expect(err).NotTo(HaveOccurred())
		Expect(access.Permissions.Edit && access.Permissions.Publish).To(BeTrue())
		_, err = writer.Create(as(ctx, "setter"), users["setter"], &problem.CreateInput{Title: "No longer a creator"})
		Expect(err).To(MatchError(domain.ErrForbidden))
	})

	It("computes group inheritance dynamically and never converts it into permanent user grants", func(ctx SpecContext) {
		group, err := domains.CreateGroup(ctx, "team", users["manager"], domain.GroupInput{Name: "Reviewers"})
		Expect(err).NotTo(HaveOccurred())
		Expect(domains.SetGroupMember(ctx, "team", users["manager"], group.ID, "reader", "member", false)).To(Succeed())
		Expect(writer.SetGrant(as(ctx, "setter"), item.ID, problem.GrantInput{Group: group.ID, Role: problem.AccessReader})).To(Succeed())
		Expect(writer.SetGrant(as(ctx, "setter"), item.ID, problem.GrantInput{Username: "reader", Role: problem.AccessEditor})).To(Succeed())
		access, err := reader.Access(as(ctx, "reader"), item.ID, users["reader"])
		Expect(err).NotTo(HaveOccurred())
		Expect(access.Role).To(Equal(problem.AccessEditor))
		grants, err := reader.Grants(as(ctx, "setter"), item.ID)
		Expect(err).NotTo(HaveOccurred())
		for _, grant := range grants {
			if grant.UserID != nil {
				Expect(writer.RemoveGrant(as(ctx, "setter"), item.ID, grant.ID)).To(Succeed())
			}
		}
		access, err = reader.Access(as(ctx, "reader"), item.ID, users["reader"])
		Expect(err).NotTo(HaveOccurred())
		Expect(access.Role).To(Equal(problem.AccessReader))
		Expect(access.Permissions.ReadPackage).To(BeTrue())
		Expect(access.Permissions.Edit).To(BeFalse())
		Expect(domains.SetGroupMember(ctx, "team", users["manager"], group.ID, "reader", "member", true)).To(Succeed())
		access, err = reader.Access(as(ctx, "reader"), item.ID, users["reader"])
		Expect(err).NotTo(HaveOccurred())
		Expect(access.Permissions.View || access.Permissions.ReadPackage).To(BeFalse())
		items, total, err := reader.List(as(ctx, "reader"), problem.Filters{Workspace: true, ViewerID: users["reader"]})
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(BeEmpty())
		Expect(total).To(BeZero())
	})

	It("transfers ownership once under concurrency and preserves the original creator", func(ctx SpecContext) {
		results := make(chan error, 2)
		var start sync.WaitGroup
		start.Add(2)
		for _, target := range []string{"reader", "editor"} {
			go func(target string) {
				start.Done()
				start.Wait()
				results <- writer.Transfer(as(ctx, "setter"), item.ID, target)
			}(target)
		}
		first, second := <-results, <-results
		Expect((first == nil) != (second == nil)).To(BeTrue())
		if first != nil {
			Expect(errors.Is(first, domain.ErrForbidden)).To(BeTrue())
		} else {
			Expect(errors.Is(second, domain.ErrForbidden)).To(BeTrue())
		}
		stored, err := reader.Get(as(ctx, "setter"), item.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.AuthorID).NotTo(BeNil())
		Expect(*stored.AuthorID).To(Equal(users["setter"]))
		Expect(stored.OwnerID).NotTo(Equal(users["setter"]))
		Expect(writer.Delete(as(ctx, "setter"), item.ID)).To(MatchError(domain.ErrForbidden))
		visible, err := content.NewAccessStore(integrationDB).CanViewProblem(as(ctx, "setter"), item.ID, users["setter"], false)
		Expect(err).NotTo(HaveOccurred())
		Expect(visible).To(BeFalse())
	})

	It("serializes revocation behind an in-flight write and rejects the next write", func(ctx SpecContext) {
		group, err := domains.CreateGroup(ctx, "team", users["manager"], domain.GroupInput{Name: "Editors"})
		Expect(err).NotTo(HaveOccurred())
		Expect(domains.SetGroupMember(ctx, "team", users["manager"], group.ID, "editor", "member", false)).To(Succeed())
		Expect(writer.SetGrant(as(ctx, "setter"), item.ID, problem.GrantInput{Group: group.ID, Role: problem.AccessEditor})).To(Succeed())
		tx, err := integrationDB.Pool.BeginTxx(ctx, nil)
		Expect(err).NotTo(HaveOccurred())
		defer tx.Rollback()
		access, err := problem.LockAccess(as(ctx, "editor"), tx, item.ID, users["editor"])
		Expect(err).NotTo(HaveOccurred())
		Expect(access.Permissions.Edit).To(BeTrue())
		done := make(chan error, 1)
		go func() {
			done <- domains.SetGroupMember(ctx, "team", users["manager"], group.ID, "editor", "member", true)
		}()
		Eventually(func() (int, error) {
			var waiting int
			err := integrationDB.Pool.GetContext(ctx, &waiting, `SELECT count(*) FROM pg_stat_activity
			 WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT id FROM domains WHERE slug=%'`)
			return waiting, err
		}, time.Second*3).Should(BeNumerically(">", 0))
		Expect(tx.Commit()).To(Succeed())
		Eventually(done, time.Second*3).Should(Receive(Succeed()))
		_, err = authoring.NewPackageStore(integrationDB).SaveStatement(as(ctx, "editor"), authoring.Statement{ProblemID: item.ID, Language: "zh", Name: "Too late"})
		Expect(err).To(MatchError(domain.ErrForbidden))
	})

	It("enforces collaboration at HTTP boundaries without trusting global role claims or body ownership", func(ctx SpecContext) {
		service := problem.NewService(reader, writer)
		auth := middleware.NewAuthMiddleware(problemTestAuthenticator(users))
		router := gin.New()
		problemhandler.RegisterRoutes(router.Group("/api/domains/:domain"), problemhandler.NewProblemHandler(service), problemhandler.NewAdminProblemHandler(service), auth.Optional(), auth.Require(), middleware.ResolveDomain(domains), httpapi.PublicIDs(publicid.NewStore(integrationDB)))
		request := func(method, path, actor, body string) *httptest.ResponseRecorder {
			r := httptest.NewRequest(method, "/api/domains/team"+path, strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			if actor != "" {
				r.Header.Set("Authorization", "Bearer "+actor)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, r)
			return response
		}
		path := "/admin/problems/" + item.PublicID
		Expect(request("GET", path, "reader", "").Code).To(Equal(404))
		Expect(request("POST", "/admin/problems", "reader", `{"title":"Denied"}`).Code).To(Equal(403))
		created := request("POST", "/admin/problems", "setter", `{"title":"New","ownerId":"`+users["reader"]+`","domainId":"`+domain.OfficialID+`"}`)
		Expect(created.Code).To(Equal(201))
		Expect(created.Body.String()).To(ContainSubstring(`"ownerId":"` + users["setter"] + `"`))
		Expect(created.Body.String()).To(ContainSubstring(`"domainId":"` + scope.Domain.ID + `"`))
		Expect(request("PUT", path+"/access", "setter", `{"username":"editor","role":"editor"}`).Code).To(Equal(200))
		Expect(request("PUT", path+"/access", "editor", `{"username":"reader","role":"editor"}`).Code).To(Equal(403))
		Expect(request("PUT", path+"/access", "setter", `{"username":"reader","role":"owner"}`).Code).To(Equal(400))
		owned := request("GET", path, "editor", "")
		Expect(owned.Code).To(Equal(200))
		Expect(owned.Body.String()).To(ContainSubstring(`"edit":true`))
		Expect(owned.Body.String()).To(ContainSubstring(`"manageAccess":false`))
		Expect(owned.Body.String()).To(ContainSubstring(`"tags":[]`))
		Expect(request("DELETE", path, "editor", "").Code).To(Equal(403))
		Expect(request("GET", "/admin/problems", "editor", "").Body.String()).To(ContainSubstring(`"total":1`))
		upload := &unreadUpload{}
		r := httptest.NewRequest("POST", "/api/domains/team"+path+"/testdata", upload)
		r.Header.Set("Authorization", "Bearer reader")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, r)
		Expect(response.Code).To(Equal(403))
		Expect(upload.read).To(BeFalse())
		Expect(request("PUT", path+"/owner", "setter", `{"username":"reader"}`).Code).To(Equal(200))
		Expect(request("DELETE", path, "setter", "").Code).To(Equal(403))
		Expect(request("GET", path, "", "").Code).To(Equal(401))
	})
})
