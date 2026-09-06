package problemset_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/identity"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/RimuruChan/Vertex/server/internal/problem"
	setapp "github.com/RimuruChan/Vertex/server/internal/problemset"
	sethandler "github.com/RimuruChan/Vertex/server/internal/problemset/handler"
	"github.com/RimuruChan/Vertex/server/internal/publicid"
	"github.com/RimuruChan/Vertex/server/internal/transport/httpapi"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type setTestAuth map[string]string

func (a setTestAuth) Authenticate(_ context.Context, token string) (*identity.Identity, error) {
	id, ok := a[token]
	if !ok {
		return nil, errors.New("unknown actor")
	}
	return &identity.Identity{User: &identity.User{ID: id, Username: token, Role: "admin"}}, nil
}

var _ = Describe("Problem set ownership against PostgreSQL", func() {
	var spaces *domain.Service
	var store *setapp.SetStore
	var writer *problem.ProblemAdminStore
	var users map[string]string
	var scope domain.Scope
	var set *setapp.Set
	var task *problem.Problem
	as := func(ctx context.Context, name string) context.Context {
		return domain.WithScope(ctx, domain.Scope{Domain: scope.Domain, UserID: users[name]})
	}
	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		spaces = domain.NewService(domain.NewStore(integrationDB))
		store = setapp.NewSetStore(integrationDB)
		writer = problem.NewProblemAdminStore(integrationDB, GinkgoT().TempDir())
		users = map[string]string{}
		for _, name := range []string{"manager", "curator", "setter", "editor", "reader", "outsider"} {
			user, err := identity.NewUserStore(integrationDB).Create(ctx, name, name+"@example.test", "fixture")
			Expect(err).NotTo(HaveOccurred())
			users[name] = user.ID
		}
		var err error
		scope, err = spaces.Create(ctx, users["manager"], domain.CreateInput{Slug: "team", Name: "Team"})
		Expect(err).NotTo(HaveOccurred())
		for _, name := range []string{"curator", "setter", "editor", "reader"} {
			role := "member"
			if name == "setter" {
				role = "author"
			}
			Expect(spaces.SetMember(ctx, "team", users["manager"], domain.MemberInput{Username: name, RoleKey: role, Status: "active"})).To(Succeed())
		}
		set, err = store.Create(as(ctx, "curator"), users["curator"], setapp.UpsertInput{Title: "Training", Visibility: "private"})
		Expect(err).NotTo(HaveOccurred())
		task, err = writer.Create(as(ctx, "setter"), users["setter"], &problem.CreateInput{Title: "Hidden source", Visibility: "private"})
		Expect(err).NotTo(HaveOccurred())
	})

	It("separates editing from owner actions and does not trust administrator hints", func(ctx SpecContext) {
		Expect(store.SetGrant(as(ctx, "curator"), set.ID, setapp.GrantInput{Username: "editor", Role: setapp.AccessEditor})).To(Succeed())
		item, err := store.Update(as(ctx, "editor"), set.ID, users["editor"], true, setapp.UpsertInput{Title: "Edited", Visibility: "private"})
		Expect(err).NotTo(HaveOccurred())
		Expect(item.Permissions.Edit).To(BeTrue())
		Expect(item.Permissions.Delete).To(BeFalse())
		_, err = store.Update(as(ctx, "editor"), set.ID, users["editor"], true, setapp.UpsertInput{Title: "Publish", Visibility: "public"})
		Expect(err).To(MatchError(setapp.ErrForbidden))
		Expect(store.Delete(as(ctx, "editor"), set.ID, users["editor"])).To(MatchError(setapp.ErrForbidden))
		Expect(store.SetGrant(as(ctx, "editor"), set.ID, setapp.GrantInput{Username: "reader", Role: setapp.AccessEditor})).To(MatchError(setapp.ErrForbidden))
		_, err = store.Get(as(ctx, "reader"), set.ID, users["reader"], true)
		Expect(err).To(MatchError(setapp.ErrNotFound))
		Expect(spaces.SetMember(ctx, "team", users["manager"], domain.MemberInput{Username: "curator", RoleKey: "viewer", Status: "active"})).To(Succeed())
		_, err = store.Create(as(ctx, "curator"), users["curator"], setapp.UpsertInput{Title: "Denied"})
		Expect(err).To(MatchError(setapp.ErrForbidden))
		item, err = store.Update(as(ctx, "curator"), set.ID, users["curator"], false, setapp.UpsertInput{Title: "Still mine", Visibility: "private"})
		Expect(err).NotTo(HaveOccurred())
		Expect(item.OwnerID).To(Equal(users["curator"]))
		Expect(spaces.SetMember(ctx, "team", users["manager"], domain.MemberInput{Username: "curator", RoleKey: "viewer", Status: "suspended"})).To(Succeed())
		_, err = store.Get(as(ctx, "curator"), set.ID, users["curator"], true)
		Expect(err).To(MatchError(setapp.ErrNotFound))
		Expect(spaces.SetMember(ctx, "team", users["manager"], domain.MemberInput{Username: "curator", RoleKey: "viewer", Status: "active"})).To(Succeed())
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE domains SET archived=true WHERE id=$1", scope.Domain.ID)
		Expect(err).NotTo(HaveOccurred())
		item, err = store.Get(as(ctx, "curator"), set.ID, users["curator"], false)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.Permissions.View).To(BeTrue())
		Expect(item.Permissions.Edit).To(BeFalse())
		_, err = store.Update(as(ctx, "curator"), set.ID, users["curator"], true, setapp.UpsertInput{Title: "Archived write", Visibility: "private"})
		Expect(err).To(MatchError(setapp.ErrForbidden))
	})

	It("inherits user and group grants dynamically with the strongest role", func(ctx SpecContext) {
		group, err := spaces.CreateGroup(ctx, "team", users["manager"], domain.GroupInput{Name: "Readers"})
		Expect(err).NotTo(HaveOccurred())
		Expect(spaces.SetGroupMember(ctx, "team", users["manager"], group.ID, "reader", "member", false)).To(Succeed())
		Expect(store.SetGrant(as(ctx, "curator"), set.ID, setapp.GrantInput{Group: group.PublicID, Role: setapp.AccessReader})).To(Succeed())
		Expect(store.SetGrant(as(ctx, "curator"), set.ID, setapp.GrantInput{Username: "reader", Role: setapp.AccessEditor})).To(Succeed())
		item, err := store.Get(as(ctx, "reader"), set.ID, users["reader"], false)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.Permissions.Edit).To(BeTrue())
		grants, err := store.Grants(as(ctx, "curator"), set.ID)
		Expect(err).NotTo(HaveOccurred())
		for _, g := range grants {
			if g.UserID != nil {
				Expect(store.RemoveGrant(as(ctx, "curator"), set.ID, g.ID)).To(Succeed())
			}
		}
		item, err = store.Get(as(ctx, "reader"), set.ID, users["reader"], false)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.Permissions.View).To(BeTrue())
		Expect(item.Permissions.Edit).To(BeFalse())
		Expect(spaces.SetGroupMember(ctx, "team", users["manager"], group.ID, "reader", "member", true)).To(Succeed())
		_, err = store.Get(as(ctx, "reader"), set.ID, users["reader"], true)
		Expect(err).To(MatchError(setapp.ErrNotFound))
		items, total, err := store.List(as(ctx, "reader"), setapp.Filters{ViewerID: users["reader"], Admin: true})
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(BeEmpty())
		Expect(total).To(BeZero())
	})

	It("does not leak private problem metadata or delete invisible existing entries", func(ctx SpecContext) {
		Expect(writer.SetGrant(as(ctx, "setter"), task.ID, problem.GrantInput{Username: "curator", Role: problem.AccessReader})).To(Succeed())
		Expect(store.SetItems(as(ctx, "curator"), set.ID, users["curator"], false, []setapp.ItemInput{{ProblemID: task.ID, Note: "private note"}})).To(Succeed())
		Expect(store.SetGrant(as(ctx, "curator"), set.ID, setapp.GrantInput{Username: "editor", Role: setapp.AccessEditor})).To(Succeed())
		item, err := store.Get(as(ctx, "editor"), set.ID, users["editor"], true)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.Items).To(BeEmpty())
		Expect(item.ProblemCount).To(BeZero())
		Expect(item.Permissions.Edit).To(BeTrue())
		Expect(item.Permissions.EditItems).To(BeFalse())
		Expect(store.SetItems(as(ctx, "editor"), set.ID, users["editor"], true, nil)).To(MatchError(setapp.ErrInvalidInput))
		item, err = store.Get(as(ctx, "curator"), set.ID, users["curator"], false)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.Items).To(HaveLen(1))
		group, err := spaces.CreateGroup(ctx, "team", users["manager"], domain.GroupInput{Name: "Problem reviewers"})
		Expect(err).NotTo(HaveOccurred())
		Expect(spaces.SetGroupMember(ctx, "team", users["manager"], group.ID, "editor", "member", false)).To(Succeed())
		Expect(writer.SetGrant(as(ctx, "setter"), task.ID, problem.GrantInput{Group: group.ID, Role: problem.AccessReader})).To(Succeed())
		item, err = store.Get(as(ctx, "editor"), set.ID, users["editor"], false)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.Items).To(HaveLen(1))
		Expect(item.Permissions.EditItems).To(BeTrue())
		listed, total, err := store.List(as(ctx, "editor"), setapp.Filters{ViewerID: users["editor"]})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(listed[0].ProblemCount).To(Equal(1))
		Expect(spaces.SetGroupMember(ctx, "team", users["manager"], group.ID, "editor", "member", true)).To(Succeed())
		listed, _, err = store.List(as(ctx, "editor"), setapp.Filters{ViewerID: users["editor"]})
		Expect(err).NotTo(HaveOccurred())
		Expect(listed[0].ProblemCount).To(BeZero())
	})

	It("transfers ownership once without preserving creator privileges", func(ctx SpecContext) {
		start := make(chan struct{})
		results := make(chan error, 2)
		for _, name := range []string{"editor", "reader"} {
			go func(target string) { <-start; results <- store.Transfer(as(ctx, "curator"), set.ID, target) }(name)
		}
		close(start)
		var first, second error
		Eventually(results, 5*time.Second).Should(Receive(&first))
		Eventually(results, 5*time.Second).Should(Receive(&second))
		Expect((first == nil) != (second == nil)).To(BeTrue())
		item, err := store.Get(as(ctx, "manager"), set.ID, users["manager"], false)
		Expect(err).NotTo(HaveOccurred())
		Expect(*item.AuthorID).To(Equal(users["curator"]))
		Expect(item.OwnerID).NotTo(Equal(users["curator"]))
		_, err = store.Get(as(ctx, "curator"), set.ID, users["curator"], true)
		Expect(err).To(MatchError(setapp.ErrNotFound))
		Expect(store.Transfer(as(ctx, "manager"), set.ID, "outsider")).To(MatchError(setapp.ErrInvalidInput))
	})

	It("rechecks grants after waiting for a concurrent revocation", func(ctx SpecContext) {
		Expect(store.SetGrant(as(ctx, "curator"), set.ID, setapp.GrantInput{Username: "editor", Role: setapp.AccessEditor})).To(Succeed())
		tx, err := integrationDB.Pool.BeginTxx(ctx, nil)
		Expect(err).NotTo(HaveOccurred())
		defer tx.Rollback()
		var locked string
		Expect(tx.GetContext(ctx, &locked, "SELECT id FROM problem_sets WHERE id=$1 FOR UPDATE", set.ID)).To(Succeed())
		_, err = tx.ExecContext(ctx, "DELETE FROM problem_set_access WHERE set_id=$1", set.ID)
		Expect(err).NotTo(HaveOccurred())
		done := make(chan error, 1)
		go func() {
			_, err := store.Update(as(ctx, "editor"), set.ID, users["editor"], true, setapp.UpsertInput{Title: "Stale", Visibility: "private"})
			done <- err
		}()
		Eventually(func() (int, error) {
			var n int
			err := integrationDB.Pool.GetContext(ctx, &n, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT id,owner_id,visibility FROM problem_sets%'`)
			return n, err
		}, 3*time.Second).Should(BeNumerically(">", 0))
		Expect(tx.Commit()).To(Succeed())
		Eventually(done, 5*time.Second).Should(Receive(MatchError(setapp.ErrNotFound)))
		item, err := store.Get(as(ctx, "curator"), set.ID, users["curator"], false)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.Title).To(Equal("Training"))
	})

	It("uses route scope and current roles over HTTP and rejects foreign grants", func(ctx SpecContext) {
		service := setapp.NewService(store)
		auth := middleware.NewAuthMiddleware(setTestAuth(users))
		router := gin.New()
		sethandler.RegisterRoutes(router.Group("/api/domains/:domain"), sethandler.NewSetHandler(service), auth.Optional(), auth.Require(), middleware.ResolveDomain(spaces), httpapi.PublicIDs(publicid.NewStore(integrationDB)))
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
		path := "team/problem-sets/" + set.PublicID
		Expect(request("GET", path, "reader", "").Code).To(Equal(404))
		Expect(request("PUT", path+"/access", "curator", `{"username":"editor","role":"editor"}`).Code).To(Equal(200))
		response := request("PUT", path, "editor", `{"title":"Updated","visibility":"private","ownerId":"`+users["editor"]+`","domainId":"`+domain.OfficialID+`"}`)
		Expect(response.Code).To(Equal(200))
		Expect(response.Body.String()).To(ContainSubstring(`"ownerId":"` + users["curator"] + `"`))
		Expect(request("DELETE", path, "editor", "").Code).To(Equal(403))
		Expect(request("PUT", path+"/owner", "editor", `{"username":"reader"}`).Code).To(Equal(403))
		Expect(request("GET", "official/problem-sets/"+set.ID, "curator", "").Code).To(Equal(404))
		Expect(request("PUT", path+"/access", "curator", `{"username":"outsider","role":"reader"}`).Code).To(Equal(400))
		// Grant a real site role only for fixture group creation in the official domain.
		_, err := integrationDB.Pool.ExecContext(ctx, "UPDATE users SET role='admin' WHERE id=$1", users["manager"])
		Expect(err).NotTo(HaveOccurred())
		group, err := spaces.CreateGroup(ctx, "official", users["manager"], domain.GroupInput{Name: "Foreign group"})
		Expect(err).NotTo(HaveOccurred())
		Expect(store.SetGrant(as(ctx, "curator"), set.ID, setapp.GrantInput{Group: group.ID, Role: setapp.AccessReader})).To(MatchError(setapp.ErrInvalidInput))
		_, err = integrationDB.Pool.ExecContext(ctx, "INSERT INTO problem_set_access(domain_id,set_id,group_id,role) VALUES($1,$2,$3,'reader')", scope.Domain.ID, set.ID, group.ID)
		var constraint *pgconn.PgError
		Expect(errors.As(err, &constraint)).To(BeTrue())
		Expect(constraint.Code).To(Equal("23503"))
	})
})
