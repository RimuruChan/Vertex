package postgres_test

import (
	"context"
	"errors"
	consoleapp "github.com/RimuruChan/Vertex/server/internal/console/application"
	consoledomain "github.com/RimuruChan/Vertex/server/internal/console/domain"
	consolepg "github.com/RimuruChan/Vertex/server/internal/console/infrastructure/postgres"
	consolehttp "github.com/RimuruChan/Vertex/server/internal/console/transport/http"
	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	identityapp "github.com/RimuruChan/Vertex/server/internal/identity/application"
	identitydomain "github.com/RimuruChan/Vertex/server/internal/identity/domain"
	identitypg "github.com/RimuruChan/Vertex/server/internal/identity/infrastructure/postgres"
	identityhttp "github.com/RimuruChan/Vertex/server/internal/identity/transport/http"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/problem/domain"
	problemfiles "github.com/RimuruChan/Vertex/server/internal/problem/infrastructure/filesystem"
	problempg "github.com/RimuruChan/Vertex/server/internal/problem/infrastructure/postgres"
	publicidpg "github.com/RimuruChan/Vertex/server/internal/publicid/infrastructure/postgres"
	tenancyapp "github.com/RimuruChan/Vertex/server/internal/tenancy/application"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/tenancy/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/transport/httpapi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"io"
	"net/http/httptest"
	"strings"
	"time"
)

var _ = Describe("Domain resource governance against PostgreSQL", func() {
	var store *consolepg.Repository
	var spaces *tenancyapp.Service
	var alpha, beta tenancydomain.Scope
	var users map[string]string
	actor := func(ctx context.Context, space tenancydomain.Scope, user string) context.Context {
		return tenancydomain.WithScope(ctx, tenancydomain.Scope{Domain: space.Domain, UserID: users[user], SiteAdmin: true})
	}
	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		store = consolepg.NewRepository(integrationDB)
		spaces = tenancyapp.NewService(tenancypg.NewRepository(integrationDB))
		users = map[string]string{}
		for _, name := range []string{"owner", "other", "author", "governor"} {
			u, err := identitypg.NewUserRepository(integrationDB).Create(ctx, name, name+"@example.test", "fixture")
			Expect(err).NotTo(HaveOccurred())
			users[name] = u.ID
		}
		var err error
		alpha, err = spaces.Create(ctx, users["owner"], tenancydomain.CreateInput{Slug: "alpha", Name: "Alpha"})
		Expect(err).NotTo(HaveOccurred())
		beta, err = spaces.Create(ctx, users["other"], tenancydomain.CreateInput{Slug: "beta", Name: "Beta"})
		Expect(err).NotTo(HaveOccurred())
		for _, name := range []string{"author", "governor"} {
			role := "author"
			if name == "governor" {
				role = "admin"
			}
			Expect(spaces.SetMember(ctx, "alpha", users["owner"], tenancydomain.MemberInput{Username: name, RoleKey: role, Status: "active"})).To(Succeed())
		}
	})

	It("requires fresh domain rights and keeps notice pagination and numbers local", func(ctx SpecContext) {
		a, err := store.CreateAnnouncement(actor(ctx, alpha, "owner"), users["owner"], consoledomain.AnnouncementInput{Title: "Notice", Published: true})
		Expect(err).NotTo(HaveOccurred())
		Expect(a.PublicID).To(Equal("1"))
		b, err := store.CreateAnnouncement(actor(ctx, beta, "other"), users["other"], consoledomain.AnnouncementInput{Title: "Notice", Published: true})
		Expect(err).NotTo(HaveOccurred())
		Expect(b.PublicID).To(Equal("1"))
		draft, err := store.CreateAnnouncement(actor(ctx, alpha, "owner"), users["owner"], consoledomain.AnnouncementInput{Title: "Private draft"})
		Expect(err).NotTo(HaveOccurred())
		items, total, err := store.AnnouncementPage(actor(ctx, alpha, "owner"), true, consoledomain.AnnouncementFilters{Limit: 1})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items[0].ID).To(Equal(a.ID))
		_, total, err = store.AnnouncementPage(actor(ctx, alpha, "owner"), false, consoledomain.AnnouncementFilters{Limit: 1})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(2))
		_, err = store.Announcement(actor(ctx, alpha, "owner"), draft.ID, false)
		Expect(err).To(MatchError(consoledomain.ErrNotFound))
		_, err = store.Announcement(actor(ctx, alpha, "owner"), b.ID, true)
		Expect(err).To(MatchError(consoledomain.ErrNotFound))
		_, err = store.CreateTag(actor(ctx, alpha, "author"), "No")
		Expect(err).To(MatchError(tenancydomain.ErrForbidden))
		_, _, err = store.AnnouncementPage(actor(ctx, alpha, "author"), false, consoledomain.AnnouncementFilters{Limit: 10})
		Expect(err).To(MatchError(tenancydomain.ErrForbidden))
		resolved, err := publicidpg.NewResolver(integrationDB).Resolve(actor(ctx, alpha, "owner"), "announcements", "1")
		Expect(err).NotTo(HaveOccurred())
		Expect(resolved).To(Equal(a.ID))
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE announcements SET public_id=99 WHERE id=$1", a.ID)
		Expect(err).To(HaveOccurred())
	})

	It("routes domain governance without site-admin privileges and denies unauthorized bodies before parsing", func(ctx SpecContext) {
		auth := middleware.NewAuthMiddleware(resourceActors{users: users})
		router := httpapi.Router(httpapi.Dependencies{Auth: &identityhttp.AuthHandler{}, Health: &httpapi.HealthHandler{},
			Console: consolehttp.NewConsoleHandler(consoleapp.NewService(store)), PublicIDs: publicidpg.NewResolver(integrationDB), ResolveDomain: middleware.ResolveDomain(spaces),
			OptionalAuth: auth.Optional(), RequireAuth: auth.Require(), RequireAdmin: middleware.RequireAdmin(), RequireJudge: auth.Require()})
		request := func(method, path, user string, body io.Reader) *httptest.ResponseRecorder {
			r := httptest.NewRequest(method, path, body)
			if user != "" {
				r.Header.Set("Authorization", "Bearer "+user)
			}
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			return w
		}
		body := &resourceBodyProbe{}
		Expect(request("POST", "/api/domains/alpha/admin/announcements", "author", body).Code).To(Equal(403))
		Expect(body.reads).To(BeZero())
		created := request("POST", "/api/domains/alpha/admin/announcements", "owner", strings.NewReader(`{"title":"Draft secret","published":false,"domainId":"wrong"}`))
		Expect(created.Code).To(Equal(201))
		Expect(created.Body.String()).To(ContainSubstring(`"publicId":"1"`))
		Expect(request("GET", "/api/domains/alpha/announcements/1", "owner", nil).Code).To(Equal(404))
		Expect(request("GET", "/api/domains/alpha/admin/announcements/1", "owner", nil).Code).To(Equal(200))
		Expect(request("GET", "/api/domains/beta/admin/announcements/1", "other", nil).Code).To(Equal(404))
		Expect(request("GET", "/api/domains/alpha/admin/tags", "author", nil).Code).To(Equal(403))
		Expect(request("POST", "/api/domains/alpha/admin/tags", "owner", strings.NewReader(`{"name":"new"}`)).Code).To(Equal(201))
		Expect(request("GET", "/api/domains/alpha/admin/tags", "owner", nil).Body.String()).To(ContainSubstring(`"name":"new"`))
	})

	It("renames and merges working labels without rewriting immutable release tags", func(ctx SpecContext) {
		p, err := problempg.NewRepository(integrationDB, problemfiles.NewTestdataStorage(GinkgoT().TempDir())).Create(actor(ctx, alpha, "owner"), users["owner"], &problemdomain.CreateInput{Title: "Tagged", Tags: []string{"dp", "DP"}})
		Expect(err).NotTo(HaveOccurred())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, p.ID)).To(Succeed())
		var beforeRevision int
		Expect(integrationDB.Pool.QueryRowContext(ctx, "SELECT package_revision FROM problems WHERE id=$1", p.ID).Scan(&beforeRevision)).To(Succeed())
		tags, err := store.ListTags(actor(ctx, alpha, "owner"))
		Expect(err).NotTo(HaveOccurred())
		var source, target int64
		for _, tag := range tags {
			if tag.Name == "dp" {
				source = tag.ID
			} else {
				target = tag.ID
			}
		}
		_, err = store.MergeTags(actor(ctx, alpha, "owner"), source, target)
		Expect(err).NotTo(HaveOccurred())
		working, err := problempg.NewQueries(integrationDB).GetWorkspace(actor(ctx, alpha, "owner"), p.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(working.Tags).To(Equal([]string{"DP"}))
		var frozen string
		Expect(integrationDB.Pool.QueryRowContext(ctx, "SELECT tags_json::text FROM problem_versions WHERE problem_id=$1", p.ID).Scan(&frozen)).To(Succeed())
		Expect(frozen).To(ContainSubstring("dp"))
		var revision int
		Expect(integrationDB.Pool.QueryRowContext(ctx, "SELECT package_revision FROM problems WHERE id=$1", p.ID).Scan(&revision)).To(Succeed())
		Expect(revision).To(Equal(beforeRevision + 1))
		_, err = store.RenameTag(actor(ctx, alpha, "owner"), target, "dynamic")
		Expect(err).NotTo(HaveOccurred())
		Expect(store.DeleteTag(actor(ctx, alpha, "owner"), target)).To(Succeed())
		working, err = problempg.NewQueries(integrationDB).GetWorkspace(actor(ctx, alpha, "owner"), p.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(working.Tags).To(BeEmpty())
	})

	It("waits for an in-flight problem mutation before changing its taxonomy", func(ctx SpecContext) {
		p, err := problempg.NewRepository(integrationDB, problemfiles.NewTestdataStorage(GinkgoT().TempDir())).Create(actor(ctx, alpha, "owner"), users["owner"], &problemdomain.CreateInput{Title: "Guarded", Tags: []string{"old"}})
		Expect(err).NotTo(HaveOccurred())
		tag, err := store.CreateTag(actor(ctx, alpha, "owner"), "old")
		Expect(err).NotTo(HaveOccurred())
		tx, err := integrationDB.Pool.BeginTxx(ctx, nil)
		Expect(err).NotTo(HaveOccurred())
		defer tx.Rollback()
		_, err = problempg.LockAccess(actor(ctx, alpha, "owner"), tx, p.ID, users["owner"])
		Expect(err).NotTo(HaveOccurred())
		result := make(chan error, 1)
		go func() { _, err := store.RenameTag(actor(ctx, alpha, "owner"), tag.ID, "new"); result <- err }()
		Eventually(func() int {
			var n int
			_ = integrationDB.Pool.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity WHERE wait_event_type='Lock' AND query LIKE '%SELECT pg_advisory_xact_lock(%'`).Scan(&n)
			return n
		}, 2*time.Second, 20*time.Millisecond).Should(BeNumerically(">", 0))
		Expect(tx.Commit()).To(Succeed())
		Eventually(result, 2*time.Second).Should(Receive(Succeed()))
		p, err = problempg.NewQueries(integrationDB).GetWorkspace(actor(ctx, alpha, "owner"), p.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(p.Tags).To(Equal([]string{"new"}))
	})

	It("rechecks a queued mutation after member revocation and retains read-only archive access", func(ctx SpecContext) {
		tag, err := store.CreateTag(actor(ctx, alpha, "owner"), "original")
		Expect(err).NotTo(HaveOccurred())
		tx, err := integrationDB.Pool.BeginTxx(ctx, nil)
		Expect(err).NotTo(HaveOccurred())
		defer tx.Rollback()
		_, err = tx.ExecContext(ctx, "SELECT 1 FROM domains WHERE id=$1 FOR UPDATE", alpha.Domain.ID)
		Expect(err).NotTo(HaveOccurred())
		result := make(chan error, 1)
		go func() { _, err := store.RenameTag(actor(ctx, alpha, "governor"), tag.ID, "forbidden"); result <- err }()
		Eventually(func() int {
			var n int
			_ = integrationDB.Pool.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity WHERE wait_event_type='Lock' AND query LIKE '%SELECT 1 FROM domains%'`).Scan(&n)
			return n
		}, 2*time.Second, 20*time.Millisecond).Should(BeNumerically(">", 0))
		_, err = tx.ExecContext(ctx, "UPDATE domain_members SET status='suspended' WHERE domain_id=$1 AND user_id=$2", alpha.Domain.ID, users["governor"])
		Expect(err).NotTo(HaveOccurred())
		Expect(tx.Commit()).To(Succeed())
		Eventually(result, 2*time.Second).Should(Receive(HaveOccurred()))
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE domains SET archived=true WHERE id=$1", alpha.Domain.ID)
		Expect(err).NotTo(HaveOccurred())
		tags, err := store.ListTags(actor(ctx, alpha, "owner"))
		Expect(err).NotTo(HaveOccurred())
		Expect(tags[0].Name).To(Equal("original"))
		Expect(store.DeleteTag(actor(ctx, alpha, "owner"), tag.ID)).To(MatchError(tenancydomain.ErrForbidden))
	})
})

type resourceActors struct{ users map[string]string }

func (a resourceActors) Authenticate(_ context.Context, token string) (*identityapp.Identity, error) {
	id, ok := a.users[token]
	if !ok {
		return nil, errors.New("unknown fixture")
	}
	role := "user"
	if token == "author" {
		role = "admin"
	} // stale global claim must not grant domain governance
	return &identityapp.Identity{User: &identitydomain.User{ID: id, Username: token, Role: role}}, nil
}

type resourceBodyProbe struct{ reads int }

func (b *resourceBodyProbe) Read([]byte) (int, error) { b.reads++; return 0, io.EOF }
