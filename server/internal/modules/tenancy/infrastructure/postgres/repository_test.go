package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
	"github.com/RimuruChan/Vertex/server/internal/platform/database/dbtest"
	identityapp "github.com/RimuruChan/Vertex/server/internal/modules/identity/application"
	identitydomain "github.com/RimuruChan/Vertex/server/internal/modules/identity/domain"
	identitypg "github.com/RimuruChan/Vertex/server/internal/modules/identity/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/middleware"
	tenancyapp "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/application"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"
	tenancyhttp "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/transport/http"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

var integrationDB *database.DB
var releaseDomainSuite = func() {}

type staleRoleAuthenticator struct{ users map[string]string }

func (a staleRoleAuthenticator) Authenticate(_ context.Context, token string) (*identityapp.Identity, error) {
	id, ok := a.users[token]
	if !ok {
		return nil, errors.New("unknown test actor")
	}
	return &identityapp.Identity{User: &identitydomain.User{ID: id, Username: token, Role: "admin"}}, nil
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
	var service *tenancyapp.Service
	var users map[string]string
	var private tenancydomain.Scope
	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		service = tenancyapp.NewService(tenancypg.NewRepository(integrationDB))
		users = map[string]string{}
		for _, name := range []string{"owner", "alice", "bob", "outsider", "root"} {
			user, err := identitypg.NewUserRepository(integrationDB).Create(ctx, name, name+"@example.test", "fixture-hash")
			Expect(err).NotTo(HaveOccurred())
			users[name] = user.ID
		}
		_, err := integrationDB.Pool.ExecContext(ctx, "UPDATE users SET role='admin' WHERE id=$1", users["root"])
		Expect(err).NotTo(HaveOccurred())
		private, err = service.Create(ctx, users["owner"], tenancydomain.CreateInput{Slug: "classroom", Name: "Classroom"})
		Expect(err).NotTo(HaveOccurred())
	})

	It("bootstraps official membership and keeps private domains out of totals", func(ctx SpecContext) {
		official, err := service.Get(ctx, "official", users["alice"])
		Expect(err).NotTo(HaveOccurred())
		Expect(official.MemberStatus).To(Equal("active"))
		Expect(official.Allows(tenancydomain.CreateSubmission)).To(BeTrue())
		_, total, err := service.List(ctx, users["outsider"], tenancydomain.Filters{})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		_, err = service.Get(ctx, "classroom", users["outsider"])
		Expect(err).To(MatchError(tenancydomain.ErrNotFound))
		visible, total, err := service.List(ctx, users["root"], tenancydomain.Filters{})
		Expect(err).NotTo(HaveOccurred())
		Expect(visible).To(HaveLen(2))
		Expect(total).To(Equal(2))
	})
	It("supports invitations, approval and suspension without granting pending permissions", func(ctx SpecContext) {
		Expect(service.SetMember(ctx, "classroom", users["owner"], tenancydomain.MemberInput{Username: "alice", RoleKey: "author", Status: "invited"})).To(Succeed())
		pending, err := service.Get(ctx, "classroom", users["alice"])
		Expect(err).NotTo(HaveOccurred())
		Expect(pending.CanEnter()).To(BeFalse())
		joined, err := service.Join(ctx, "classroom", users["alice"])
		Expect(err).NotTo(HaveOccurred())
		Expect(joined.Allows(tenancydomain.CreateProblem)).To(BeTrue())
		Expect(service.SetMember(ctx, "classroom", users["owner"], tenancydomain.MemberInput{Username: "alice", RoleKey: "author", Status: "suspended"})).To(Succeed())
		_, err = service.Join(ctx, "classroom", users["alice"])
		Expect(err).To(MatchError(tenancydomain.ErrNotFound))
		_, err = service.Update(ctx, "classroom", users["owner"], tenancydomain.UpdateInput{Name: "Classroom", Visibility: "public", JoinPolicy: "approval"})
		Expect(err).NotTo(HaveOccurred())
		requested, err := service.Join(ctx, "classroom", users["bob"])
		Expect(err).NotTo(HaveOccurred())
		Expect(requested.MemberStatus).To(Equal("pending"))
		Expect(requested.Permissions()).To(BeEmpty())
	})
	It("rejects privilege escalation and protects preset and in-use roles", func(ctx SpecContext) {
		Expect(service.SaveRole(ctx, "classroom", users["owner"], tenancydomain.RoleInput{Key: "membership_helper", Name: "Helper", Permissions: []tenancydomain.Permission{tenancydomain.ManageMembers, tenancydomain.ManageRoles}})).To(Succeed())
		Expect(service.SetMember(ctx, "classroom", users["owner"], tenancydomain.MemberInput{Username: "alice", RoleKey: "membership_helper", Status: "active"})).To(Succeed())
		err := service.SetMember(ctx, "classroom", users["alice"], tenancydomain.MemberInput{Username: "bob", RoleKey: "admin", Status: "active"})
		Expect(err).To(MatchError(tenancydomain.ErrForbidden))
		err = service.SaveRole(ctx, "classroom", users["alice"], tenancydomain.RoleInput{Key: "elevated", Name: "Elevated", Permissions: []tenancydomain.Permission{tenancydomain.CreateProblem}})
		Expect(err).To(MatchError(tenancydomain.ErrForbidden))
		err = service.SaveRole(ctx, "classroom", users["owner"], tenancydomain.RoleInput{Key: "siteadmin", Name: "Bad", Permissions: []tenancydomain.Permission{"site.admin"}})
		Expect(err).To(MatchError(tenancydomain.ErrInvalid))
		Expect(service.DeleteRole(ctx, "classroom", users["owner"], "admin")).To(MatchError(tenancydomain.ErrForbidden))
		Expect(service.DeleteRole(ctx, "classroom", users["owner"], "membership_helper")).To(MatchError(tenancydomain.ErrConflict))
	})
	It("protects the domain owner and the official domain", func(ctx SpecContext) {
		Expect(service.SetMember(ctx, "classroom", users["owner"], tenancydomain.MemberInput{Username: "owner", RoleKey: "viewer", Status: "active"})).To(MatchError(tenancydomain.ErrConflict))
		Expect(service.SetMember(ctx, "classroom", users["owner"], tenancydomain.MemberInput{Username: "owner", RoleKey: "admin", Status: "suspended"})).To(MatchError(tenancydomain.ErrConflict))
		Expect(service.Transfer(ctx, "classroom", users["owner"], "outsider")).To(MatchError(tenancydomain.ErrInvalid))
		Expect(service.Archive(ctx, "official", users["root"], true)).To(MatchError(tenancydomain.ErrForbidden))
		Expect(service.Transfer(ctx, "official", users["root"], "alice")).To(MatchError(tenancydomain.ErrForbidden))
		_, err := integrationDB.Pool.ExecContext(ctx, "UPDATE domains SET owner_id=$2 WHERE id=$1", private.Domain.ID, users["outsider"])
		Expect(err).To(HaveOccurred())
	})
	It("serializes ownership transfer so a former owner cannot transfer twice", func(ctx SpecContext) {
		for _, name := range []string{"alice", "bob"} {
			Expect(service.SetMember(ctx, "classroom", users["owner"], tenancydomain.MemberInput{Username: name, RoleKey: "member", Status: "active"})).To(Succeed())
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
				Expect(err).To(MatchError(tenancydomain.ErrForbidden))
			}
		}
		Expect(successes).To(Equal(1))
		current, err := service.Get(ctx, "classroom", users["owner"])
		Expect(err).NotTo(HaveOccurred())
		Expect(*current.Domain.OwnerID).NotTo(Equal(users["owner"]))
		Expect(current.CanGovernOwnership()).To(BeFalse())
	})
	It("scopes group lookups, ownership and group members to one domain", func(ctx SpecContext) {
		Expect(service.SetMember(ctx, "classroom", users["owner"], tenancydomain.MemberInput{Username: "alice", RoleKey: "member", Status: "active"})).To(Succeed())
		group, err := service.CreateGroup(ctx, "classroom", users["owner"], tenancydomain.GroupInput{Name: "Team"})
		Expect(err).NotTo(HaveOccurred())
		Expect(service.SetGroupMember(ctx, "classroom", users["owner"], group.PublicID, "alice", "manager", false)).To(Succeed())
		Expect(service.UpdateGroup(ctx, "classroom", users["alice"], group.ID, tenancydomain.GroupInput{Name: "Team renamed"})).To(Succeed())
		Expect(service.TransferGroup(ctx, "classroom", users["alice"], group.ID, "alice")).To(MatchError(tenancydomain.ErrForbidden))
		_, err = service.Create(ctx, users["owner"], tenancydomain.CreateInput{Slug: "other", Name: "Other"})
		Expect(err).NotTo(HaveOccurred())
		_, _, err = service.Group(ctx, "other", users["owner"], group.ID)
		Expect(err).To(MatchError(tenancydomain.ErrNotFound))
		Expect(service.SetGroupMember(ctx, "classroom", users["owner"], group.ID, "outsider", "member", false)).To(MatchError(tenancydomain.ErrInvalid))
		_, err = integrationDB.Pool.ExecContext(ctx, "INSERT INTO domain_group_members(domain_id,group_id,user_id) VALUES($1,$2,$3)", tenancydomain.OfficialID, group.ID, users["alice"])
		Expect(err).To(HaveOccurred())
		members, total, err := service.GroupMembers(ctx, "classroom", users["alice"], group.ID, tenancydomain.Filters{Limit: 1})
		Expect(err).NotTo(HaveOccurred())
		Expect(members).To(HaveLen(1))
		Expect(total).To(Equal(2))
	})
	It("revokes group management when domain membership is suspended", func(ctx SpecContext) {
		Expect(service.SetMember(ctx, "classroom", users["owner"], tenancydomain.MemberInput{Username: "alice", RoleKey: "member", Status: "active"})).To(Succeed())
		group, err := service.CreateGroup(ctx, "classroom", users["owner"], tenancydomain.GroupInput{Name: "Team", OwnerUsername: "alice"})
		Expect(err).NotTo(HaveOccurred())
		Expect(service.SetMember(ctx, "classroom", users["owner"], tenancydomain.MemberInput{Username: "alice", RoleKey: "member", Status: "suspended"})).To(Succeed())
		Expect(service.UpdateGroup(ctx, "classroom", users["alice"], group.ID, tenancydomain.GroupInput{Name: "Forbidden"})).To(MatchError(tenancydomain.ErrNotFound))
		Expect(service.TransferGroup(ctx, "classroom", users["owner"], group.ID, "owner")).To(Succeed())
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE users SET disabled_at=now() WHERE id=$1", users["alice"])
		Expect(err).NotTo(HaveOccurred())
		Expect(service.SetGroupMember(ctx, "classroom", users["owner"], group.ID, "alice", "", true)).To(Succeed())
		members, total, err := service.GroupMembers(ctx, "classroom", users["owner"], group.ID, tenancydomain.Filters{})
		Expect(err).NotTo(HaveOccurred())
		Expect(members).To(HaveLen(1))
		Expect(total).To(Equal(1))
	})
	It("does not trust a revoked global account and records governance events", func(ctx SpecContext) {
		_, err := integrationDB.Pool.ExecContext(ctx, "UPDATE users SET disabled_at=now() WHERE id=$1", users["owner"])
		Expect(err).NotTo(HaveOccurred())
		_, err = service.Create(ctx, users["owner"], tenancydomain.CreateInput{Slug: "blocked", Name: "Blocked"})
		Expect(err).To(MatchError(tenancydomain.ErrUnauthenticated))
		var count int
		Expect(integrationDB.Pool.QueryRowContext(ctx, "SELECT count(*) FROM domain_audit_events WHERE domain_id=$1 AND actor_id=$2", private.Domain.ID, users["owner"]).Scan(&count)).To(Succeed())
		Expect(count).To(BeNumerically(">", 0), fmt.Sprint(private.Domain.ID))
	})

	It("checks actual database permissions through the HTTP routes and bounds write bodies", func(ctx SpecContext) {
		router := gin.New()
		auth := middleware.NewAuthMiddleware(staleRoleAuthenticator{users: users})
		tenancyhttp.NewHandler(service).RegisterRoutes(router.Group("/api"), auth.Optional(), auth.Require())
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

var _ = Describe("Resource relationship constraints", func() {
	It("rejects cross-domain foreign keys independently of the services", func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		user, err := identitypg.NewUserRepository(integrationDB).Create(ctx, "owner", "owner@example.test", "fixture-hash")
		Expect(err).NotTo(HaveOccurred())
		scope, err := tenancyapp.NewService(tenancypg.NewRepository(integrationDB)).Create(ctx, user.ID, tenancydomain.CreateInput{Slug: "training", Name: "Training"})
		Expect(err).NotTo(HaveOccurred())
		type fixture struct {
			domain, problem, contest, set, editorial, submission, rejudging string
			tag, post, clarification                                        int64
		}
		seed := func(domainID string) fixture {
			item := fixture{domain: domainID}
			Expect(integrationDB.Pool.GetContext(ctx, &item.problem, "INSERT INTO problems(domain_id,title,owner_id) VALUES($1,'Problem',$2) RETURNING id", domainID, user.ID)).To(Succeed())
			Expect(dbtest.PublishedProblems(ctx, integrationDB, item.problem)).To(Succeed())
			Expect(integrationDB.Pool.GetContext(ctx, &item.contest, "INSERT INTO contests(domain_id,title,begin_at,end_at,owner_id) VALUES($1,'Contest',now(),now(),$2) RETURNING id", domainID, user.ID)).To(Succeed())
			Expect(integrationDB.Pool.GetContext(ctx, &item.set, "INSERT INTO problem_sets(domain_id,title,owner_id) VALUES($1,'Set',$2) RETURNING id", domainID, user.ID)).To(Succeed())
			Expect(integrationDB.Pool.GetContext(ctx, &item.editorial, "INSERT INTO editorials(domain_id,problem_id,title) VALUES($1,$2,'Editorial') RETURNING id", domainID, item.problem)).To(Succeed())
			Expect(integrationDB.Pool.GetContext(ctx, &item.submission, "INSERT INTO submissions(domain_id,user_id,problem_id,language,source_code) VALUES($1,$2,$3,'cpp','source') RETURNING id", domainID, user.ID, item.problem)).To(Succeed())
			Expect(integrationDB.Pool.GetContext(ctx, &item.rejudging, "INSERT INTO rejudgings(domain_id) VALUES($1) RETURNING id", domainID)).To(Succeed())
			Expect(integrationDB.Pool.GetContext(ctx, &item.tag, "INSERT INTO tags(domain_id,name) VALUES($1,'Tag') RETURNING id", domainID)).To(Succeed())
			Expect(integrationDB.Pool.GetContext(ctx, &item.post, "INSERT INTO discussion_posts(domain_id,problem_id,content_md) VALUES($1,$2,'Post') RETURNING id", domainID, item.problem)).To(Succeed())
			Expect(integrationDB.Pool.GetContext(ctx, &item.clarification, "INSERT INTO clarifications(domain_id,contest_id,body,from_jury) VALUES($1,$2,'Announcement',true) RETURNING id", domainID, item.contest)).To(Succeed())
			return item
		}
		a, b := seed(tenancydomain.OfficialID), seed(scope.Domain.ID)
		cases := []struct {
			name, query string
			args        []any
		}{
			{"submission problem", "UPDATE submissions SET problem_id=$2 WHERE id=$1", []any{a.submission, b.problem}},
			{"submission contest", "UPDATE submissions SET contest_id=$2 WHERE id=$1", []any{a.submission, b.contest}},
			{"editorial problem", "UPDATE editorials SET problem_id=$2 WHERE id=$1", []any{a.editorial, b.problem}},
			{"discussion problem", "UPDATE discussion_posts SET problem_id=$2 WHERE id=$1", []any{a.post, b.problem}},
			{"discussion editorial", "UPDATE discussion_posts SET problem_id=NULL,editorial_id=$2 WHERE id=$1", []any{a.post, b.editorial}},
			{"discussion parent", "UPDATE discussion_posts SET parent_id=$2 WHERE id=$1", []any{a.post, b.post}},
			{"rejudging contest", "UPDATE rejudgings SET contest_id=$2 WHERE id=$1", []any{a.rejudging, b.contest}},
			{"rejudging problem", "UPDATE rejudgings SET problem_id=$2 WHERE id=$1", []any{a.rejudging, b.problem}},
			{"problem tag", "INSERT INTO problem_tags(problem_id,tag_id) VALUES($1,$2)", []any{a.problem, b.tag}},
			{"tag problem", "INSERT INTO problem_tags(problem_id,tag_id) VALUES($1,$2)", []any{b.problem, a.tag}},
			{"contest problem", "INSERT INTO contest_problems(contest_id,problem_id) VALUES($1,$2)", []any{a.contest, b.problem}},
			{"problem contest", "INSERT INTO contest_problems(contest_id,problem_id) VALUES($1,$2)", []any{b.contest, a.problem}},
			{"set problem", "INSERT INTO problem_set_problems(set_id,problem_id) VALUES($1,$2)", []any{a.set, b.problem}},
			{"problem set", "INSERT INTO problem_set_problems(set_id,problem_id) VALUES($1,$2)", []any{b.set, a.problem}},
			{"rejudging submission", "INSERT INTO rejudging_submissions(rejudging_id,submission_id,generation,prior_status,prior_score,prior_total_time_ms,prior_peak_memory_kb,prior_compile_result,prior_case_results,prior_judged_cases,prior_total_cases,prior_problem_version) VALUES($1,$2,1,'Accepted',100,0,0,'','[]',0,0,1)", []any{a.rejudging, b.submission}},
			{"submission rejudging", "INSERT INTO rejudging_submissions(rejudging_id,submission_id,generation,prior_status,prior_score,prior_total_time_ms,prior_peak_memory_kb,prior_compile_result,prior_case_results,prior_judged_cases,prior_total_cases,prior_problem_version) VALUES($1,$2,1,'Accepted',100,0,0,'','[]',0,0,1)", []any{b.rejudging, a.submission}},
			{"scoreboard problem", "INSERT INTO contest_submission_cells(contest_id,problem_id,user_id) VALUES($1,$2,$3)", []any{a.contest, b.problem, user.ID}},
			{"scoreboard contest", "INSERT INTO contest_submission_cells(contest_id,problem_id,user_id) VALUES($1,$2,$3)", []any{b.contest, a.problem, user.ID}},
			{"clarification problem", "UPDATE clarifications SET problem_id=$2 WHERE id=$1", []any{a.clarification, b.problem}},
			{"clarification contest", "UPDATE clarifications SET contest_id=$2 WHERE id=$1", []any{a.clarification, b.contest}},
			{"clarification parent", "UPDATE clarifications SET parent_id=$2 WHERE id=$1", []any{a.clarification, b.clarification}},
		}
		for _, test := range cases {
			_, err := integrationDB.Pool.ExecContext(ctx, test.query, test.args...)
			var pgErr *pgconn.PgError
			Expect(errors.As(err, &pgErr)).To(BeTrue(), "%s: %v", test.name, err)
			Expect(pgErr.Code).To(Equal("23503"), test.name)
		}
		// Public-number allocation is local to the kind as well as the domain.
		for _, table := range []string{"contests", "submissions", "problem_sets", "editorials"} {
			var numbers []int64
			Expect(integrationDB.Pool.SelectContext(ctx, &numbers, fmt.Sprintf("SELECT public_id FROM %s ORDER BY domain_id", table))).To(Succeed())
			Expect(numbers).To(Equal([]int64{1, 1}), table)
		}
	})
})

func TestDomain(t *testing.T) { RegisterFailHandler(Fail); RunSpecs(t, "Domain Suite") }
