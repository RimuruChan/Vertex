package postgres_test

import (
	"context"
	"encoding/json"
	"fmt"
	authoringdomain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	authoringpg "github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres"
	authoringhandler "github.com/RimuruChan/Vertex/server/internal/modules/authoring/transport/http"
	consoledomain "github.com/RimuruChan/Vertex/server/internal/modules/console/domain"
	consolepg "github.com/RimuruChan/Vertex/server/internal/modules/console/infrastructure/postgres"
	contentapp "github.com/RimuruChan/Vertex/server/internal/modules/content/application"
	contentdomain "github.com/RimuruChan/Vertex/server/internal/modules/content/domain"
	contentpg "github.com/RimuruChan/Vertex/server/internal/modules/content/infrastructure/postgres"
	contenthttp "github.com/RimuruChan/Vertex/server/internal/modules/content/transport/http"
	contestapp "github.com/RimuruChan/Vertex/server/internal/modules/contest/application"
	contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	contestpg "github.com/RimuruChan/Vertex/server/internal/modules/contest/infrastructure/postgres"
	contesthttp "github.com/RimuruChan/Vertex/server/internal/modules/contest/transport/http"
	identitypg "github.com/RimuruChan/Vertex/server/internal/modules/identity/infrastructure/postgres"
	identityhttp "github.com/RimuruChan/Vertex/server/internal/modules/identity/transport/http"
	problemapp "github.com/RimuruChan/Vertex/server/internal/modules/problem/application"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
	problempg "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres"
	problemhttp "github.com/RimuruChan/Vertex/server/internal/modules/problem/transport/http"
	setdomain "github.com/RimuruChan/Vertex/server/internal/modules/problemset/domain"
	setpg "github.com/RimuruChan/Vertex/server/internal/modules/problemset/infrastructure/postgres"
	profilepg "github.com/RimuruChan/Vertex/server/internal/modules/profile/infrastructure/postgres"
	submissiondomain "github.com/RimuruChan/Vertex/server/internal/modules/submission/domain"
	submission "github.com/RimuruChan/Vertex/server/internal/modules/submission/infrastructure/postgres"
	tenancyapp "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/application"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/platform/database/dbtest"
	"github.com/RimuruChan/Vertex/server/internal/platform/ratelimit"
	"github.com/RimuruChan/Vertex/server/internal/transport/http"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/middleware"
	references "github.com/RimuruChan/Vertex/server/internal/transport/http/references"
	evaluationpg "github.com/RimuruChan/Vertex/server/internal/workflows/evaluation/postgres"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"io"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"time"
)

var _ = Describe("Resource domain boundaries against PostgreSQL", func() {
	var scope tenancydomain.Scope
	var owner string
	BeforeEach(func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		user, err := identitypg.NewUserRepository(integrationDB).Create(ctx, "owner", "owner@example.test", "fixture-hash")
		Expect(err).NotTo(HaveOccurred())
		owner = user.ID
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE domain_members SET role_key='author' WHERE user_id=$1 AND domain_id=$2", owner, tenancydomain.OfficialID)
		Expect(err).NotTo(HaveOccurred())
		scope, err = tenancyapp.NewService(tenancypg.NewRepository(integrationDB)).Create(ctx, owner, tenancydomain.CreateInput{Slug: "training", Name: "Training"})
		Expect(err).NotTo(HaveOccurred())
	})

	It("allocates distinct stable numbers within each domain and rejects identity changes", func(spec SpecContext) {
		ctx := tenancydomain.WithScope(spec, tenancydomain.Scope{Domain: tenancydomain.Domain{ID: tenancydomain.OfficialID}, UserID: owner})
		scoped := tenancydomain.WithScope(ctx, scope)
		writer := problempg.NewRepository(integrationDB)
		first, err := writer.Create(ctx, owner, &problemdomain.CreateInput{Title: "Official"})
		Expect(err).NotTo(HaveOccurred())
		second, err := writer.Create(scoped, owner, &problemdomain.CreateInput{Title: "Training"})
		Expect(err).NotTo(HaveOccurred())
		Expect(first.PublicID).To(Equal(second.PublicID))
		Expect(first.ID).NotTo(Equal(second.ID))
		resolver := references.NewResolver(integrationDB)
		resolved, err := resolver.Resolve(ctx, "problems", fmt.Sprint(first.PublicID))
		Expect(err).NotTo(HaveOccurred())
		Expect(resolved).To(Equal(first.ID))
		resolved, err = resolver.Resolve(scoped, "problems", fmt.Sprint(second.PublicID))
		Expect(err).NotTo(HaveOccurred())
		Expect(resolved).To(Equal(second.ID))
		for _, statement := range []string{
			"UPDATE problems SET domain_id=$2 WHERE id=$1",
			"UPDATE problems SET id=$2 WHERE id=$1",
		} {
			_, err := integrationDB.Pool.ExecContext(ctx, statement, first.ID, scope.Domain.ID)
			Expect(err).To(HaveOccurred())
			var pgErr *pgconn.PgError
			Expect(err).To(BeAssignableToTypeOf(pgErr))
			Expect(err.(*pgconn.PgError).Code).To(Equal("23514"))
		}
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE problems SET public_id=public_id+1 WHERE id=$1", first.ID)
		Expect(err).To(HaveOccurred())
		Expect(writer.Delete(scoped, second.ID)).To(Succeed())
		third, err := writer.Create(scoped, owner, &problemdomain.CreateInput{Title: "Not reused"})
		Expect(err).NotTo(HaveOccurred())
		number, err := strconv.ParseInt(second.PublicID, 10, 64)
		Expect(err).NotTo(HaveOccurred())
		Expect(third.PublicID).To(Equal(strconv.FormatInt(number+1, 10)))
	})

	It("serializes concurrent allocation without reusing a committed number", func(spec SpecContext) {
		ctx := tenancydomain.WithScope(spec, tenancydomain.Scope{Domain: tenancydomain.Domain{ID: tenancydomain.OfficialID}, UserID: owner})
		const count = 12
		var ready sync.WaitGroup
		ready.Add(count)
		start := make(chan struct{})
		numbers := make(chan int64, count)
		failures := make(chan error, count)
		for index := 0; index < count; index++ {
			go func(index int) {
				ready.Done()
				<-start
				var number int64
				err := integrationDB.Pool.QueryRowContext(ctx,
					"INSERT INTO problems(domain_id,title,owner_id) VALUES($1,$2,$3) RETURNING public_id",
					scope.Domain.ID, fmt.Sprint(index), owner).Scan(&number)
				numbers <- number
				failures <- err
			}(index)
		}
		ready.Wait()
		close(start)
		seen := map[int64]bool{}
		for index := 0; index < count; index++ {
			Expect(<-failures).To(Succeed())
			seen[<-numbers] = true
		}
		Expect(seen).To(HaveLen(count))
		for number := int64(1000); number < 1000+count; number++ {
			Expect(seen).To(HaveKey(number))
		}
	})

	It("scopes submissions, rejudging, contest staff, clarifications and profile activity", func(spec SpecContext) {
		ctx := tenancydomain.WithScope(spec, tenancydomain.Scope{Domain: tenancydomain.Domain{ID: tenancydomain.OfficialID}, UserID: owner})
		scoped := tenancydomain.WithScope(ctx, scope)
		writer := problempg.NewRepository(integrationDB)
		foreign, err := writer.Create(scoped, owner, &problemdomain.CreateInput{Title: "Training practice", Visibility: "public"})
		Expect(err).NotTo(HaveOccurred())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, foreign.ID)).To(Succeed())
		contests := contestpg.NewRepository(integrationDB)
		competition, err := contests.Create(scoped, owner, &contestdomain.PersistInput{Title: "Training round", Rule: "icpc", BeginAt: time.Now().Add(-time.Hour), EndAt: time.Now().Add(time.Hour), Visibility: "public", Feedback: "full"})
		Expect(err).NotTo(HaveOccurred())
		Expect(contests.SetProblems(scoped, competition.ID, []contestdomain.ProblemEntry{{ProblemID: foreign.ID, Label: "A"}})).To(Succeed())
		_, total, err := contests.ListAdmin(ctx, 20, 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(BeZero())
		_, err = contests.Get(ctx, competition.ID)
		Expect(err).To(MatchError(contestdomain.ErrNotFound))
		Expect(contests.SetProblems(ctx, competition.ID, nil)).To(MatchError(contestdomain.ErrNotFound))
		Expect(contests.Register(ctx, competition.ID, owner)).To(MatchError(contestdomain.ErrNotFound))
		_, err = contests.AddStaff(scoped, competition.ID, "owner", "jury")
		Expect(err).NotTo(HaveOccurred())
		role, err := contests.StaffRole(ctx, competition.ID, owner)
		Expect(err).NotTo(HaveOccurred())
		Expect(role).To(BeEmpty())
		staff, err := contests.ListStaff(ctx, competition.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(staff).To(BeEmpty())
		message, err := contests.CreateClarification(scoped, contestdomain.ClarificationInput{ContestID: competition.ID, AuthorID: owner, Body: "Private-domain broadcast", FromJury: true})
		Expect(err).NotTo(HaveOccurred())
		_, err = contests.GetClarification(ctx, competition.ID, message.ID)
		Expect(err).To(MatchError(contestdomain.ErrClarificationNotFound))
		messages, err := contests.ListClarifications(ctx, competition.ID, contestdomain.Viewer{UserID: owner, Role: "admin"})
		Expect(err).NotTo(HaveOccurred())
		Expect(messages).To(BeEmpty())

		submissions := submission.NewRepository(integrationDB, evaluationpg.Rebuild)
		input := &submissiondomain.Submission{UserID: owner, ProblemID: foreign.ID, Language: "cpp", SourceCode: "private code"}
		_, err = submissions.Create(ctx, input)
		Expect(err).To(MatchError(submissiondomain.ErrNotFound))
		created, err := submissions.Create(scoped, input)
		Expect(err).NotTo(HaveOccurred())
		_, total, err = submissions.List(ctx, submissiondomain.Filters{}, submissiondomain.Viewer{})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(BeZero())
		_, err = submissions.Get(ctx, created.ID, submissiondomain.Viewer{UserID: owner})
		Expect(err).To(MatchError(submissiondomain.ErrNotFound))
		_, err = submissions.Progress(ctx, created.ID, submissiondomain.Viewer{})
		Expect(err).To(MatchError(submissiondomain.ErrNotFound))
		Expect(submissions.Rejudge(ctx, created.ID)).To(MatchError(submissiondomain.ErrNotFound))
		var state string
		Expect(integrationDB.Pool.GetContext(ctx, &state, "SELECT state FROM judge_jobs WHERE submission_id=$1", created.ID)).To(Succeed())
		Expect(state).To(Equal("queued"))
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE judgements j SET judged_at=now(),status='Accepted' FROM submissions s WHERE j.submission_id=s.id AND j.generation=s.result_generation AND s.id=$1", created.ID)
		Expect(err).NotTo(HaveOccurred())
		batch, err := submissions.CreateRejudging(scoped, submissiondomain.RejudgeSelector{SubmissionIDs: []string{created.ID}}, owner)
		Expect(err).NotTo(HaveOccurred())
		_, err = submissions.Rejudging(ctx, batch.ID)
		Expect(err).To(MatchError(submissiondomain.ErrRejudgeNotFound))
		Expect(submissions.CancelRejudging(ctx, batch.ID)).To(MatchError(submissiondomain.ErrRejudgeNotFound))
		batches, err := submissions.ListRejudgings(ctx, "", 20)
		Expect(err).NotTo(HaveOccurred())
		Expect(batches).To(BeEmpty())
		changes, err := submissions.RejudgingChanges(ctx, batch.ID, 20)
		Expect(err).To(MatchError(submissiondomain.ErrRejudgeNotFound))
		Expect(changes).To(BeEmpty())
		changes, err = submissions.RejudgingChanges(scoped, batch.ID, 20)
		Expect(err).NotTo(HaveOccurred())
		Expect(changes).To(HaveLen(1))
		account, err := profilepg.NewQueries(integrationDB).ByUsername(ctx, "owner")
		Expect(err).NotTo(HaveOccurred())
		Expect(account.SubmissionCount).To(BeZero())
		Expect(account.Activity).To(BeEmpty())
		Expect(account.ByDifficulty).To(BeEmpty())
	})

	It("does not expose authoring sources or build control across domains while internal workers can claim them", func(spec SpecContext) {
		ctx := tenancydomain.WithScope(spec, tenancydomain.Scope{Domain: tenancydomain.Domain{ID: tenancydomain.OfficialID}, UserID: owner})
		scoped := tenancydomain.WithScope(ctx, scope)
		writer := problempg.NewRepository(integrationDB)
		foreign, err := writer.Create(scoped, owner, &problemdomain.CreateInput{Title: "Unreleased package", StatementMD: "Fixture statement"})
		Expect(err).NotTo(HaveOccurred())
		fixture := newAuthoringFixture(scoped, foreign.ID, GinkgoT().TempDir())
		_, err = fixture.repo.WorkingCopy(ctx, foreign.ID)
		Expect(err).To(MatchError(authoringdomain.ErrNotFound))
		_, err = fixture.service.Material(ctx, foreign.ID, "reference", 0)
		Expect(err).To(MatchError(authoringdomain.ErrNotFound))
		_, err = fixture.service.DeleteEntry(ctx, foreign.ID, fixture.copy.ETag, "main")
		Expect(err).To(MatchError(authoringdomain.ErrNotFound))
		build, err := fixture.repo.StartCheck(scoped, foreign.ID, authoringdomain.CheckSelection{ETag: fixture.copy.ETag})
		Expect(err).NotTo(HaveOccurred())
		_, err = fixture.repo.Check(ctx, foreign.ID, build.ID)
		Expect(err).To(MatchError(authoringdomain.ErrNotFound))
		_, err = fixture.repo.CancelCheck(ctx, foreign.ID, build.ID)
		Expect(err).To(MatchError(authoringdomain.ErrNotFound))
		builds := authoringpg.NewBuildRepository(integrationDB)
		workerCtx := authoringdomain.WithCheckProtocol(ctx, authoringdomain.CheckPolicyVersion)
		claimed, pkg, err := builds.Claim(workerCtx, "domain-fixture-worker", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed.ID).To(Equal(build.ID))
		Expect(pkg.ProblemID).To(Equal(foreign.ID))
		Expect(pkg.Check.Programs).To(HaveLen(1))
		for digest, expected := range map[string]string{pkg.Check.Programs[0].Files[0].Blob.SHA256: "private source", pkg.Check.Tests[0].Input.SHA256: "hidden input"} {
			_, reader, err := fixture.repo.CheckContent(ctx, claimed.ID, "domain-fixture-worker", claimed.LeaseToken, digest)
			Expect(err).NotTo(HaveOccurred())
			data, err := io.ReadAll(reader)
			Expect(reader.Close()).To(Succeed())
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(Equal(expected))
		}

	})

	It("keeps tag management and announcements within the requested domain", func(spec SpecContext) {
		ctx := tenancydomain.WithScope(spec, tenancydomain.Scope{Domain: tenancydomain.Domain{ID: tenancydomain.OfficialID}, UserID: owner})
		scoped := tenancydomain.WithScope(ctx, scope)
		writer := problempg.NewRepository(integrationDB)
		_, err := writer.Create(ctx, owner, &problemdomain.CreateInput{Title: "Official", Tags: []string{"shared"}})
		Expect(err).NotTo(HaveOccurred())
		_, err = writer.Create(scoped, owner, &problemdomain.CreateInput{Title: "Training", Tags: []string{"shared", "training"}})
		Expect(err).NotTo(HaveOccurred())
		Expect(dbtest.PublishedProblems(ctx, integrationDB)).To(Succeed())
		store := consolepg.NewRepository(integrationDB)
		_, err = store.ListTags(ctx)
		Expect(err).To(MatchError(tenancydomain.ErrForbidden))
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE domain_members SET role_key='admin' WHERE domain_id=$1 AND user_id=$2", tenancydomain.OfficialID, owner)
		Expect(err).NotTo(HaveOccurred())
		localTags, err := store.ListTags(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(localTags).To(HaveLen(1))
		foreignTags, err := store.ListTags(scoped)
		Expect(err).NotTo(HaveOccurred())
		Expect(foreignTags).To(HaveLen(2))
		_, err = store.MergeTags(ctx, foreignTags[0].ID, localTags[0].ID)
		Expect(err).To(MatchError(consoledomain.ErrNotFound))
		_, err = store.MergeTags(ctx, localTags[0].ID, foreignTags[0].ID)
		Expect(err).To(MatchError(consoledomain.ErrNotFound))
		_, err = store.RenameTag(ctx, foreignTags[0].ID, "wrong scope")
		Expect(err).To(MatchError(consoledomain.ErrNotFound))
		announcement, err := store.CreateAnnouncement(scoped, owner, consoledomain.AnnouncementInput{Title: "Training news", Published: true})
		Expect(err).NotTo(HaveOccurred())
		announcements, err := store.ListAnnouncements(ctx, true, 20)
		Expect(err).NotTo(HaveOccurred())
		Expect(announcements).To(BeEmpty())
		Expect(store.DeleteAnnouncement(ctx, announcement.ID)).To(MatchError(consoledomain.ErrNotFound))
	})

	It("scopes root lists, counts, tags, details and writes even for administrators", func(spec SpecContext) {
		ctx := tenancydomain.WithScope(spec, tenancydomain.Scope{Domain: tenancydomain.Domain{ID: tenancydomain.OfficialID}, UserID: owner})
		scoped := tenancydomain.WithScope(ctx, scope)
		writer := problempg.NewRepository(integrationDB)
		reader := problempg.NewQueries(integrationDB)
		foreign, err := writer.Create(scoped, owner, &problemdomain.CreateInput{Title: "Training secret", Tags: []string{"same tag"}})
		Expect(err).NotTo(HaveOccurred())
		local, err := writer.Create(ctx, owner, &problemdomain.CreateInput{Title: "Official", Visibility: "public", Tags: []string{"same tag"}})
		Expect(err).NotTo(HaveOccurred())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, local.ID, foreign.ID)).To(Succeed())
		items, total, err := reader.List(ctx, problemdomain.Filters{})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items).To(HaveLen(1))
		Expect(items[0].ID).To(Equal(local.ID))
		_, err = reader.Get(ctx, foreign.ID)
		Expect(err).To(MatchError(problemdomain.ErrNotFound))
		Expect(writer.Delete(ctx, foreign.ID)).To(MatchError(problemdomain.ErrNotFound))
		allowed, err := contentpg.NewProblemAccess(integrationDB).CanViewProblem(ctx, foreign.ID, owner)
		Expect(err).NotTo(HaveOccurred())
		Expect(allowed).To(BeFalse())
		var distinctTags int
		Expect(integrationDB.Pool.GetContext(ctx, &distinctTags, "SELECT count(*) FROM tags WHERE name='same tag'")).To(Succeed())
		Expect(distinctTags).To(Equal(2))

		sets := setpg.NewRepository(integrationDB)
		set, err := sets.Create(scoped, owner, setdomain.UpsertInput{Title: "Private-domain set"})
		Expect(err).NotTo(HaveOccurred())
		_, total, err = sets.List(ctx, setdomain.Filters{Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(BeZero())
		_, err = sets.Get(ctx, set.ID, owner)
		Expect(err).To(MatchError(setdomain.ErrNotFound))
		Expect(sets.SetItems(ctx, set.ID, owner, nil)).To(MatchError(setdomain.ErrNotFound))
		Expect(sets.SetItems(scoped, set.ID, owner, []setdomain.ItemInput{{ProblemID: local.ID}})).To(HaveOccurred())
		Expect(sets.SetItems(scoped, set.ID, owner, []setdomain.ItemInput{{ProblemID: foreign.ID}})).To(Succeed())

		editorials := contentpg.NewEditorialRepository(integrationDB)
		editorial, err := editorials.Create(scoped, owner, contentdomain.EditorialInput{ProblemID: foreign.ID, Title: "Private solution", ContentMD: "private solution body", Visibility: "public", Status: "published"})
		Expect(err).NotTo(HaveOccurred())
		_, total, err = editorials.List(ctx, contentdomain.EditorialFilters{Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(BeZero())
		_, err = editorials.Get(ctx, editorial.ID, owner)
		Expect(err).To(MatchError(contentdomain.ErrNotFound))
		_, err = editorials.Vote(ctx, editorial.ID, owner, true)
		Expect(err).To(MatchError(contentdomain.ErrNotFound))

		posts := contentpg.NewDiscussionRepository(integrationDB)
		post, err := posts.CreateProblemPost(scoped, foreign.ID, owner, "Private discussion", nil)
		Expect(err).NotTo(HaveOccurred())
		_, err = posts.Get(ctx, post.ID, owner)
		Expect(err).To(MatchError(contentdomain.ErrNotFound))
		_, err = posts.Update(ctx, post.ID, owner, "Tampered")
		Expect(err).To(MatchError(contentdomain.ErrNotFound))
		list, err := posts.ListByProblem(ctx, foreign.ID, owner)
		Expect(err).To(MatchError(contentdomain.ErrNotFound))
		Expect(list.Posts).To(BeEmpty())
	})
})

var _ = Describe("HTTP resource scope", func() {
	It("binds the path domain before number resolution and rechecks suspended membership", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		users := map[string]string{}
		for _, name := range []string{"owner", "member", "outsider"} {
			user, err := identitypg.NewUserRepository(integrationDB).Create(ctx, name, name+"@example.test", "fixture-hash")
			Expect(err).NotTo(HaveOccurred())
			users[name] = user.ID
		}
		service := tenancyapp.NewService(tenancypg.NewRepository(integrationDB))
		_, err := integrationDB.Pool.ExecContext(ctx, "UPDATE domain_members SET role_key='author' WHERE user_id=$1 AND domain_id=$2", users["owner"], tenancydomain.OfficialID)
		Expect(err).NotTo(HaveOccurred())
		scope, err := service.Create(ctx, users["owner"], tenancydomain.CreateInput{Slug: "training", Name: "Training"})
		Expect(err).NotTo(HaveOccurred())
		Expect(service.SetMember(ctx, "training", users["owner"], tenancydomain.MemberInput{Username: "member", RoleKey: "member", Status: "active"})).To(Succeed())
		writer := problempg.NewRepository(integrationDB)
		local, err := writer.Create(ctx, users["owner"], &problemdomain.CreateInput{Title: "Official exercise", Visibility: "public"})
		Expect(err).NotTo(HaveOccurred())
		foreign, err := writer.Create(tenancydomain.WithScope(ctx, scope), users["owner"], &problemdomain.CreateInput{Title: "Training exercise", Visibility: "public"})
		Expect(err).NotTo(HaveOccurred())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, local.ID, foreign.ID)).To(Succeed())
		handler := problemhttp.NewProblemHandler(problemapp.NewService(problempg.NewQueries(integrationDB), writer), nil)
		auth := middleware.NewAuthMiddleware(staleRoleAuthenticator{users: users})
		// Exercise production composition, not a test-only scoped route group.
		router := httpapi.Router(httpapi.Dependencies{
			Auth: &identityhttp.AuthHandler{}, Health: &httpapi.HealthHandler{},
			Problems: handler, ResourceReferences: references.NewResolver(integrationDB), ResolveDomain: middleware.ResolveDomain(service),
			OptionalAuth: auth.Optional(), RequireAuth: auth.Require(), RequireAdmin: middleware.RequireAdmin(), RequireJudge: auth.Require(),
		})
		request := func(path, token string) *httptest.ResponseRecorder {
			r := httptest.NewRequest("GET", path, nil)
			if token != "" {
				r.Header.Set("Authorization", "Bearer "+token)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, r)
			return response
		}
		response := request("/api/domains/official/problems/"+local.PublicID+"?domain=training", "owner")
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
		Expect(service.SetMember(ctx, "training", users["owner"], tenancydomain.MemberInput{Username: "member", RoleKey: "member", Status: "suspended"})).To(Succeed())
		Expect(request("/api/domains/training/problems/"+foreign.PublicID, "member").Code).To(Equal(404))
		// The path also determines the scope for non-resource-number endpoints.
		Expect(request("/api/domains/training/tags", "member").Code).To(Equal(404))
		for _, path := range []string{"problem-sets", "contests", "submissions", "editorials", "announcements", "users/member", "admin/problems", "admin/contests", "admin/problems/" + foreign.ID + "/package"} {
			Expect(request("/api/domains/training/"+path, "member").Code).To(Equal(404), path)
		}
	})
})

var _ = Describe("Content HTTP domain boundaries", func() {
	It("uses parent and domain capabilities instead of stale global role claims", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		users := map[string]string{}
		for _, name := range []string{"manager", "author", "reader"} {
			u, err := identitypg.NewUserRepository(integrationDB).Create(ctx, name, name+"@example.test", "fixture")
			Expect(err).NotTo(HaveOccurred())
			users[name] = u.ID
		}
		spaces := tenancyapp.NewService(tenancypg.NewRepository(integrationDB))
		scope, err := spaces.Create(ctx, users["manager"], tenancydomain.CreateInput{Slug: "team", Name: "Team"})
		Expect(err).NotTo(HaveOccurred())
		for _, name := range []string{"author", "reader"} {
			Expect(spaces.SetMember(ctx, "team", users["manager"], tenancydomain.MemberInput{Username: name, RoleKey: "member", Status: "active"})).To(Succeed())
		}
		as := func(name string) context.Context {
			return tenancydomain.WithScope(ctx, tenancydomain.Scope{Domain: scope.Domain, UserID: users[name]})
		}
		writer := problempg.NewRepository(integrationDB)
		task, err := writer.Create(as("manager"), users["manager"], &problemdomain.CreateInput{Title: "HTTP parent", Visibility: "public"})
		Expect(err).NotTo(HaveOccurred())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, task.ID)).To(Succeed())
		editorials := contentpg.NewEditorialRepository(integrationDB)
		item, err := editorials.Create(as("author"), users["author"], contentdomain.EditorialInput{ProblemID: task.ID, Title: "HTTP solution", ContentMD: "protected solution"})
		Expect(err).NotTo(HaveOccurred())
		service := contentapp.NewService(editorials, contentpg.NewDiscussionRepository(integrationDB), contentpg.NewProblemAccess(integrationDB))
		auth := middleware.NewAuthMiddleware(staleRoleAuthenticator{users: users})
		router := gin.New()
		contenthttp.RegisterRoutes(router.Group("/api/domains/:domain"), contenthttp.NewEditorialHandler(service), contenthttp.NewDiscussionHandler(service), auth.Optional(), auth.Require(), middleware.ResolveDomain(spaces), httpapi.ResourceReferences(references.NewResolver(integrationDB)))
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
		path := "team/editorials/" + item.PublicID
		response := request("GET", path, "manager", "")
		Expect(response.Code).To(Equal(200))
		Expect(response.Body.String()).To(ContainSubstring(`"edit":false`))
		Expect(response.Body.String()).To(ContainSubstring(`"delete":true`))
		Expect(request("PUT", path, "manager", `{"title":"Tampered","contentMd":"no"}`).Code).To(Equal(403))
		Expect(request("DELETE", path, "reader", "").Code).To(Equal(403))
		response = request("GET", "team/editorials", "reader", "")
		Expect(response.Code).To(Equal(200))
		Expect(response.Body.String()).NotTo(ContainSubstring("contentMd"))
		Expect(request("GET", "official/editorials/"+item.ID, "manager", "").Code).To(Equal(404))
		response = request("POST", "team/problems/"+task.PublicID+"/discussions", "author", `{"contentMd":"comment","domainId":"`+tenancydomain.OfficialID+`","authorId":"`+users["reader"]+`"}`)
		Expect(response.Code).To(Equal(201))
		var post struct {
			ID       int64  `json:"id"`
			DomainID string `json:"domainId"`
			AuthorID string `json:"authorId"`
		}
		Expect(json.Unmarshal(response.Body.Bytes(), &post)).To(Succeed())
		Expect(post.DomainID).To(Equal(scope.Domain.ID))
		Expect(post.AuthorID).To(Equal(users["author"]))
		Expect(spaces.SetMember(ctx, "team", users["manager"], tenancydomain.MemberInput{Username: "reader", RoleKey: "viewer", Status: "active"})).To(Succeed())
		response = request("GET", "team/problems/"+task.PublicID+"/discussions", "reader", "")
		Expect(response.Code).To(Equal(200))
		Expect(response.Body.String()).To(ContainSubstring(`"canPost":false`))
		Expect(request("POST", "team/problems/"+task.PublicID+"/discussions", "reader", `{"contentMd":"not allowed"}`).Code).To(Equal(403))
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE problems SET visibility='private' WHERE id=$1", task.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(request("GET", path, "author", "").Code).To(Equal(404))
		Expect(request("PUT", path, "author", `{"title":"Stale author","contentMd":"no"}`).Code).To(Equal(404))
		Expect(writer.SetGrant(as("manager"), task.ID, problemdomain.GrantInput{Username: "author", Role: problemdomain.AccessReader})).To(Succeed())
		Expect(request("PUT", path, "author", `{"title":"Restored","contentMd":"yes"}`).Code).To(Equal(200))
		for _, method := range []string{"GET", "POST"} {
			Expect(request(method, "team/contests/1/discussions", "manager", `{"contentMd":"use clarifications"}`).Code).To(Equal(404))
		}
	})
})

var _ = Describe("Publication HTTP boundaries", func() {
	It("requires the reviewed input and current resource rights for publication and contest adoption", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		users := map[string]string{}
		for _, name := range []string{"owner", "editor", "observer"} {
			u, err := identitypg.NewUserRepository(integrationDB).Create(ctx, name, name+"@example.test", "fixture")
			Expect(err).NotTo(HaveOccurred())
			users[name] = u.ID
		}
		spaces := tenancyapp.NewService(tenancypg.NewRepository(integrationDB))
		scope, err := spaces.Create(ctx, users["owner"], tenancydomain.CreateInput{Slug: "publishing", Name: "Publishing"})
		Expect(err).NotTo(HaveOccurred())
		for _, name := range []string{"editor", "observer"} {
			Expect(spaces.SetMember(ctx, "publishing", users["owner"], tenancydomain.MemberInput{Username: name, RoleKey: "member", Status: "active"})).To(Succeed())
		}
		as := func(actor string) context.Context {
			return tenancydomain.WithScope(ctx, tenancydomain.Scope{Domain: scope.Domain, UserID: users[actor]})
		}
		root := GinkgoT().TempDir()
		writer := problempg.NewRepository(integrationDB)
		task, err := writer.Create(as("owner"), users["owner"], &problemdomain.CreateInput{Title: "Reviewed task", StatementMD: "First statement", Visibility: "public"})
		Expect(err).NotTo(HaveOccurred())
		Expect(writer.SetGrant(as("owner"), task.ID, problemdomain.GrantInput{Username: "editor", Role: problemdomain.AccessEditor})).To(Succeed())
		fixture := newAuthoringFixture(as("owner"), task.ID, root)
		checkID := fixture.checked()
		revision := fixture.commit("first")
		contests := contestpg.NewRepository(integrationDB)
		auth := middleware.NewAuthMiddleware(staleRoleAuthenticator{users: users})
		router := gin.New()
		api := router.Group("/api/domains/:domain")
		resolve := middleware.ResolveDomain(spaces)
		numbers := httpapi.ResourceReferences(references.NewResolver(integrationDB))
		authoringhandler.NewWorkbenchHandler(fixture.service, 64<<20).RegisterRoutes(api, auth.Require(), resolve, numbers)
		contesthttp.NewContestHandler(contestapp.NewService(contests, nil), ratelimit.Policy{}, nil).RegisterRoutes(api, auth.Optional(), auth.Require(), resolve, numbers)
		request := func(method, route, actor, body string) *httptest.ResponseRecorder {
			r := httptest.NewRequest(method, "/api/domains/"+route, strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			if actor != "" {
				r.Header.Set("Authorization", "Bearer "+actor)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, r)
			return response
		}
		route := "publishing/authoring/problems/" + task.PublicID
		for _, body := range []string{`{}`, `{"revision":null}`, `{"revision":-1}`, `{"revision":0}`} {
			Expect(request("POST", route+"/releases", "owner", body).Code).To(Equal(400), body)
		}
		input := fmt.Sprintf(`{"revision":%d,"checkId":"%s","expectedVersion":0}`, revision, checkID)
		Expect(request("POST", route+"/releases", "", input).Code).To(Equal(401))
		Expect(request("POST", route+"/releases", "owner", `{"language":"`+strings.Repeat("x", 65<<10)+`"}`).Code).To(Equal(413))
		Expect(request("POST", route+"/releases", "editor", input).Code).To(Equal(403))
		Expect(request("POST", "official/authoring/problems/"+task.ID+"/releases", "owner", input).Code).To(Equal(404))
		response := request("POST", route+"/releases", "owner", input)
		Expect(response.Code).To(Equal(200), response.Body.String())
		var release struct {
			Version int `json:"version"`
		}
		Expect(json.Unmarshal(response.Body.Bytes(), &release)).To(Succeed())
		Expect(release.Version).To(Equal(1))
		Expect(request("GET", route+"/releases", "editor", "").Code).To(Equal(200))

		event, err := contests.Create(as("owner"), users["owner"], &contestdomain.PersistInput{Title: "Pinned", Rule: "icpc", Visibility: "private", Feedback: "full", BeginAt: time.Now().Add(time.Hour), EndAt: time.Now().Add(2 * time.Hour)})
		Expect(err).NotTo(HaveOccurred())
		Expect(contests.SetProblems(as("owner"), event.ID, []contestdomain.ProblemEntry{{ProblemID: task.ID, Label: "A", Points: 100}})).To(Succeed())
		Expect(contests.SetGrant(as("owner"), event.ID, contestdomain.GrantInput{Username: "observer", Role: contestdomain.AccessObserver})).To(Succeed())
		// A collaborator commits a separate copy; an old publication remains idempotent.
		fixture.ctx = as("editor")
		fixture.copy, err = fixture.repo.Open(fixture.ctx, task.ID)
		Expect(err).NotTo(HaveOccurred())
		material, err := fixture.service.Material(fixture.ctx, task.ID, "problem", 0)
		Expect(err).NotTo(HaveOccurred())
		material.Metadata.Title = "Reviewed v2"
		fixture.document("problem", "vertex/problem.json", authoringdomain.EntryMetadata, material.Metadata)
		revision = fixture.commit("second")
		Expect(request("POST", route+"/releases", "owner", input).Code).To(Equal(200))
		input = fmt.Sprintf(`{"revision":%d,"checkId":"%s","expectedVersion":0}`, revision, checkID)
		Expect(request("POST", route+"/releases", "owner", input).Code).To(Equal(409))
		input = fmt.Sprintf(`{"revision":%d,"checkId":"%s","expectedVersion":1}`, revision, checkID)
		Expect(request("POST", route+"/releases", "owner", input).Code).To(Equal(200))
		adopt := "publishing/contests/" + event.PublicID + "/problems/A/version"
		Expect(request("PUT", adopt, "owner", `{"version":2}`).Code).To(Equal(400))
		Expect(request("PUT", adopt, "observer", `{"version":2,"expectedVersion":1}`).Code).To(Equal(403))
		Expect(request("PUT", adopt, "owner", `{"version":2,"expectedVersion":1}`).Code).To(Equal(200))
		Expect(request("PUT", adopt, "owner", `{"version":1,"expectedVersion":1}`).Code).To(Equal(409))
		Expect(request("PUT", adopt, "owner", `{"version":99,"expectedVersion":2}`).Code).To(Equal(400))
		pinned, err := contests.Problem(as("owner"), event.ID, "A")
		Expect(err).NotTo(HaveOccurred())
		Expect(pinned.Version).To(Equal(2))
		Expect(pinned.Title).To(Equal("Reviewed v2"))
		Expect(spaces.SetMember(ctx, "publishing", users["owner"], tenancydomain.MemberInput{Username: "editor", RoleKey: "member", Status: "suspended"})).To(Succeed())
		Expect(request("GET", route+"/releases", "editor", "").Code).To(Equal(404))
	})
})

var _ = Describe("Copy HTTP domain boundaries", func() {
	It("binds the destination path, checks both resources and keeps private provenance out of public responses", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		users := map[string]string{}
		for _, name := range []string{"setter", "copier", "reader"} {
			u, err := identitypg.NewUserRepository(integrationDB).Create(ctx, name, name+"@example.test", "fixture")
			Expect(err).NotTo(HaveOccurred())
			users[name] = u.ID
		}
		spaces := tenancyapp.NewService(tenancypg.NewRepository(integrationDB))
		source, err := spaces.Create(ctx, users["setter"], tenancydomain.CreateInput{Slug: "private-source", Name: "Private source"})
		Expect(err).NotTo(HaveOccurred())
		Expect(spaces.SetMember(ctx, source.Domain.Slug, users["setter"], tenancydomain.MemberInput{Username: "copier", RoleKey: "member", Status: "active"})).To(Succeed())
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE domain_members SET role_key='author' WHERE domain_id=$1 AND user_id=$2", tenancydomain.OfficialID, users["copier"])
		Expect(err).NotTo(HaveOccurred())
		as := func(spaceID, actor string) context.Context {
			return tenancydomain.WithScope(ctx, tenancydomain.Scope{Domain: tenancydomain.Domain{ID: spaceID}, UserID: users[actor]})
		}
		root := GinkgoT().TempDir()
		writer := problempg.NewRepository(integrationDB)
		task, err := writer.Create(as(source.Domain.ID, "setter"), users["setter"], &problemdomain.CreateInput{Title: "Source release", StatementMD: "Statement", Visibility: "public", Source: "Public credit"})
		Expect(err).NotTo(HaveOccurred())
		fixture := newAuthoringFixture(as(source.Domain.ID, "setter"), task.ID, root)
		checkID := fixture.checked()
		revision := fixture.commit("source")
		_, err = fixture.repo.PublishCommit(fixture.ctx, task.ID, authoringdomain.CommitPublication{Revision: revision, CheckID: checkID, ExpectedVersion: 0})
		Expect(err).NotTo(HaveOccurred())
		auth := middleware.NewAuthMiddleware(staleRoleAuthenticator{users: users})
		router := gin.New()
		authoringhandler.NewWorkbenchHandler(fixture.service, 64<<20).RegisterRoutes(router.Group("/api/domains/:domain"), auth.Require(), middleware.ResolveDomain(spaces), httpapi.ResourceReferences(references.NewResolver(integrationDB)))
		reader := problemhttp.NewProblemHandler(problemapp.NewService(problempg.NewQueries(integrationDB), writer), nil)
		router.GET("/api/domains/:domain/problems/:id", auth.Optional(), middleware.ResolveDomain(spaces), httpapi.ResourceReferences(references.NewResolver(integrationDB)), httpx.NumberParam("problems", "id"), reader.Get)
		request := func(method, route, actor, body string) *httptest.ResponseRecorder {
			r := httptest.NewRequest(method, "/api/domains/"+route, strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			if actor != "" {
				r.Header.Set("Authorization", "Bearer "+actor)
			}
			result := httptest.NewRecorder()
			router.ServeHTTP(result, r)
			return result
		}
		body := fmt.Sprintf(`{"sourceDomain":"private-source","sourceProblem":"%s","sourceVersion":1,"attribution":"Approved training copy","domainId":"%s","ownerId":"%s"}`, task.PublicID, source.Domain.ID, users["setter"])
		Expect(request("POST", "official/authoring/problem-copies", "", body).Code).To(Equal(401))
		Expect(request("POST", "official/authoring/problem-copies", "copier", `{}`).Code).To(Equal(400))
		Expect(request("POST", "official/authoring/problem-copies", "copier", `{"attribution":"`+strings.Repeat("x", 65<<10)+`"}`).Code).To(Equal(413))
		Expect(request("POST", "official/authoring/problem-copies", "copier", body).Code).To(Equal(403))
		Expect(writer.SetGrant(as(source.Domain.ID, "setter"), task.ID, problemdomain.GrantInput{Username: "copier", Role: problemdomain.AccessReader})).To(Succeed())
		wrongSource := fmt.Sprintf(`{"sourceDomain":"official","sourceProblem":"%s","sourceVersion":1,"attribution":"Wrong domain"}`, task.PublicID)
		Expect(request("POST", "official/authoring/problem-copies", "copier", wrongSource).Code).To(Equal(404))
		response := request("POST", "official/authoring/problem-copies", "copier", body)
		Expect(response.Code).To(Equal(201), response.Body.String())
		var created struct {
			ProblemID string `json:"problemId"`
			PublicID  string `json:"-"`
			DomainID  string `json:"domainId"`
		}
		Expect(json.Unmarshal(response.Body.Bytes(), &created)).To(Succeed())
		Expect(created.DomainID).To(Equal(tenancydomain.OfficialID))
		copiedID, err := problempg.NewQueries(integrationDB).ResolveNumber(as(tenancydomain.OfficialID, "copier"), created.ProblemID)
		Expect(err).NotTo(HaveOccurred())
		copied, err := problempg.NewQueries(integrationDB).GetWorkspace(as(tenancydomain.OfficialID, "copier"), copiedID)
		Expect(err).NotTo(HaveOccurred())
		Expect(copied.OwnerID).To(Equal(users["copier"]))
		Expect(copied.Visibility).To(Equal("private"))
		originRoute := "official/authoring/problems/" + created.ProblemID + "/origin"
		response = request("GET", originRoute, "copier", "")
		Expect(response.Code).To(Equal(200))
		Expect(response.Body.String()).To(ContainSubstring("private-source"))
		response = request("GET", originRoute, "reader", "")
		Expect(response.Code).To(Equal(403))
		Expect(response.Body.String()).NotTo(ContainSubstring("private-source"))
		// Equal numbers in different domains identify different resources.
		sourceOrigin := request("GET", "private-source/authoring/problems/"+created.ProblemID+"/origin", "setter", "")
		Expect(sourceOrigin.Code).To(Equal(200))
		Expect(sourceOrigin.Body.String()).To(MatchJSON(`{}`))
		Expect(request("GET", "private-source/authoring/problems/"+copiedID+"/origin", "setter", "").Code).To(Equal(404))
		Expect(spaces.SetMember(ctx, source.Domain.Slug, users["setter"], tenancydomain.MemberInput{Username: "copier", RoleKey: "member", Status: "suspended"})).To(Succeed())
		Expect(request("POST", "official/authoring/problem-copies", "copier", body).Code).To(Equal(404))
		Expect(request("GET", originRoute, "copier", "").Code).To(Equal(200))
		Expect(copied.Source).To(Equal("Public credit"))
		copiedContext := as(tenancydomain.OfficialID, "copier")
		_, err = fixture.repo.SetVisibility(copiedContext, copiedID, authoringdomain.VisibilityChange{Visibility: "public", ExpectedVisibility: "private"})
		Expect(err).NotTo(HaveOccurred())
		copy, err := fixture.repo.WorkingCopy(copiedContext, copiedID)
		Expect(err).NotTo(HaveOccurred())
		commit, err := fixture.repo.Commit(copiedContext, copiedID, authoringdomain.CommitInput{ETag: copy.ETag, Message: "Adopt reviewed source", RequestID: "target"})
		Expect(err).NotTo(HaveOccurred())
		checks, err := fixture.repo.Checks(copiedContext, copiedID, 10)
		Expect(err).NotTo(HaveOccurred())
		Expect(checks).To(HaveLen(1))
		_, err = fixture.repo.PublishCommit(copiedContext, copiedID, authoringdomain.CommitPublication{Revision: commit.Commit.Revision, CheckID: checks[0].ID, ExpectedVersion: 0})
		Expect(err).NotTo(HaveOccurred())
		response = request("GET", "official/problems/"+created.ProblemID, "", "")
		Expect(response.Code).To(Equal(200))
		Expect(response.Body.String()).To(ContainSubstring("Public credit"))
		Expect(response.Body.String()).NotTo(ContainSubstring("private-source"))
		Expect(response.Body.String()).NotTo(ContainSubstring("Approved training copy"))
	})
})
