package domain_test

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	"github.com/RimuruChan/Vertex/server/internal/domain"
	domainhandler "github.com/RimuruChan/Vertex/server/internal/domain/handler"
	"github.com/RimuruChan/Vertex/server/internal/identity"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var integrationDB *database.DB
var releaseDomainSuite = func() {}

type staleRoleAuthenticator struct{ users map[string]string }

func (a staleRoleAuthenticator) Authenticate(_ context.Context, token string) (*identity.Identity, error) {
	id, ok := a.users[token]
	if !ok {
		return nil, errors.New("unknown test actor")
	}
	return &identity.Identity{User: &identity.User{ID: id, Username: token, Role: "admin"}}, nil
}

var _ = BeforeSuite(func(ctx SpecContext) {
	var err error
	integrationDB, releaseDomainSuite, err = dbtest.Shared(ctx)
	Expect(err).NotTo(HaveOccurred())
})
var _ = AfterSuite(func() {
	releaseDomainSuite()
	if integrationDB != nil {
		integrationDB.Close()
	}
})

var _ = Describe("Domain use cases against PostgreSQL", func() {
	var service *domain.Service
	var users map[string]string
	var private domain.Scope
	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		service = domain.NewService(domain.NewStore(integrationDB))
		users = map[string]string{}
		for _, name := range []string{"owner", "alice", "bob", "outsider", "root"} {
			user, err := identity.NewUserStore(integrationDB).Create(ctx, name, name+"@example.test", "fixture-hash")
			Expect(err).NotTo(HaveOccurred())
			users[name] = user.ID
		}
		_, err := integrationDB.Pool.ExecContext(ctx, "UPDATE users SET role='admin' WHERE id=$1", users["root"])
		Expect(err).NotTo(HaveOccurred())
		private, err = service.Create(ctx, users["owner"], domain.CreateInput{Slug: "classroom", Name: "Classroom"})
		Expect(err).NotTo(HaveOccurred())
	})

	It("bootstraps official membership and keeps private domains out of totals", func(ctx SpecContext) {
		official, err := service.Get(ctx, "official", users["alice"])
		Expect(err).NotTo(HaveOccurred())
		Expect(official.MemberStatus).To(Equal("active"))
		Expect(official.Allows(domain.CreateSubmission)).To(BeTrue())
		_, total, err := service.List(ctx, users["outsider"], domain.Filters{})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		_, err = service.Get(ctx, "classroom", users["outsider"])
		Expect(err).To(MatchError(domain.ErrNotFound))
		visible, total, err := service.List(ctx, users["root"], domain.Filters{})
		Expect(err).NotTo(HaveOccurred())
		Expect(visible).To(HaveLen(2))
		Expect(total).To(Equal(2))
	})
	It("supports invitations, approval and suspension without granting pending permissions", func(ctx SpecContext) {
		Expect(service.SetMember(ctx, "classroom", users["owner"], domain.MemberInput{Username: "alice", RoleKey: "author", Status: "invited"})).To(Succeed())
		pending, err := service.Get(ctx, "classroom", users["alice"])
		Expect(err).NotTo(HaveOccurred())
		Expect(pending.CanEnter()).To(BeFalse())
		joined, err := service.Join(ctx, "classroom", users["alice"])
		Expect(err).NotTo(HaveOccurred())
		Expect(joined.Allows(domain.CreateProblem)).To(BeTrue())
		Expect(service.SetMember(ctx, "classroom", users["owner"], domain.MemberInput{Username: "alice", RoleKey: "author", Status: "suspended"})).To(Succeed())
		_, err = service.Join(ctx, "classroom", users["alice"])
		Expect(err).To(MatchError(domain.ErrNotFound))
		_, err = service.Update(ctx, "classroom", users["owner"], domain.UpdateInput{Name: "Classroom", Visibility: "public", JoinPolicy: "approval"})
		Expect(err).NotTo(HaveOccurred())
		requested, err := service.Join(ctx, "classroom", users["bob"])
		Expect(err).NotTo(HaveOccurred())
		Expect(requested.MemberStatus).To(Equal("pending"))
		Expect(requested.Permissions()).To(BeEmpty())
	})
	It("rejects privilege escalation and protects preset and in-use roles", func(ctx SpecContext) {
		Expect(service.SaveRole(ctx, "classroom", users["owner"], domain.RoleInput{Key: "membership_helper", Name: "Helper", Permissions: []domain.Permission{domain.ManageMembers, domain.ManageRoles}})).To(Succeed())
		Expect(service.SetMember(ctx, "classroom", users["owner"], domain.MemberInput{Username: "alice", RoleKey: "membership_helper", Status: "active"})).To(Succeed())
		err := service.SetMember(ctx, "classroom", users["alice"], domain.MemberInput{Username: "bob", RoleKey: "admin", Status: "active"})
		Expect(err).To(MatchError(domain.ErrForbidden))
		err = service.SaveRole(ctx, "classroom", users["alice"], domain.RoleInput{Key: "elevated", Name: "Elevated", Permissions: []domain.Permission{domain.CreateProblem}})
		Expect(err).To(MatchError(domain.ErrForbidden))
		err = service.SaveRole(ctx, "classroom", users["owner"], domain.RoleInput{Key: "siteadmin", Name: "Bad", Permissions: []domain.Permission{"site.admin"}})
		Expect(err).To(MatchError(domain.ErrInvalid))
		Expect(service.DeleteRole(ctx, "classroom", users["owner"], "admin")).To(MatchError(domain.ErrForbidden))
		Expect(service.DeleteRole(ctx, "classroom", users["owner"], "membership_helper")).To(MatchError(domain.ErrConflict))
	})
	It("protects the domain owner and the official domain", func(ctx SpecContext) {
		Expect(service.SetMember(ctx, "classroom", users["owner"], domain.MemberInput{Username: "owner", RoleKey: "viewer", Status: "active"})).To(MatchError(domain.ErrConflict))
		Expect(service.SetMember(ctx, "classroom", users["owner"], domain.MemberInput{Username: "owner", RoleKey: "admin", Status: "suspended"})).To(MatchError(domain.ErrConflict))
		Expect(service.Transfer(ctx, "classroom", users["owner"], "outsider")).To(MatchError(domain.ErrInvalid))
		Expect(service.Archive(ctx, "official", users["root"], true)).To(MatchError(domain.ErrForbidden))
		Expect(service.Transfer(ctx, "official", users["root"], "alice")).To(MatchError(domain.ErrForbidden))
		_, err := integrationDB.Pool.ExecContext(ctx, "UPDATE domains SET owner_id=$2 WHERE id=$1", private.Domain.ID, users["outsider"])
		Expect(err).To(HaveOccurred())
	})
	It("serializes ownership transfer so a former owner cannot transfer twice", func(ctx SpecContext) {
		for _, name := range []string{"alice", "bob"} {
			Expect(service.SetMember(ctx, "classroom", users["owner"], domain.MemberInput{Username: name, RoleKey: "member", Status: "active"})).To(Succeed())
		}
		results := make(chan error, 2)
		var ready sync.WaitGroup
		ready.Add(2)
		start := make(chan struct{})
		for _, name := range []string{"alice", "bob"} {
			go func(target string) {
				defer GinkgoRecover()
				ready.Done()
				<-start
				results <- service.Transfer(ctx, "classroom", users["owner"], target)
			}(name)
		}
		ready.Wait()
		close(start)
		first, second := <-results, <-results
		successes := 0
		for _, err := range []error{first, second} {
			if err == nil {
				successes++
			} else {
				Expect(err).To(MatchError(domain.ErrForbidden))
			}
		}
		Expect(successes).To(Equal(1))
		current, err := service.Get(ctx, "classroom", users["owner"])
		Expect(err).NotTo(HaveOccurred())
		Expect(*current.Domain.OwnerID).NotTo(Equal(users["owner"]))
		Expect(current.CanGovernOwnership()).To(BeFalse())
	})
	It("scopes group lookups, ownership and group members to one domain", func(ctx SpecContext) {
		Expect(service.SetMember(ctx, "classroom", users["owner"], domain.MemberInput{Username: "alice", RoleKey: "member", Status: "active"})).To(Succeed())
		group, err := service.CreateGroup(ctx, "classroom", users["owner"], domain.GroupInput{Name: "Team"})
		Expect(err).NotTo(HaveOccurred())
		Expect(service.SetGroupMember(ctx, "classroom", users["owner"], group.PublicID, "alice", "manager", false)).To(Succeed())
		Expect(service.UpdateGroup(ctx, "classroom", users["alice"], group.ID, domain.GroupInput{Name: "Team renamed"})).To(Succeed())
		Expect(service.TransferGroup(ctx, "classroom", users["alice"], group.ID, "alice")).To(MatchError(domain.ErrForbidden))
		_, err = service.Create(ctx, users["owner"], domain.CreateInput{Slug: "other", Name: "Other"})
		Expect(err).NotTo(HaveOccurred())
		_, _, err = service.Group(ctx, "other", users["owner"], group.ID)
		Expect(err).To(MatchError(domain.ErrNotFound))
		Expect(service.SetGroupMember(ctx, "classroom", users["owner"], group.ID, "outsider", "member", false)).To(MatchError(domain.ErrInvalid))
		_, err = integrationDB.Pool.ExecContext(ctx, "INSERT INTO domain_group_members(domain_id,group_id,user_id) VALUES($1,$2,$3)", domain.OfficialID, group.ID, users["alice"])
		Expect(err).To(HaveOccurred())
		members, total, err := service.GroupMembers(ctx, "classroom", users["alice"], group.ID, domain.Filters{Limit: 1})
		Expect(err).NotTo(HaveOccurred())
		Expect(members).To(HaveLen(1))
		Expect(total).To(Equal(2))
	})
	It("revokes group management when domain membership is suspended", func(ctx SpecContext) {
		Expect(service.SetMember(ctx, "classroom", users["owner"], domain.MemberInput{Username: "alice", RoleKey: "member", Status: "active"})).To(Succeed())
		group, err := service.CreateGroup(ctx, "classroom", users["owner"], domain.GroupInput{Name: "Team", OwnerUsername: "alice"})
		Expect(err).NotTo(HaveOccurred())
		Expect(service.SetMember(ctx, "classroom", users["owner"], domain.MemberInput{Username: "alice", RoleKey: "member", Status: "suspended"})).To(Succeed())
		Expect(service.UpdateGroup(ctx, "classroom", users["alice"], group.ID, domain.GroupInput{Name: "Forbidden"})).To(MatchError(domain.ErrNotFound))
		Expect(service.TransferGroup(ctx, "classroom", users["owner"], group.ID, "owner")).To(Succeed())
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE users SET disabled_at=now() WHERE id=$1", users["alice"])
		Expect(err).NotTo(HaveOccurred())
		Expect(service.SetGroupMember(ctx, "classroom", users["owner"], group.ID, "alice", "", true)).To(Succeed())
		members, total, err := service.GroupMembers(ctx, "classroom", users["owner"], group.ID, domain.Filters{})
		Expect(err).NotTo(HaveOccurred())
		Expect(members).To(HaveLen(1))
		Expect(total).To(Equal(1))
	})
	It("does not trust a revoked global account and records governance events", func(ctx SpecContext) {
		_, err := integrationDB.Pool.ExecContext(ctx, "UPDATE users SET disabled_at=now() WHERE id=$1", users["owner"])
		Expect(err).NotTo(HaveOccurred())
		_, err = service.Create(ctx, users["owner"], domain.CreateInput{Slug: "blocked", Name: "Blocked"})
		Expect(err).To(MatchError(domain.ErrUnauthenticated))
		var count int
		Expect(integrationDB.Pool.QueryRowContext(ctx, "SELECT count(*) FROM domain_audit_events WHERE domain_id=$1 AND actor_id=$2", private.Domain.ID, users["owner"]).Scan(&count)).To(Succeed())
		Expect(count).To(BeNumerically(">", 0), fmt.Sprint(private.Domain.ID))
	})

	It("checks actual database permissions through the HTTP routes and bounds write bodies", func(ctx SpecContext) {
		router := gin.New()
		auth := middleware.NewAuthMiddleware(staleRoleAuthenticator{users: users})
		domainhandler.NewHandler(service).RegisterRoutes(router.Group("/api"), auth.Optional(), auth.Require())
		send := func(method, path, username, body string) *httptest.ResponseRecorder {
			request := httptest.NewRequest(method, path, strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			if username != "" {
				request.Header.Set("Authorization", "Bearer "+username)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			return response
		}
		Expect(send("GET", "/api/domains/classroom", "outsider", "").Code).To(Equal(404))
		Expect(send("PUT", "/api/domains/classroom/members/bob", "outsider", `{"roleKey":"admin","status":"active"}`).Code).To(Equal(404))
		Expect(send("PUT", "/api/domains/classroom/members/bob", "owner", `{"roleKey":"admin","status":"active"}`).Code).To(Equal(200))
		Expect(send("PUT", "/api/domains/classroom/owner", "bob", `{"username":"bob"}`).Code).To(Equal(403))
		Expect(send("PUT", "/api/domains/classroom/roles/large", "owner", `{"name":"`+strings.Repeat("a", 20000)+`"}`).Code).To(Equal(413))
		Expect(send("GET", "/api/domains", "", "").Code).To(Equal(200))
	})
})
