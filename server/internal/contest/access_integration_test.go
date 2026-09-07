package contest_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/contest"
	contesthandler "github.com/RimuruChan/Vertex/server/internal/contest/handler"
	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/identity"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/RimuruChan/Vertex/server/internal/problem"
	"github.com/RimuruChan/Vertex/server/internal/publicid"
	"github.com/RimuruChan/Vertex/server/internal/ratelimit"
	"github.com/RimuruChan/Vertex/server/internal/transport/httpapi"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type contestTestAuth map[string]string

func (a contestTestAuth) Authenticate(_ context.Context, token string) (*identity.Identity, error) {
	id, ok := a[token]
	if !ok {
		return nil, errors.New("unknown fixture actor")
	}
	return &identity.Identity{User: &identity.User{ID: id, Username: token, Role: "admin"}}, nil
}

var _ = Describe("Contest collaboration against PostgreSQL", func() {
	var spaces *domain.Service
	var store *contest.ContestStore
	var users map[string]string
	var scope domain.Scope
	var event *contest.Contest
	var question *problem.Problem
	as := func(ctx context.Context, name string) context.Context {
		return domain.WithScope(ctx, domain.Scope{Domain: scope.Domain, UserID: users[name]})
	}
	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		spaces = domain.NewService(domain.NewStore(integrationDB))
		store = contest.NewContestStore(integrationDB)
		users = map[string]string{}
		for _, name := range []string{"manager", "owner", "editor", "jury", "observer", "entrant", "outside"} {
			user, err := identity.NewUserStore(integrationDB).Create(ctx, name, name+"@example.test", "fixture")
			Expect(err).NotTo(HaveOccurred())
			users[name] = user.ID
		}
		var err error
		scope, err = spaces.Create(ctx, users["manager"], domain.CreateInput{Slug: "team", Name: "Team"})
		Expect(err).NotTo(HaveOccurred())
		for _, name := range []string{"owner", "editor", "jury", "observer", "entrant"} {
			role := "member"
			if name == "owner" {
				role = "author"
			}
			Expect(spaces.SetMember(ctx, "team", users["manager"], domain.MemberInput{Username: name, RoleKey: role, Status: "active"})).To(Succeed())
		}
		event, err = store.Create(as(ctx, "owner"), users["owner"], &contest.PersistInput{Title: "Private round", Rule: "icpc", Visibility: "private", Admission: contest.AdmissionRestricted, Feedback: "full", BeginAt: time.Now().Add(time.Hour), EndAt: time.Now().Add(2 * time.Hour), RankboardVisible: true})
		Expect(err).NotTo(HaveOccurred())
		question, err = problem.NewProblemAdminStore(integrationDB, GinkgoT().TempDir()).Create(as(ctx, "owner"), users["owner"], &problem.CreateInput{Title: "Hidden task"})
		Expect(err).NotTo(HaveOccurred())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, question.ID)).To(Succeed())
		Expect(store.SetProblems(as(ctx, "owner"), event.ID, []contest.ProblemEntry{{ProblemID: question.ID, Label: "A"}})).To(Succeed())
	})

	It("filters management lists and separates preparation, jury operations and participation", func(ctx SpecContext) {
		for _, role := range []string{contest.AccessEditor, contest.AccessJury, contest.AccessObserver} {
			Expect(store.SetGrant(as(ctx, "owner"), event.ID, contest.GrantInput{Username: role, Role: role})).To(Succeed())
		}
		_, total, err := store.ListAdmin(as(ctx, "entrant"), 20, 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(BeZero())
		managed, total, err := store.ListAdmin(as(ctx, "editor"), 20, 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(managed).To(HaveLen(1))
		Expect(managed[0].Permissions.Edit).To(BeTrue())
		Expect(managed[0].Permissions.ViewJury).To(BeFalse())
		_, err = store.AddStaff(as(ctx, "jury"), event.ID, "entrant", contest.StaffJury)
		Expect(err).To(MatchError(contest.ErrForbidden))
		Expect(store.Register(as(ctx, "observer"), event.ID, users["observer"])).To(MatchError(contest.ErrForbidden))
		Expect(store.Register(as(ctx, "entrant"), event.ID, users["entrant"])).To(MatchError(contest.ErrForbidden))
		Expect(store.SetGrant(as(ctx, "owner"), event.ID, contest.GrantInput{Username: "entrant", Role: contest.AccessParticipant})).To(Succeed())
		Expect(store.Register(as(ctx, "entrant"), event.ID, users["entrant"])).To(Succeed())
		access, err := store.Access(as(ctx, "entrant"), event.ID, users["entrant"])
		Expect(err).NotTo(HaveOccurred())
		Expect(access.Permissions.Submit).To(BeTrue())
		Expect(access.Permissions.PreviewProblems).To(BeFalse())
		service := contest.NewService(store, nil)
		_, err = service.Problem(as(ctx, "entrant"), event.ID, "A", users["entrant"], "admin")
		Expect(err).To(MatchError(contest.ErrNotFound))
		preview, err := service.Problem(as(ctx, "editor"), event.ID, "A", users["editor"], "user")
		Expect(err).NotTo(HaveOccurred())
		Expect(preview.Title).To(Equal("Hidden task"))
	})

	It("keeps group jury grants live only while membership is active", func(ctx SpecContext) {
		group, err := spaces.CreateGroup(ctx, "team", users["manager"], domain.GroupInput{Name: "Jury"})
		Expect(err).NotTo(HaveOccurred())
		Expect(spaces.SetGroupMember(ctx, "team", users["manager"], group.ID, "jury", "member", false)).To(Succeed())
		Expect(store.SetGrant(as(ctx, "owner"), event.ID, contest.GrantInput{Group: group.ID, Role: contest.AccessJury})).To(Succeed())
		role, err := store.StaffRole(as(ctx, "jury"), event.ID, users["jury"])
		Expect(err).NotTo(HaveOccurred())
		Expect(role).To(Equal(contest.StaffJury))
		Expect(spaces.SetMember(ctx, "team", users["manager"], domain.MemberInput{Username: "jury", RoleKey: "member", Status: "suspended"})).To(Succeed())
		role, err = store.StaffRole(as(ctx, "owner"), event.ID, users["jury"])
		Expect(err).NotTo(HaveOccurred())
		Expect(role).To(BeEmpty())
		_, err = store.Access(as(ctx, "jury"), event.ID, users["jury"])
		Expect(err).To(MatchError(contest.ErrNotFound))
		Expect(spaces.SetMember(ctx, "team", users["manager"], domain.MemberInput{Username: "jury", RoleKey: "member", Status: "active"})).To(Succeed())
		Expect(spaces.SetGroupMember(ctx, "team", users["manager"], group.ID, "jury", "member", true)).To(Succeed())
		access, err := store.Access(as(ctx, "jury"), event.ID, users["jury"])
		Expect(err).NotTo(HaveOccurred())
		Expect(access.Permissions.ViewJury).To(BeFalse())
	})

	It("rechecks verified passwords and eligibility under the registration lock", func(ctx SpecContext) {
		oldHash, err := identity.HashPassword("old-password")
		Expect(err).NotTo(HaveOccurred())
		newHash, err := identity.HashPassword("new-password")
		Expect(err).NotTo(HaveOccurred())
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE contests SET visibility='password',admission='members',password_hash=$2 WHERE id=$1", event.ID, newHash)
		Expect(err).NotTo(HaveOccurred())
		Expect(store.Register(as(ctx, "entrant"), event.ID, users["entrant"], oldHash)).To(MatchError(contest.ErrInvalidPassword))
		Expect(store.Register(as(ctx, "entrant"), event.ID, users["entrant"], newHash)).To(Succeed())
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE contests SET admission='restricted' WHERE id=$1", event.ID)
		Expect(err).NotTo(HaveOccurred())
		access, err := store.Access(as(ctx, "entrant"), event.ID, users["entrant"])
		Expect(err).NotTo(HaveOccurred())
		Expect(access.Registered).To(BeTrue())
		Expect(access.Permissions.Submit).To(BeFalse())
	})

	It("rejects foreign principals and transfers once without changing the creator", func(ctx SpecContext) {
		_, err := integrationDB.Pool.ExecContext(ctx, "INSERT INTO contest_access(domain_id,contest_id,user_id,role) VALUES($1,$2,$3,'jury')", scope.Domain.ID, event.ID, users["outside"])
		Expect(err).To(HaveOccurred())
		Expect(store.SetGrant(as(ctx, "owner"), event.ID, contest.GrantInput{Username: "outside", Role: contest.AccessJury})).To(MatchError(contest.ErrInvalidInput))
		results := make(chan error, 2)
		var start sync.WaitGroup
		start.Add(2)
		for _, name := range []string{"entrant", "editor"} {
			go func(name string) {
				start.Done()
				start.Wait()
				results <- store.Transfer(as(ctx, "owner"), event.ID, name)
			}(name)
		}
		first, second := <-results, <-results
		Expect((first == nil) != (second == nil)).To(BeTrue())
		if first != nil {
			Expect(errors.Is(first, contest.ErrForbidden)).To(BeTrue())
		} else {
			Expect(errors.Is(second, contest.ErrForbidden)).To(BeTrue())
		}
		stored, err := store.Get(as(ctx, "manager"), event.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(*stored.CreatedBy).To(Equal(users["owner"]))
		Expect(stored.OwnerID).NotTo(Equal(users["owner"]))
		Expect(store.Delete(as(ctx, "owner"), event.ID)).To(MatchError(contest.ErrForbidden))
	})

	It("checks current capabilities at HTTP boundaries despite stale role claims", func(ctx SpecContext) {
		service := contest.NewService(store, nil)
		handler := contesthandler.NewContestHandler(service, ratelimit.Policy{})
		auth := middleware.NewAuthMiddleware(contestTestAuth(users))
		router := gin.New()
		handler.RegisterRoutes(router.Group("/api/domains/:domain"), auth.Optional(), auth.Require(), middleware.ResolveDomain(spaces), httpapi.PublicIDs(publicid.NewStore(integrationDB)))
		request := func(method, path, actor, body string) *httptest.ResponseRecorder {
			r := httptest.NewRequest(method, "/api/domains/team"+path, strings.NewReader(body))
			if actor != "" {
				r.Header.Set("Authorization", "Bearer "+actor)
			}
			r.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, r)
			return response
		}
		path := "/contests/" + event.PublicID
		Expect(request("GET", path, "entrant", "").Code).To(Equal(404))
		Expect(request("GET", "/admin/contests", "entrant", "").Body.String()).To(ContainSubstring(`"total":0`))
		Expect(request("PUT", path+"/access", "owner", `{"username":"jury","role":"jury"}`).Code).To(Equal(200))
		Expect(request("POST", path+"/staff", "jury", `{"username":"entrant","role":"jury"}`).Code).To(Equal(403))
		Expect(request("PUT", path+"/access", "owner", `{"username":"observer","role":"observer"}`).Code).To(Equal(200))
		Expect(request("POST", path+"/clarifications/reply", "observer", `{"body":"Denied"}`).Code).To(Equal(403))
		Expect(request("POST", path+"/register", "observer", "").Code).To(Equal(403))
		Expect(request("PUT", path+"/access", "owner", `{"username":"entrant","role":"participant"}`).Code).To(Equal(200))
		Expect(request("POST", path+"/register", "entrant", "").Code).To(Equal(200))
		Expect(request("GET", path+"/problems/A", "entrant", "").Code).To(Equal(404))
		Expect(request("GET", path+"/problems/A", "jury", "").Code).To(Equal(200))
		_, err := integrationDB.Pool.ExecContext(ctx, "UPDATE contests SET begin_at=now()-interval '1 hour',end_at=now()+interval '1 hour',freeze_at=now()-interval '1 minute' WHERE id=$1", event.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(request("GET", path+"/rankboard?view=jury", "entrant", "").Body.String()).To(ContainSubstring(`"juryView":false`))
		Expect(request("GET", path+"/rankboard?view=jury", "observer", "").Body.String()).To(ContainSubstring(`"juryView":true`))
		Expect(request("PUT", path+"/owner", "owner", `{"username":"editor"}`).Code).To(Equal(200))
		Expect(request("DELETE", path, "owner", "").Code).To(Equal(403))
	})
})
