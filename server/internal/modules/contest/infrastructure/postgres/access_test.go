package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	contestapp "github.com/RimuruChan/Vertex/server/internal/modules/contest/application"
	contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	contestpg "github.com/RimuruChan/Vertex/server/internal/modules/contest/infrastructure/postgres"
	contesthttp "github.com/RimuruChan/Vertex/server/internal/modules/contest/transport/http"
	"github.com/RimuruChan/Vertex/server/internal/platform/database/dbtest"
	identityapp "github.com/RimuruChan/Vertex/server/internal/modules/identity/application"
	identitydomain "github.com/RimuruChan/Vertex/server/internal/modules/identity/domain"
	identitypg "github.com/RimuruChan/Vertex/server/internal/modules/identity/infrastructure/postgres"
	identitytoken "github.com/RimuruChan/Vertex/server/internal/modules/identity/infrastructure/token"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/middleware"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
	problemfiles "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/filesystem"
	problempg "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres"
	publicidpg "github.com/RimuruChan/Vertex/server/internal/modules/publicid/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/platform/ratelimit"
	tenancyapp "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/application"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/transport/http"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type contestTestAuth map[string]string

func (a contestTestAuth) Authenticate(_ context.Context, token string) (*identityapp.Identity, error) {
	id, ok := a[token]
	if !ok {
		return nil, errors.New("unknown fixture actor")
	}
	return &identityapp.Identity{User: &identitydomain.User{ID: id, Username: token, Role: "admin"}}, nil
}

var _ = Describe("Contest collaboration against PostgreSQL", func() {
	var spaces *tenancyapp.Service
	var store *contestpg.Repository
	var users map[string]string
	var scope tenancydomain.Scope
	var event *contestdomain.Contest
	var question *problemdomain.Problem
	as := func(ctx context.Context, name string) context.Context {
		return tenancydomain.WithScope(ctx, tenancydomain.Scope{Domain: scope.Domain, UserID: users[name]})
	}
	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		spaces = tenancyapp.NewService(tenancypg.NewRepository(integrationDB))
		store = contestpg.NewRepository(integrationDB)
		users = map[string]string{}
		for _, name := range []string{"manager", "owner", "editor", "jury", "observer", "entrant", "outside"} {
			user, err := identitypg.NewUserRepository(integrationDB).Create(ctx, name, name+"@example.test", "fixture")
			Expect(err).NotTo(HaveOccurred())
			users[name] = user.ID
		}
		var err error
		scope, err = spaces.Create(ctx, users["manager"], tenancydomain.CreateInput{Slug: "team", Name: "Team"})
		Expect(err).NotTo(HaveOccurred())
		for _, name := range []string{"owner", "editor", "jury", "observer", "entrant"} {
			role := "member"
			if name == "owner" {
				role = "author"
			}
			Expect(spaces.SetMember(ctx, "team", users["manager"], tenancydomain.MemberInput{Username: name, RoleKey: role, Status: "active"})).To(Succeed())
		}
		event, err = store.Create(as(ctx, "owner"), users["owner"], &contestdomain.PersistInput{Title: "Private round", Rule: "icpc", Visibility: "private", Admission: contestdomain.AdmissionRestricted, Feedback: "full", BeginAt: time.Now().Add(time.Hour), EndAt: time.Now().Add(2 * time.Hour), RankboardVisible: true})
		Expect(err).NotTo(HaveOccurred())
		question, err = problempg.NewRepository(integrationDB, problemfiles.NewTestdataStorage(GinkgoT().TempDir())).Create(as(ctx, "owner"), users["owner"], &problemdomain.CreateInput{Title: "Hidden task"})
		Expect(err).NotTo(HaveOccurred())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, question.ID)).To(Succeed())
		Expect(store.SetProblems(as(ctx, "owner"), event.ID, []contestdomain.ProblemEntry{{ProblemID: question.ID, Label: "A"}})).To(Succeed())
	})

	Describe("registration policy", func() {
		settings := func() *contestdomain.PersistInput {
			return &contestdomain.PersistInput{Title: event.Title, Rule: event.Rule, Visibility: event.Visibility, Admission: event.Admission, Feedback: event.Feedback, BeginAt: event.BeginAt, EndAt: event.EndAt, RankboardVisible: true}
		}
		It("defaults to pre-start self registration and preserves omitted updates and existing entrants", func(ctx SpecContext) {
			Expect(event.AllowSelfRegistration).To(BeTrue())
			Expect(event.AllowLateRegistration).To(BeFalse())
			Expect(store.SetGrant(as(ctx, "owner"), event.ID, contestdomain.GrantInput{Username: "entrant", Role: contestdomain.AccessParticipant})).To(Succeed())
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
			service := contestapp.NewService(store, nil)
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
			hash, err := identitytoken.HashPassword("late-fixture")
			Expect(err).NotTo(HaveOccurred())
			input.PasswordHash = hash
			_, err = store.Update(as(ctx, "owner"), event.ID, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(store.Register(as(ctx, "entrant"), event.ID, users["entrant"], hash)).To(MatchError(contestdomain.ErrForbidden))
			Expect(store.SetGrant(as(ctx, "owner"), event.ID, contestdomain.GrantInput{Username: "entrant", Role: contestdomain.AccessParticipant})).To(Succeed())
			Expect(store.SetGrant(as(ctx, "owner"), event.ID, contestdomain.GrantInput{Username: "observer", Role: contestdomain.AccessObserver})).To(Succeed())
			Expect(store.Register(as(ctx, "observer"), event.ID, users["observer"], hash)).To(MatchError(contestdomain.ErrForbidden))
			passwords, err := identitytoken.NewManager("late-registration-fixture-secret", time.Minute)
			Expect(err).NotTo(HaveOccurred())
			service := contestapp.NewService(store, passwords)
			Expect(service.Register(as(ctx, "entrant"), event.ID, users["entrant"], "user", "wrong")).To(MatchError(contestdomain.ErrInvalidPassword))
			Expect(service.Register(as(ctx, "entrant"), event.ID, users["entrant"], "user", "late-fixture")).To(Succeed())
			Expect(service.ValidateSubmission(as(ctx, "entrant"), event.ID, users["entrant"], "user", question.ID)).To(Succeed())
			_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE contests SET end_at=now()-interval '1 minute' WHERE id=$1", event.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(store.SetGrant(as(ctx, "owner"), event.ID, contestdomain.GrantInput{Username: "editor", Role: contestdomain.AccessParticipant})).To(Succeed())
			Expect(service.Register(as(ctx, "editor"), event.ID, users["editor"], "user", "late-fixture")).To(MatchError(contestdomain.ErrRegistrationClosed))
		})
		It("rechecks a concurrent registration closure and retains omitted settings under the row lock", func(ctx SpecContext) {
			Expect(store.SetGrant(as(ctx, "owner"), event.ID, contestdomain.GrantInput{Username: "entrant", Role: contestdomain.AccessParticipant})).To(Succeed())
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
						_, err := contestapp.NewService(store, nil).Update(as(jobContext, "owner"), event.ID, contestdomain.UpsertInput{Title: event.Title, BeginAt: event.BeginAt, EndAt: event.EndAt, Rule: event.Rule, Feedback: event.Feedback})
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
					Eventually(done).WithTimeout(3 * time.Second).Should(Receive(MatchError(contestdomain.ErrSelfRegistrationDisabled)))
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
			Expect(store.SetGrant(as(ctx, "owner"), event.ID, contestdomain.GrantInput{Username: "editor", Role: contestdomain.AccessEditor})).To(Succeed())
			Expect(store.SetGrant(as(ctx, "owner"), event.ID, contestdomain.GrantInput{Username: "entrant", Role: contestdomain.AccessParticipant})).To(Succeed())
			handler := contesthttp.NewContestHandler(contestapp.NewService(store, nil), ratelimit.Policy{})
			auth := middleware.NewAuthMiddleware(contestTestAuth(users))
			router := gin.New()
			handler.RegisterRoutes(router.Group("/api/domains/:domain"), auth.Optional(), auth.Require(), middleware.ResolveDomain(spaces), httpapi.PublicIDs(publicidpg.NewResolver(integrationDB)))
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
		for _, role := range []string{contestdomain.AccessEditor, contestdomain.AccessJury, contestdomain.AccessObserver} {
			Expect(store.SetGrant(as(ctx, "owner"), event.ID, contestdomain.GrantInput{Username: role, Role: role})).To(Succeed())
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
		_, err = store.AddStaff(as(ctx, "jury"), event.ID, "entrant", contestdomain.StaffJury)
		Expect(err).To(MatchError(contestdomain.ErrForbidden))
		Expect(store.Register(as(ctx, "observer"), event.ID, users["observer"])).To(MatchError(contestdomain.ErrForbidden))
		Expect(store.Register(as(ctx, "entrant"), event.ID, users["entrant"])).To(MatchError(contestdomain.ErrForbidden))
		Expect(store.SetGrant(as(ctx, "owner"), event.ID, contestdomain.GrantInput{Username: "entrant", Role: contestdomain.AccessParticipant})).To(Succeed())
		Expect(store.Register(as(ctx, "entrant"), event.ID, users["entrant"])).To(Succeed())
		access, err := store.Access(as(ctx, "entrant"), event.ID, users["entrant"])
		Expect(err).NotTo(HaveOccurred())
		Expect(access.Permissions.Submit).To(BeTrue())
		Expect(access.Permissions.PreviewProblems).To(BeFalse())
		service := contestapp.NewService(store, nil)
		_, err = service.Problem(as(ctx, "entrant"), event.ID, "A", users["entrant"], "admin")
		Expect(err).To(MatchError(contestdomain.ErrNotFound))
		preview, err := service.Problem(as(ctx, "editor"), event.ID, "A", users["editor"], "user")
		Expect(err).NotTo(HaveOccurred())
		Expect(preview.Title).To(Equal("Hidden task"))
	})

	It("searches and counts only visible contests in the routed domain", func(ctx SpecContext) {
		Expect(store.SetGrant(as(ctx, "owner"), event.ID, contestdomain.GrantInput{Username: "editor", Role: contestdomain.AccessEditor})).To(Succeed())
		other, err := spaces.Create(ctx, users["manager"], tenancydomain.CreateInput{Slug: "other", Name: "Other"})
		Expect(err).NotTo(HaveOccurred())
		_, err = store.Create(tenancydomain.WithScope(ctx, other), users["manager"], &contestdomain.PersistInput{Title: event.Title, Rule: "icpc", Visibility: "public", Feedback: "full", BeginAt: event.BeginAt, EndAt: event.EndAt})
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
		passwords, err := identitytoken.NewManager("test-secret", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		service := contestapp.NewService(store, passwords)
		input := contestdomain.UpsertInput{Title: event.Title, Visibility: "password", Password: "old-fixture", Admission: event.Admission, Rule: "icpc", Feedback: "full", BeginAt: event.BeginAt, EndAt: event.EndAt}
		_, err = service.Update(as(ctx, "owner"), event.ID, input)
		Expect(err).NotTo(HaveOccurred())
		Expect(store.SetGrant(as(ctx, "owner"), event.ID, contestdomain.GrantInput{Username: "editor", Role: contestdomain.AccessEditor})).To(Succeed())
		input.Password = ""
		input.Title = "Prepared by editor"
		prepared, err := service.Update(as(ctx, "editor"), event.ID, input)
		Expect(err).NotTo(HaveOccurred())
		Expect(identitytoken.CheckPassword(prepared.PasswordHash, "old-fixture")).To(BeTrue())
		rotated, err := identitytoken.HashPassword("new-fixture")
		Expect(err).NotTo(HaveOccurred())
		rotating := &beforeUpdateStore{Repository: store, beforeUpdate: func() {
			_, err := integrationDB.Pool.ExecContext(ctx, "UPDATE contests SET password_hash=$2 WHERE id=$1", event.ID, rotated)
			Expect(err).NotTo(HaveOccurred())
		}}
		prepared, err = contestapp.NewService(rotating, passwords).Update(as(ctx, "owner"), event.ID, input)
		Expect(err).NotTo(HaveOccurred())
		Expect(prepared.PasswordHash).To(Equal(rotated))
		input.Password = "forbidden-fixture"
		_, err = service.Update(as(ctx, "editor"), event.ID, input)
		Expect(err).To(MatchError(contestdomain.ErrForbidden))
		input.Password = ""
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE contests SET begin_at=now()-interval '1 minute' WHERE id=$1", event.ID)
		Expect(err).NotTo(HaveOccurred())
		_, err = service.Update(as(ctx, "editor"), event.ID, input)
		Expect(err).To(MatchError(contestdomain.ErrForbidden))
		Expect(store.SetProblems(as(ctx, "editor"), event.ID, nil)).To(MatchError(contestdomain.ErrForbidden))
	})

	It("preserves contest history and only deletes unused contests", func(ctx SpecContext) {
		Expect(store.SetGrant(as(ctx, "owner"), event.ID, contestdomain.GrantInput{Username: "entrant", Role: contestdomain.AccessParticipant})).To(Succeed())
		Expect(store.Register(as(ctx, "entrant"), event.ID, users["entrant"])).To(Succeed())
		Expect(store.Delete(as(ctx, "owner"), event.ID)).To(MatchError(contestdomain.ErrInvalidInput))
		registered, err := store.IsParticipant(as(ctx, "owner"), event.ID, users["entrant"])
		Expect(err).NotTo(HaveOccurred())
		Expect(registered).To(BeTrue())
		empty, err := store.Create(as(ctx, "owner"), users["owner"], &contestdomain.PersistInput{Title: "Unused", Rule: "icpc", Visibility: "private", Feedback: "full", BeginAt: event.BeginAt, EndAt: event.EndAt})
		Expect(err).NotTo(HaveOccurred())
		Expect(store.Delete(as(ctx, "owner"), empty.ID)).To(Succeed())
		_, err = store.Get(as(ctx, "owner"), empty.ID)
		Expect(err).To(MatchError(contestdomain.ErrNotFound))
	})

	It("keeps group jury grants live only while membership is active", func(ctx SpecContext) {
		group, err := spaces.CreateGroup(ctx, "team", users["manager"], tenancydomain.GroupInput{Name: "Jury"})
		Expect(err).NotTo(HaveOccurred())
		Expect(spaces.SetGroupMember(ctx, "team", users["manager"], group.ID, "jury", "member", false)).To(Succeed())
		Expect(store.SetGrant(as(ctx, "owner"), event.ID, contestdomain.GrantInput{Group: group.ID, Role: contestdomain.AccessJury})).To(Succeed())
		role, err := store.StaffRole(as(ctx, "jury"), event.ID, users["jury"])
		Expect(err).NotTo(HaveOccurred())
		Expect(role).To(Equal(contestdomain.StaffJury))
		Expect(spaces.SetMember(ctx, "team", users["manager"], tenancydomain.MemberInput{Username: "jury", RoleKey: "member", Status: "suspended"})).To(Succeed())
		role, err = store.StaffRole(as(ctx, "owner"), event.ID, users["jury"])
		Expect(err).NotTo(HaveOccurred())
		Expect(role).To(BeEmpty())
		_, err = store.Access(as(ctx, "jury"), event.ID, users["jury"])
		Expect(err).To(MatchError(contestdomain.ErrNotFound))
		Expect(spaces.SetMember(ctx, "team", users["manager"], tenancydomain.MemberInput{Username: "jury", RoleKey: "member", Status: "active"})).To(Succeed())
		Expect(spaces.SetGroupMember(ctx, "team", users["manager"], group.ID, "jury", "member", true)).To(Succeed())
		access, err := store.Access(as(ctx, "jury"), event.ID, users["jury"])
		Expect(err).NotTo(HaveOccurred())
		Expect(access.Permissions.ViewJury).To(BeFalse())
	})

	It("rechecks verified passwords and eligibility under the registration lock", func(ctx SpecContext) {
		oldHash, err := identitytoken.HashPassword("old-password")
		Expect(err).NotTo(HaveOccurred())
		newHash, err := identitytoken.HashPassword("new-password")
		Expect(err).NotTo(HaveOccurred())
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE contests SET visibility='password',admission='members',password_hash=$2 WHERE id=$1", event.ID, newHash)
		Expect(err).NotTo(HaveOccurred())
		Expect(store.Register(as(ctx, "entrant"), event.ID, users["entrant"], oldHash)).To(MatchError(contestdomain.ErrInvalidPassword))
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
		Expect(store.SetGrant(as(ctx, "owner"), event.ID, contestdomain.GrantInput{Username: "outside", Role: contestdomain.AccessJury})).To(MatchError(contestdomain.ErrInvalidInput))
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
			Expect(errors.Is(first, contestdomain.ErrForbidden)).To(BeTrue())
		} else {
			Expect(errors.Is(second, contestdomain.ErrForbidden)).To(BeTrue())
		}
		stored, err := store.Get(as(ctx, "manager"), event.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(*stored.CreatedBy).To(Equal(users["owner"]))
		Expect(stored.OwnerID).NotTo(Equal(users["owner"]))
		Expect(store.Delete(as(ctx, "owner"), event.ID)).To(MatchError(contestdomain.ErrForbidden))
	})

	It("checks current capabilities at HTTP boundaries despite stale role claims", func(ctx SpecContext) {
		service := contestapp.NewService(store, nil)
		handler := contesthttp.NewContestHandler(service, ratelimit.Policy{})
		auth := middleware.NewAuthMiddleware(contestTestAuth(users))
		router := gin.New()
		handler.RegisterRoutes(router.Group("/api/domains/:domain"), auth.Optional(), auth.Require(), middleware.ResolveDomain(spaces), httpapi.PublicIDs(publicidpg.NewResolver(integrationDB)))
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
	*contestpg.Repository
	beforeUpdate func()
}

func (s *beforeUpdateStore) Update(ctx context.Context, id string, input *contestdomain.PersistInput) (*contestdomain.Contest, error) {
	s.beforeUpdate()
	return s.Repository.Update(ctx, id, input)
}
