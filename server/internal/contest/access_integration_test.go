package contest_test

import (
	"context"
	"encoding/json"
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

	Describe("registration policy", func() {
		settings := func() *contest.PersistInput {
			return &contest.PersistInput{Title: event.Title, Rule: event.Rule, Visibility: event.Visibility, Admission: event.Admission, Feedback: event.Feedback, BeginAt: event.BeginAt, EndAt: event.EndAt, RankboardVisible: true}
		}
		It("defaults to pre-start self registration and preserves omitted updates and existing entrants", func(ctx SpecContext) {
			Expect(event.AllowSelfRegistration).To(BeTrue())
			Expect(event.AllowLateRegistration).To(BeFalse())
			Expect(store.SetGrant(as(ctx, "owner"), event.ID, contest.GrantInput{Username: "entrant", Role: contest.AccessParticipant})).To(Succeed())
			Expect(store.Register(as(ctx, "entrant"), event.ID, users["entrant"])).To(Succeed())
			on, off := true, false
			input := settings()
			input.AllowSelfRegistration, input.AllowLateRegistration = &off, &on
			updated, err := store.Update(as(ctx, "owner"), event.ID, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(updated.AllowSelfRegistration).To(BeFalse())
			Expect(updated.AllowLateRegistration).To(BeTrue())
			updated, err = store.Update(as(ctx, "owner"), event.ID, settings())
			Expect(err).NotTo(HaveOccurred())
			Expect(updated.AllowSelfRegistration).To(BeFalse())
			Expect(updated.AllowLateRegistration).To(BeTrue())
			service := contest.NewService(store, nil)
			Expect(service.Register(as(ctx, "entrant"), event.ID, users["entrant"], "user", "")).To(Succeed())
			access, err := store.Access(as(ctx, "entrant"), event.ID, users["entrant"])
			Expect(err).NotTo(HaveOccurred())
			Expect(access.Registered && access.Permissions.Submit).To(BeTrue())
			Expect(access.Permissions.Register).To(BeFalse())
		})
		It("allows late entry without bypassing passwords, admission or staff restrictions", func(ctx SpecContext) {
			on := true
			input := settings()
			input.BeginAt, input.EndAt = time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
			input.AllowLateRegistration = &on
			input.Visibility = "password"
			hash, err := identity.HashPassword("late-fixture")
			Expect(err).NotTo(HaveOccurred())
			input.PasswordHash = hash
			_, err = store.Update(as(ctx, "owner"), event.ID, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(store.Register(as(ctx, "entrant"), event.ID, users["entrant"], hash)).To(MatchError(contest.ErrForbidden))
			Expect(store.SetGrant(as(ctx, "owner"), event.ID, contest.GrantInput{Username: "entrant", Role: contest.AccessParticipant})).To(Succeed())
			Expect(store.SetGrant(as(ctx, "owner"), event.ID, contest.GrantInput{Username: "observer", Role: contest.AccessObserver})).To(Succeed())
			Expect(store.Register(as(ctx, "observer"), event.ID, users["observer"], hash)).To(MatchError(contest.ErrForbidden))
			passwords, err := identity.NewManager("late-registration-fixture-secret", time.Minute)
			Expect(err).NotTo(HaveOccurred())
			service := contest.NewService(store, passwords)
			Expect(service.Register(as(ctx, "entrant"), event.ID, users["entrant"], "user", "wrong")).To(MatchError(contest.ErrInvalidPassword))
			Expect(service.Register(as(ctx, "entrant"), event.ID, users["entrant"], "user", "late-fixture")).To(Succeed())
			Expect(service.ValidateSubmission(as(ctx, "entrant"), event.ID, users["entrant"], "user", question.ID)).To(Succeed())
			_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE contests SET end_at=now()-interval '1 minute' WHERE id=$1", event.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(store.SetGrant(as(ctx, "owner"), event.ID, contest.GrantInput{Username: "editor", Role: contest.AccessParticipant})).To(Succeed())
			Expect(service.Register(as(ctx, "editor"), event.ID, users["editor"], "user", "late-fixture")).To(MatchError(contest.ErrRegistrationClosed))
		})
		It("rechecks a concurrent registration closure and retains omitted settings under the row lock", func(ctx SpecContext) {
			Expect(store.SetGrant(as(ctx, "owner"), event.ID, contest.GrantInput{Username: "entrant", Role: contest.AccessParticipant})).To(Succeed())
			for _, update := range []bool{false, true} {
				if update {
					_, err := integrationDB.Pool.ExecContext(ctx, "UPDATE contests SET allow_self_registration=true,allow_late_registration=false WHERE id=$1", event.ID)
					Expect(err).NotTo(HaveOccurred())
				}
				blocker, err := integrationDB.Pool.BeginTxx(ctx, nil)
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() { _ = blocker.Rollback() })
				var pid int
				Expect(blocker.GetContext(ctx, &pid, "SELECT pg_backend_pid()")).To(Succeed())
				_, err = blocker.ExecContext(ctx, "UPDATE contests SET allow_self_registration=false,allow_late_registration=true WHERE id=$1", event.ID)
				Expect(err).NotTo(HaveOccurred())
				done := make(chan error, 1)
				jobContext, cancel := context.WithTimeout(ctx, 5*time.Second)
				DeferCleanup(cancel)
				go func() {
					if update {
						_, err := contest.NewService(store, nil).Update(as(jobContext, "owner"), event.ID, contest.UpsertInput{Title: event.Title, BeginAt: event.BeginAt, EndAt: event.EndAt, Rule: event.Rule, Feedback: event.Feedback})
						done <- err
					} else {
						done <- store.Register(as(jobContext, "entrant"), event.ID, users["entrant"])
					}
				}()
				Eventually(func() bool {
					var waiting bool
					err := integrationDB.Pool.GetContext(ctx, &waiting, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND $1=ANY(pg_blocking_pids(pid)))", pid)
					return err == nil && waiting
				}).WithTimeout(3 * time.Second).Should(BeTrue())
				Expect(blocker.Commit()).To(Succeed())
				if update {
					Eventually(done).WithTimeout(3 * time.Second).Should(Receive(BeNil()))
				} else {
					Eventually(done).WithTimeout(3 * time.Second).Should(Receive(MatchError(contest.ErrSelfRegistrationDisabled)))
				}
			}
			stored, err := store.Get(as(ctx, "owner"), event.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(stored.AllowSelfRegistration).To(BeFalse())
			Expect(stored.AllowLateRegistration).To(BeTrue())
			registered, err := store.IsParticipant(as(ctx, "entrant"), event.ID, users["entrant"])
			Expect(err).NotTo(HaveOccurred())
			Expect(registered).To(BeFalse())
		})
		It("round trips explicit false over HTTP and reserves policy changes for owners", func(ctx SpecContext) {
			Expect(store.SetGrant(as(ctx, "owner"), event.ID, contest.GrantInput{Username: "editor", Role: contest.AccessEditor})).To(Succeed())
			Expect(store.SetGrant(as(ctx, "owner"), event.ID, contest.GrantInput{Username: "entrant", Role: contest.AccessParticipant})).To(Succeed())
			handler := contesthandler.NewContestHandler(contest.NewService(store, nil), ratelimit.Policy{})
			auth := middleware.NewAuthMiddleware(contestTestAuth(users))
			router := gin.New()
			handler.RegisterRoutes(router.Group("/api/domains/:domain"), auth.Optional(), auth.Require(), middleware.ResolveDomain(spaces), httpapi.PublicIDs(publicid.NewStore(integrationDB)))
			request := func(method, path, actor string, value any) *httptest.ResponseRecorder {
				body, err := json.Marshal(value)
				Expect(err).NotTo(HaveOccurred())
				base := "/api/domains/team/contests/"
				if method == "PUT" && path == "" {
					base = "/api/domains/team/admin/contests/"
				}
				r := httptest.NewRequest(method, base+event.PublicID+path, strings.NewReader(string(body)))
				r.Header.Set("Authorization", "Bearer "+actor)
				r.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				router.ServeHTTP(response, r)
				return response
			}
			body := map[string]any{"title": event.Title, "beginAt": event.BeginAt, "endAt": event.EndAt, "visibility": event.Visibility, "allowSelfRegistration": false, "allowLateRegistration": true}
			Expect(request("PUT", "", "editor", body).Code).To(Equal(403))
			Expect(request("PUT", "", "owner", body).Code).To(Equal(200))
			response := request("GET", "", "entrant", nil)
			Expect(response.Code).To(Equal(200))
			var details struct {
				Contest map[string]any `json:"contest"`
			}
			Expect(json.Unmarshal(response.Body.Bytes(), &details)).To(Succeed())
			Expect(details.Contest).To(HaveKeyWithValue("allowSelfRegistration", false))
			Expect(details.Contest).To(HaveKeyWithValue("allowLateRegistration", true))
			Expect(details.Contest["permissions"]).To(HaveKeyWithValue("register", false))
			denied := request("POST", "/register", "entrant", map[string]any{})
			Expect(denied.Code).To(Equal(403))
			Expect(denied.Body.String()).To(ContainSubstring("contest.self_registration_disabled"))
			body["allowSelfRegistration"] = "false"
			Expect(request("PUT", "", "owner", body).Code).To(Equal(400))
		})
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

	It("searches and counts only visible contests in the routed domain", func(ctx SpecContext) {
		Expect(store.SetGrant(as(ctx, "owner"), event.ID, contest.GrantInput{Username: "editor", Role: contest.AccessEditor})).To(Succeed())
		other, err := spaces.Create(ctx, users["manager"], domain.CreateInput{Slug: "other", Name: "Other"})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.Create(domain.WithScope(ctx, other), users["manager"], &contest.PersistInput{Title: event.Title, Rule: "icpc", Visibility: "public", Feedback: "full", BeginAt: event.BeginAt, EndAt: event.EndAt})
		Expect(err).NotTo(HaveOccurred())
		items, total, err := store.ListAdmin(as(ctx, "editor"), 20, 0, "Private")
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items[0].ID).To(Equal(event.ID))
		_, total, err = store.ListAdmin(as(ctx, "editor"), 20, 0, "missing")
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(BeZero())
		_, total, err = store.ListAdmin(as(ctx, "editor"), 20, 0, "%")
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(BeZero())
		_, total, err = store.List(as(ctx, "entrant"), 20, 0, event.PublicID)
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(BeZero())
	})

	It("preserves current passwords under lock and permits preparation without password-management rights", func(ctx SpecContext) {
		passwords, err := identity.NewManager("test-secret", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		service := contest.NewService(store, passwords)
		input := contest.UpsertInput{Title: event.Title, Visibility: "password", Password: "old-fixture", Admission: event.Admission, Rule: "icpc", Feedback: "full", BeginAt: event.BeginAt, EndAt: event.EndAt}
		_, err = service.Update(as(ctx, "owner"), event.ID, input)
		Expect(err).NotTo(HaveOccurred())
		Expect(store.SetGrant(as(ctx, "owner"), event.ID, contest.GrantInput{Username: "editor", Role: contest.AccessEditor})).To(Succeed())
		input.Password = ""
		input.Title = "Prepared by editor"
		prepared, err := service.Update(as(ctx, "editor"), event.ID, input)
		Expect(err).NotTo(HaveOccurred())
		Expect(identity.CheckPassword(prepared.PasswordHash, "old-fixture")).To(BeTrue())
		rotated, err := identity.HashPassword("new-fixture")
		Expect(err).NotTo(HaveOccurred())
		rotating := &beforeUpdateStore{ContestStore: store, beforeUpdate: func() {
			_, err := integrationDB.Pool.ExecContext(ctx, "UPDATE contests SET password_hash=$2 WHERE id=$1", event.ID, rotated)
			Expect(err).NotTo(HaveOccurred())
		}}
		prepared, err = contest.NewService(rotating, passwords).Update(as(ctx, "owner"), event.ID, input)
		Expect(err).NotTo(HaveOccurred())
		Expect(prepared.PasswordHash).To(Equal(rotated))
		input.Password = "forbidden-fixture"
		_, err = service.Update(as(ctx, "editor"), event.ID, input)
		Expect(err).To(MatchError(contest.ErrForbidden))
		input.Password = ""
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE contests SET begin_at=now()-interval '1 minute' WHERE id=$1", event.ID)
		Expect(err).NotTo(HaveOccurred())
		_, err = service.Update(as(ctx, "editor"), event.ID, input)
		Expect(err).To(MatchError(contest.ErrForbidden))
		Expect(store.SetProblems(as(ctx, "editor"), event.ID, nil)).To(MatchError(contest.ErrForbidden))
	})

	It("preserves contest history and only deletes unused contests", func(ctx SpecContext) {
		Expect(store.SetGrant(as(ctx, "owner"), event.ID, contest.GrantInput{Username: "entrant", Role: contest.AccessParticipant})).To(Succeed())
		Expect(store.Register(as(ctx, "entrant"), event.ID, users["entrant"])).To(Succeed())
		Expect(store.Delete(as(ctx, "owner"), event.ID)).To(MatchError(contest.ErrInvalidInput))
		registered, err := store.IsParticipant(as(ctx, "owner"), event.ID, users["entrant"])
		Expect(err).NotTo(HaveOccurred())
		Expect(registered).To(BeTrue())
		empty, err := store.Create(as(ctx, "owner"), users["owner"], &contest.PersistInput{Title: "Unused", Rule: "icpc", Visibility: "private", Feedback: "full", BeginAt: event.BeginAt, EndAt: event.EndAt})
		Expect(err).NotTo(HaveOccurred())
		Expect(store.Delete(as(ctx, "owner"), empty.ID)).To(Succeed())
		_, err = store.Get(as(ctx, "owner"), empty.ID)
		Expect(err).To(MatchError(contest.ErrNotFound))
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

type beforeUpdateStore struct {
	*contest.ContestStore
	beforeUpdate func()
}

func (s *beforeUpdateStore) Update(ctx context.Context, id string, input *contest.PersistInput) (*contest.Contest, error) {
	s.beforeUpdate()
	return s.ContestStore.Update(ctx, id, input)
}
