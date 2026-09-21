package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	authoringapp "github.com/RimuruChan/Vertex/server/internal/modules/authoring/application"
	authoringdomain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	authoringfiles "github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/filesystem"
	authoringpg "github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres"
	authoringhttp "github.com/RimuruChan/Vertex/server/internal/modules/authoring/transport/http"
	contentpg "github.com/RimuruChan/Vertex/server/internal/modules/content/infrastructure/postgres"
	identityapp "github.com/RimuruChan/Vertex/server/internal/modules/identity/application"
	identitydomain "github.com/RimuruChan/Vertex/server/internal/modules/identity/domain"
	identitypg "github.com/RimuruChan/Vertex/server/internal/modules/identity/infrastructure/postgres"
	problemapp "github.com/RimuruChan/Vertex/server/internal/modules/problem/application"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
	problempg "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres"
	problemhttp "github.com/RimuruChan/Vertex/server/internal/modules/problem/transport/http"
	tenancyapp "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/application"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/platform/database/dbtest"
	"github.com/RimuruChan/Vertex/server/internal/transport/http"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/middleware"
	references "github.com/RimuruChan/Vertex/server/internal/transport/http/references"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

type problemTestAuthenticator map[string]string

func (a problemTestAuthenticator) Authenticate(_ context.Context, token string) (*identityapp.Identity, error) {
	id, ok := a[token]
	if !ok {
		return nil, errors.New("unknown fixture actor")
	}
	// A stale upstream role must not override current domain/resource policy.
	return &identityapp.Identity{User: &identitydomain.User{ID: id, Username: token, Role: "admin"}}, nil
}

type unreadUpload struct{ read bool }

func (u *unreadUpload) Read([]byte) (int, error) {
	u.read = true
	return 0, errors.New("unauthorized upload body was read")
}

var _ = Describe("Problem ownership and collaboration against PostgreSQL", func() {
	var domains *tenancyapp.Service
	var reader *problempg.Queries
	var writer *problempg.Repository
	var users map[string]string
	var scope tenancydomain.Scope
	var item *problemdomain.ProblemView
	as := func(ctx context.Context, username string) context.Context {
		return tenancydomain.WithScope(ctx, tenancydomain.Scope{Domain: scope.Domain, UserID: users[username]})
	}
	BeforeEach(func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		domains = tenancyapp.NewService(tenancypg.NewRepository(integrationDB))
		reader = problempg.NewQueries(integrationDB)
		writer = problempg.NewRepository(integrationDB)
		users = map[string]string{}
		for _, name := range []string{"manager", "setter", "editor", "reader", "outsider"} {
			user, err := identitypg.NewUserRepository(integrationDB).Create(ctx, name, name+"@example.test", "fixture")
			Expect(err).NotTo(HaveOccurred())
			users[name] = user.ID
		}
		var err error
		scope, err = domains.Create(ctx, users["manager"], tenancydomain.CreateInput{Slug: "team", Name: "Team"})
		Expect(err).NotTo(HaveOccurred())
		for _, name := range []string{"setter", "editor", "reader"} {
			role := "member"
			if name == "setter" {
				role = "author"
			}
			Expect(domains.SetMember(ctx, "team", users["manager"], tenancydomain.MemberInput{Username: name, RoleKey: role, Status: "active"})).To(Succeed())
		}
		item, err = writer.Create(as(ctx, "setter"), users["setter"], &problemdomain.CreateInput{Title: "Unreleased", Visibility: "draft"})
		Expect(err).NotTo(HaveOccurred())
	})

	It("keeps creation, package editing and owner-only actions distinct", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		_, err := writer.Create(as(ctx, "reader"), users["reader"], &problemdomain.CreateInput{Title: "Unauthorized"})
		Expect(err).To(MatchError(tenancydomain.ErrForbidden))
		Expect(writer.SetGrant(as(ctx, "setter"), item.ID, problemdomain.GrantInput{Username: "editor", Role: problemdomain.AccessEditor})).To(Succeed())
		Expect(writer.SetGrant(as(ctx, "setter"), item.ID, problemdomain.GrantInput{Username: "reader", Role: problemdomain.AccessReader})).To(Succeed())
		blobs, err := authoringfiles.NewBlobStore(GinkgoT().TempDir(), 1<<20)
		Expect(err).NotTo(HaveOccurred())
		repo := authoringpg.NewRevisionRepository(integrationDB, blobs)
		service := authoringapp.NewWorkbench(repo)
		_, err = service.Open(as(ctx, "reader"), item.ID)
		Expect(err).To(MatchError(tenancydomain.ErrForbidden))
		copy, err := service.Open(as(ctx, "editor"), item.ID)
		Expect(err).NotTo(HaveOccurred())
		material, err := service.Material(as(ctx, "editor"), item.ID, "problem", 0)
		Expect(err).NotTo(HaveOccurred())
		material.Metadata.Title = "Edited"
		text, err := json.Marshal(material.Metadata)
		Expect(err).NotTo(HaveOccurred())
		body := string(text)
		copy, err = service.SaveEntry(as(ctx, "editor"), item.ID, copy.ETag, material.Entry, &body)
		Expect(err).NotTo(HaveOccurred())
		history, err := repo.History(as(ctx, "reader"), item.ID, 0, 10)
		Expect(err).NotTo(HaveOccurred())
		Expect(history).To(BeEmpty())
		committed, err := repo.Commit(as(ctx, "editor"), item.ID, authoringdomain.CommitInput{ETag: copy.ETag, RequestID: "share", Message: "Share edited metadata"})
		Expect(err).NotTo(HaveOccurred())
		shared, err := service.Material(as(ctx, "reader"), item.ID, "problem", committed.Commit.Revision)
		Expect(err).NotTo(HaveOccurred())
		Expect(shared.Metadata.Title).To(Equal("Edited"))
		Expect(writer.Delete(as(ctx, "editor"), item.ID)).To(MatchError(tenancydomain.ErrForbidden))
		Expect(writer.Transfer(as(ctx, "editor"), item.ID, "reader")).To(MatchError(tenancydomain.ErrForbidden))
		_, err = repo.SetVisibility(as(ctx, "editor"), item.ID, authoringdomain.VisibilityChange{Visibility: "public", ExpectedVisibility: "draft"})
		Expect(err).To(MatchError(tenancydomain.ErrForbidden))
		Expect(domains.SetMember(ctx, "team", users["manager"], tenancydomain.MemberInput{Username: "setter", RoleKey: "viewer", Status: "active"})).To(Succeed())
		access, err := reader.Access(as(ctx, "setter"), item.ID, users["setter"])
		Expect(err).NotTo(HaveOccurred())
		Expect(access.Permissions.Edit && access.Permissions.Publish).To(BeTrue())
		_, err = writer.Create(as(ctx, "setter"), users["setter"], &problemdomain.CreateInput{Title: "No longer a creator"})
		Expect(err).To(MatchError(tenancydomain.ErrForbidden))
	})

	It("lists authorized published reuse candidates without exposing private or working metadata", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		public, err := writer.Create(as(ctx, "setter"), users["setter"], &problemdomain.CreateInput{Title: "Public candidate", Visibility: "public"})
		Expect(err).NotTo(HaveOccurred())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, item.ID, public.ID)).To(Succeed())
		_, err = writer.Create(as(ctx, "setter"), users["setter"], &problemdomain.CreateInput{Title: "Unpublished", Visibility: "public"})
		Expect(err).NotTo(HaveOccurred())
		service := problemapp.NewService(reader, writer)
		filter := problemdomain.Filters{Available: true, ViewerID: users["reader"], Limit: 20}
		items, total, err := service.List(as(ctx, "reader"), filter, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items[0].ID).To(Equal(public.ID))
		Expect(writer.SetGrant(as(ctx, "setter"), item.ID, problemdomain.GrantInput{Username: "reader", Role: problemdomain.AccessReader})).To(Succeed())
		blobs, err := authoringfiles.NewBlobStore(GinkgoT().TempDir(), 1<<20)
		Expect(err).NotTo(HaveOccurred())
		workbench := authoringapp.NewWorkbench(authoringpg.NewRevisionRepository(integrationDB, blobs))
		copy, err := workbench.Open(as(ctx, "setter"), item.ID)
		Expect(err).NotTo(HaveOccurred())
		material, err := workbench.Material(as(ctx, "setter"), item.ID, "problem", 0)
		Expect(err).NotTo(HaveOccurred())
		material.Metadata.Title = "Unreleased secret title"
		encoded, err := json.Marshal(material.Metadata)
		Expect(err).NotTo(HaveOccurred())
		text := string(encoded)
		_, err = workbench.SaveEntry(as(ctx, "setter"), item.ID, copy.ETag, material.Entry, &text)
		Expect(err).NotTo(HaveOccurred())
		items, total, err = service.List(as(ctx, "reader"), filter, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(2))
		for _, candidate := range items {
			Expect(candidate.Title).NotTo(Equal("Unreleased secret title"))
		}
		filter.Available = false
		_, total, err = service.List(as(ctx, "reader"), filter, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		_, _, err = service.List(as(ctx, "reader"), problemdomain.Filters{Available: true}, false)
		Expect(err).To(MatchError(tenancydomain.ErrUnauthenticated))
	})

	It("computes group inheritance dynamically and never converts it into permanent user grants", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		group, err := domains.CreateGroup(ctx, "team", users["manager"], tenancydomain.GroupInput{Name: "Reviewers"})
		Expect(err).NotTo(HaveOccurred())
		Expect(domains.SetGroupMember(ctx, "team", users["manager"], group.ID, "reader", "member", false)).To(Succeed())
		Expect(writer.SetGrant(as(ctx, "setter"), item.ID, problemdomain.GrantInput{Group: group.ID, Role: problemdomain.AccessReader})).To(Succeed())
		Expect(writer.SetGrant(as(ctx, "setter"), item.ID, problemdomain.GrantInput{Username: "reader", Role: problemdomain.AccessEditor})).To(Succeed())
		access, err := reader.Access(as(ctx, "reader"), item.ID, users["reader"])
		Expect(err).NotTo(HaveOccurred())
		Expect(access.Role).To(Equal(problemdomain.AccessEditor))
		grants, err := reader.Grants(as(ctx, "setter"), item.ID)
		Expect(err).NotTo(HaveOccurred())
		for _, grant := range grants {
			if grant.UserID != nil {
				Expect(writer.RemoveGrant(as(ctx, "setter"), item.ID, grant.ID)).To(Succeed())
			}
		}
		access, err = reader.Access(as(ctx, "reader"), item.ID, users["reader"])
		Expect(err).NotTo(HaveOccurred())
		Expect(access.Role).To(Equal(problemdomain.AccessReader))
		Expect(access.Permissions.ReadPackage).To(BeTrue())
		Expect(access.Permissions.Edit).To(BeFalse())
		Expect(domains.SetGroupMember(ctx, "team", users["manager"], group.ID, "reader", "member", true)).To(Succeed())
		access, err = reader.Access(as(ctx, "reader"), item.ID, users["reader"])
		Expect(err).NotTo(HaveOccurred())
		Expect(access.Permissions.View || access.Permissions.ReadPackage).To(BeFalse())
		items, total, err := reader.List(as(ctx, "reader"), problemdomain.Filters{Workspace: true, ViewerID: users["reader"]})
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(BeEmpty())
		Expect(total).To(BeZero())
	})

	It("transfers ownership once under concurrency and preserves the original creator", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
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
			Expect(errors.Is(first, tenancydomain.ErrForbidden)).To(BeTrue())
		} else {
			Expect(errors.Is(second, tenancydomain.ErrForbidden)).To(BeTrue())
		}
		stored, err := reader.Get(as(ctx, "setter"), item.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.AuthorID).NotTo(BeNil())
		Expect(*stored.AuthorID).To(Equal(users["setter"]))
		Expect(stored.OwnerID).NotTo(Equal(users["setter"]))
		Expect(writer.Delete(as(ctx, "setter"), item.ID)).To(MatchError(tenancydomain.ErrForbidden))
		visible, err := contentpg.NewProblemAccess(integrationDB).CanViewProblem(as(ctx, "setter"), item.ID, users["setter"])
		Expect(err).NotTo(HaveOccurred())
		Expect(visible).To(BeFalse())
	})

	It("serializes revocation behind an in-flight write and rejects the next write", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		group, err := domains.CreateGroup(ctx, "team", users["manager"], tenancydomain.GroupInput{Name: "Editors"})
		Expect(err).NotTo(HaveOccurred())
		Expect(domains.SetGroupMember(ctx, "team", users["manager"], group.ID, "editor", "member", false)).To(Succeed())
		Expect(writer.SetGrant(as(ctx, "setter"), item.ID, problemdomain.GrantInput{Group: group.ID, Role: problemdomain.AccessEditor})).To(Succeed())
		tx, err := integrationDB.Pool.BeginTxx(ctx, nil)
		Expect(err).NotTo(HaveOccurred())
		defer tx.Rollback()
		access, err := problempg.LockAccess(as(ctx, "editor"), tx, item.ID, users["editor"])
		Expect(err).NotTo(HaveOccurred())
		Expect(access.Permissions.Edit).To(BeTrue())
		done := make(chan error, 1)
		go func() {
			done <- domains.SetGroupMember(ctx, "team", users["manager"], group.ID, "editor", "member", true)
		}()
		Eventually(func() (int, error) {
			var waiting int
			err := integrationDB.Pool.GetContext(ctx, &waiting, `SELECT count(*) FROM pg_stat_activity
			 WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%SELECT id FROM domains WHERE slug=%'`)
			return waiting, err
		}, time.Second*3).Should(BeNumerically(">", 0))
		Expect(tx.Commit()).To(Succeed())
		Eventually(done, time.Second*3).Should(Receive(Succeed()))
		blobs, err := authoringfiles.NewBlobStore(GinkgoT().TempDir(), 1<<20)
		Expect(err).NotTo(HaveOccurred())
		_, err = authoringpg.NewRevisionRepository(integrationDB, blobs).Open(as(ctx, "editor"), item.ID)
		Expect(err).To(MatchError(tenancydomain.ErrForbidden))
	})

	It("enforces collaboration at HTTP boundaries without trusting global role claims or body ownership", func(spec SpecContext) {

		service := problemapp.NewService(reader, writer)
		auth := middleware.NewAuthMiddleware(problemTestAuthenticator(users))
		router := gin.New()
		problemhttp.RegisterRoutes(router.Group("/api/domains/:domain"), problemhttp.NewProblemHandler(service, nil), problemhttp.NewAdminProblemHandler(service), auth.Optional(), auth.Require(), middleware.ResolveDomain(domains), httpapi.ResourceReferences(references.NewResolver(integrationDB)))
		blobs, err := authoringfiles.NewBlobStore(GinkgoT().TempDir(), 1<<20)
		Expect(err).NotTo(HaveOccurred())
		workbench := authoringapp.NewWorkbench(authoringpg.NewRevisionRepository(integrationDB, blobs))
		authoringhttp.NewWorkbenchHandler(workbench, 1<<20).RegisterRoutes(router.Group("/api/domains/:domain"), auth.Require(), middleware.ResolveDomain(domains), httpapi.ResourceReferences(references.NewResolver(integrationDB)))
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
		created := request("POST", "/admin/problems", "setter", `{"title":"New","ownerId":"`+users["reader"]+`","domainId":"`+tenancydomain.OfficialID+`"}`)
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
		for _, action := range []string{"blobs", "imports"} {
			upload := &unreadUpload{}
			r := httptest.NewRequest("POST", "/api/domains/team/authoring/problems/"+item.PublicID+"/"+action, upload)
			r.Header.Set("Authorization", "Bearer reader")
			r.Header.Set("Content-Type", "multipart/form-data; boundary=fixture")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, r)
			Expect(response.Code).To(Equal(403))
			Expect(upload.read).To(BeFalse())
		}
		Expect(request("PUT", path+"/owner", "setter", `{"username":"reader"}`).Code).To(Equal(200))
		Expect(request("DELETE", path, "setter", "").Code).To(Equal(403))
		Expect(request("GET", path, "", "").Code).To(Equal(401))
	})
})
