package domain_test

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/authoring"
	"github.com/RimuruChan/Vertex/server/internal/console"
	"github.com/RimuruChan/Vertex/server/internal/content"
	"github.com/RimuruChan/Vertex/server/internal/contest"
	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/identity"
	"github.com/RimuruChan/Vertex/server/internal/problem"
	"github.com/RimuruChan/Vertex/server/internal/problemset"
	"github.com/RimuruChan/Vertex/server/internal/profile"
	"github.com/RimuruChan/Vertex/server/internal/publicid"
	"github.com/RimuruChan/Vertex/server/internal/submission"
	"github.com/jackc/pgx/v5/pgconn"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Resource domain boundaries against PostgreSQL", func() {
	var scope domain.Scope
	var owner string
	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		user, err := identity.NewUserStore(integrationDB).Create(ctx, "owner", "owner@example.test", "fixture-hash")
		Expect(err).NotTo(HaveOccurred())
		owner = user.ID
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE domain_members SET role_key='author' WHERE user_id=$1 AND domain_id=$2", owner, domain.OfficialID)
		Expect(err).NotTo(HaveOccurred())
		scope, err = domain.NewService(domain.NewStore(integrationDB)).Create(ctx, owner, domain.CreateInput{Slug: "training", Name: "Training"})
		Expect(err).NotTo(HaveOccurred())
	})

	It("allocates distinct stable numbers within each domain and rejects identity changes", func(spec SpecContext) {
		ctx := domain.WithScope(spec, domain.Scope{Domain: domain.Domain{ID: domain.OfficialID}, UserID: owner})
		scoped := domain.WithScope(ctx, scope)
		writer := problem.NewProblemAdminStore(integrationDB, GinkgoT().TempDir())
		first, err := writer.Create(ctx, owner, &problem.CreateInput{Title: "Official"})
		Expect(err).NotTo(HaveOccurred())
		second, err := writer.Create(scoped, owner, &problem.CreateInput{Title: "Training"})
		Expect(err).NotTo(HaveOccurred())
		Expect(first.PublicID).To(Equal(second.PublicID))
		Expect(first.ID).NotTo(Equal(second.ID))
		resolver := publicid.NewStore(integrationDB)
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
		third, err := writer.Create(scoped, owner, &problem.CreateInput{Title: "Not reused"})
		Expect(err).NotTo(HaveOccurred())
		number, err := strconv.ParseInt(second.PublicID, 10, 64)
		Expect(err).NotTo(HaveOccurred())
		Expect(third.PublicID).To(Equal(strconv.FormatInt(number+1, 10)))
	})

	It("serializes concurrent allocation without reusing a committed number", func(spec SpecContext) {
		ctx := domain.WithScope(spec, domain.Scope{Domain: domain.Domain{ID: domain.OfficialID}, UserID: owner})
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
		ctx := domain.WithScope(spec, domain.Scope{Domain: domain.Domain{ID: domain.OfficialID}, UserID: owner})
		scoped := domain.WithScope(ctx, scope)
		writer := problem.NewProblemAdminStore(integrationDB, GinkgoT().TempDir())
		foreign, err := writer.Create(scoped, owner, &problem.CreateInput{Title: "Training practice", Visibility: "public"})
		Expect(err).NotTo(HaveOccurred())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, foreign.ID)).To(Succeed())
		contests := contest.NewContestStore(integrationDB)
		competition, err := contests.Create(scoped, owner, &contest.PersistInput{Title: "Training round", Rule: "icpc", BeginAt: time.Now().Add(-time.Hour), EndAt: time.Now().Add(time.Hour), Visibility: "public", Feedback: "full"})
		Expect(err).NotTo(HaveOccurred())
		Expect(contests.SetProblems(scoped, competition.ID, []contest.ProblemEntry{{ProblemID: foreign.ID, Label: "A"}})).To(Succeed())
		_, total, err := contests.ListAdmin(ctx, 20, 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(BeZero())
		_, err = contests.Get(ctx, competition.ID)
		Expect(err).To(MatchError(contest.ErrNotFound))
		Expect(contests.SetProblems(ctx, competition.ID, nil)).To(MatchError(contest.ErrNotFound))
		Expect(contests.Register(ctx, competition.ID, owner)).To(MatchError(contest.ErrNotFound))
		_, err = contests.AddStaff(scoped, competition.ID, "owner", "jury")
		Expect(err).NotTo(HaveOccurred())
		role, err := contests.StaffRole(ctx, competition.ID, owner)
		Expect(err).NotTo(HaveOccurred())
		Expect(role).To(BeEmpty())
		staff, err := contests.ListStaff(ctx, competition.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(staff).To(BeEmpty())
		message, err := contests.CreateClarification(scoped, contest.ClarificationInput{ContestID: competition.ID, AuthorID: owner, Body: "Private-domain broadcast", FromJury: true})
		Expect(err).NotTo(HaveOccurred())
		_, err = contests.GetClarification(ctx, competition.ID, message.ID)
		Expect(err).To(MatchError(contest.ErrClarificationNotFound))
		messages, err := contests.ListClarifications(ctx, competition.ID, contest.Viewer{Role: "admin"})
		Expect(err).NotTo(HaveOccurred())
		Expect(messages).To(BeEmpty())

		submissions := submission.NewSubmissionStore(integrationDB)
		input := &submission.Submission{UserID: owner, ProblemID: foreign.ID, Language: "cpp", SourceCode: "private code"}
		_, err = submissions.Create(ctx, input)
		Expect(err).To(MatchError(submission.ErrNotFound))
		created, err := submissions.Create(scoped, input)
		Expect(err).NotTo(HaveOccurred())
		_, total, err = submissions.List(ctx, submission.Filters{}, submission.Viewer{Admin: true})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(BeZero())
		_, err = submissions.Get(ctx, created.ID, submission.Viewer{UserID: owner, Admin: true})
		Expect(err).To(MatchError(submission.ErrNotFound))
		_, err = submissions.Progress(ctx, created.ID, submission.Viewer{Admin: true})
		Expect(err).To(MatchError(submission.ErrNotFound))
		Expect(submissions.Rejudge(ctx, created.ID)).To(MatchError(submission.ErrNotFound))
		var state string
		Expect(integrationDB.Pool.GetContext(ctx, &state, "SELECT state FROM judge_jobs WHERE submission_id=$1", created.ID)).To(Succeed())
		Expect(state).To(Equal("queued"))
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE submissions SET judged_at=now(),status='Accepted' WHERE id=$1", created.ID)
		Expect(err).NotTo(HaveOccurred())
		batch, err := submissions.CreateRejudging(scoped, submission.RejudgeSelector{SubmissionIDs: []string{created.ID}}, owner)
		Expect(err).NotTo(HaveOccurred())
		_, err = submissions.Rejudging(ctx, batch.ID)
		Expect(err).To(MatchError(submission.ErrRejudgeNotFound))
		Expect(submissions.CancelRejudging(ctx, batch.ID)).To(MatchError(submission.ErrRejudgeNotFound))
		batches, err := submissions.ListRejudgings(ctx, "", 20)
		Expect(err).NotTo(HaveOccurred())
		Expect(batches).To(BeEmpty())
		changes, err := submissions.RejudgingChanges(ctx, batch.ID, 20)
		Expect(err).To(MatchError(submission.ErrRejudgeNotFound))
		Expect(changes).To(BeEmpty())
		changes, err = submissions.RejudgingChanges(scoped, batch.ID, 20)
		Expect(err).NotTo(HaveOccurred())
		Expect(changes).To(HaveLen(1))
		account, err := profile.NewProfileStore(integrationDB).ByUsername(ctx, "owner")
		Expect(err).NotTo(HaveOccurred())
		Expect(account.SubmissionCount).To(BeZero())
		Expect(account.Activity).To(BeEmpty())
		Expect(account.ByDifficulty).To(BeEmpty())
	})

	It("does not expose authoring sources or build control across domains while internal workers can claim them", func(spec SpecContext) {
		ctx := domain.WithScope(spec, domain.Scope{Domain: domain.Domain{ID: domain.OfficialID}, UserID: owner})
		scoped := domain.WithScope(ctx, scope)
		writer := problem.NewProblemAdminStore(integrationDB, GinkgoT().TempDir())
		foreign, err := writer.Create(scoped, owner, &problem.CreateInput{Title: "Unreleased package"})
		Expect(err).NotTo(HaveOccurred())
		packages := authoring.NewPackageStore(integrationDB)
		file, err := packages.SaveFile(scoped, authoring.File{ProblemID: foreign.ID, Kind: "solution", Name: "main", Language: "cpp", SourceCode: "private source", ExpectedVerdict: "Accepted"})
		Expect(err).NotTo(HaveOccurred())
		test, err := packages.CreateTest(scoped, authoring.Test{ProblemID: foreign.ID, Source: "manual", InputData: "hidden input"})
		Expect(err).NotTo(HaveOccurred())
		_, err = packages.Meta(ctx, foreign.ID)
		Expect(err).To(MatchError(authoring.ErrNotFound))
		_, err = packages.Snapshot(ctx, foreign.ID)
		Expect(err).To(MatchError(authoring.ErrNotFound))
		_, err = packages.File(ctx, foreign.ID, file.ID)
		Expect(err).To(MatchError(authoring.ErrNotFound))
		_, err = packages.Tests(ctx, foreign.ID, true)
		Expect(err).To(MatchError(authoring.ErrNotFound))
		Expect(packages.DeleteFile(ctx, foreign.ID, file.ID)).To(MatchError(authoring.ErrNotFound))
		Expect(packages.ReorderTest(ctx, foreign.ID, test.ID, test.Index)).To(MatchError(authoring.ErrNotFound))
		_, _, err = writer.SaveTestdata(ctx, foreign.ID, nil, "diff")
		Expect(err).To(MatchError(problem.ErrNotFound))
		builds := authoring.NewBuildStore(integrationDB)
		build, err := builds.Enqueue(scoped, foreign.ID, owner)
		Expect(err).NotTo(HaveOccurred())
		_, err = builds.Get(ctx, foreign.ID, build.ID)
		Expect(err).To(MatchError(authoring.ErrNotFound))
		Expect(builds.Cancel(ctx, foreign.ID, build.ID)).To(MatchError(authoring.ErrNotFound))
		claimed, pkg, err := builds.Claim(ctx, "domain-fixture-worker", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed.ID).To(Equal(build.ID))
		Expect(pkg.ProblemID).To(Equal(foreign.ID))
		Expect(pkg.Solutions).To(HaveLen(1))
		Expect(pkg.Solutions[0].SourceCode).To(Equal("private source"))
		Expect(pkg.Tests[0].InputData).To(Equal("hidden input"))
	})

	It("keeps tag management and announcements within the requested domain", func(spec SpecContext) {
		ctx := domain.WithScope(spec, domain.Scope{Domain: domain.Domain{ID: domain.OfficialID}, UserID: owner})
		scoped := domain.WithScope(ctx, scope)
		writer := problem.NewProblemAdminStore(integrationDB, GinkgoT().TempDir())
		_, err := writer.Create(ctx, owner, &problem.CreateInput{Title: "Official", Tags: []string{"shared"}})
		Expect(err).NotTo(HaveOccurred())
		_, err = writer.Create(scoped, owner, &problem.CreateInput{Title: "Training", Tags: []string{"shared", "training"}})
		Expect(err).NotTo(HaveOccurred())
		Expect(dbtest.PublishedProblems(ctx, integrationDB)).To(Succeed())
		store := console.NewConsoleStore(integrationDB)
		_, err = store.ListTags(ctx)
		Expect(err).To(MatchError(domain.ErrForbidden))
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE domain_members SET role_key='admin' WHERE domain_id=$1 AND user_id=$2", domain.OfficialID, owner)
		Expect(err).NotTo(HaveOccurred())
		localTags, err := store.ListTags(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(localTags).To(HaveLen(1))
		foreignTags, err := store.ListTags(scoped)
		Expect(err).NotTo(HaveOccurred())
		Expect(foreignTags).To(HaveLen(2))
		_, err = store.MergeTags(ctx, foreignTags[0].ID, localTags[0].ID)
		Expect(err).To(MatchError(console.ErrNotFound))
		_, err = store.MergeTags(ctx, localTags[0].ID, foreignTags[0].ID)
		Expect(err).To(MatchError(console.ErrNotFound))
		_, err = store.RenameTag(ctx, foreignTags[0].ID, "wrong scope")
		Expect(err).To(MatchError(console.ErrNotFound))
		announcement, err := store.CreateAnnouncement(scoped, owner, console.AnnouncementInput{Title: "Training news", Published: true})
		Expect(err).NotTo(HaveOccurred())
		announcements, err := store.ListAnnouncements(ctx, true, 20)
		Expect(err).NotTo(HaveOccurred())
		Expect(announcements).To(BeEmpty())
		Expect(store.DeleteAnnouncement(ctx, announcement.ID)).To(MatchError(console.ErrNotFound))
	})

	It("scopes root lists, counts, tags, details and writes even for administrators", func(spec SpecContext) {
		ctx := domain.WithScope(spec, domain.Scope{Domain: domain.Domain{ID: domain.OfficialID}, UserID: owner})
		scoped := domain.WithScope(ctx, scope)
		writer := problem.NewProblemAdminStore(integrationDB, GinkgoT().TempDir())
		reader := problem.NewProblemStore(integrationDB)
		foreign, err := writer.Create(scoped, owner, &problem.CreateInput{Title: "Training secret", Tags: []string{"same tag"}})
		Expect(err).NotTo(HaveOccurred())
		local, err := writer.Create(ctx, owner, &problem.CreateInput{Title: "Official", Visibility: "public", Tags: []string{"same tag"}})
		Expect(err).NotTo(HaveOccurred())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, local.ID, foreign.ID)).To(Succeed())
		items, total, err := reader.List(ctx, problem.Filters{})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items).To(HaveLen(1))
		Expect(items[0].ID).To(Equal(local.ID))
		_, err = reader.Get(ctx, foreign.ID)
		Expect(err).To(MatchError(problem.ErrNotFound))
		_, err = writer.Update(ctx, foreign.ID, &problem.UpdateInput{CreateInput: problem.CreateInput{Title: "Tampered", TimeLimitMs: 1000, MemoryLimitKb: 65536, Visibility: "public"}})
		Expect(err).To(MatchError(problem.ErrNotFound))
		Expect(writer.Delete(ctx, foreign.ID)).To(MatchError(problem.ErrNotFound))
		allowed, err := content.NewAccessStore(integrationDB).CanViewProblem(ctx, foreign.ID, owner, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(allowed).To(BeFalse())
		var distinctTags int
		Expect(integrationDB.Pool.GetContext(ctx, &distinctTags, "SELECT count(*) FROM tags WHERE name='same tag'")).To(Succeed())
		Expect(distinctTags).To(Equal(2))

		sets := problemset.NewSetStore(integrationDB)
		set, err := sets.Create(scoped, owner, problemset.UpsertInput{Title: "Private-domain set"})
		Expect(err).NotTo(HaveOccurred())
		_, total, err = sets.List(ctx, problemset.Filters{Admin: true, Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(BeZero())
		_, err = sets.Get(ctx, set.ID, owner, true)
		Expect(err).To(MatchError(problemset.ErrNotFound))
		Expect(sets.SetItems(ctx, set.ID, owner, true, nil)).To(MatchError(problemset.ErrNotFound))
		Expect(sets.SetItems(scoped, set.ID, owner, true, []problemset.ItemInput{{ProblemID: local.ID}})).To(HaveOccurred())
		Expect(sets.SetItems(scoped, set.ID, owner, true, []problemset.ItemInput{{ProblemID: foreign.ID}})).To(Succeed())

		editorials := content.NewEditorialStore(integrationDB)
		editorial, err := editorials.Create(scoped, owner, content.EditorialInput{ProblemID: foreign.ID, Title: "Private solution", ContentMD: "private solution body", Visibility: "public", Status: "published"})
		Expect(err).NotTo(HaveOccurred())
		_, total, err = editorials.List(ctx, content.EditorialFilters{Admin: true, Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(BeZero())
		_, err = editorials.Get(ctx, editorial.ID, owner)
		Expect(err).To(MatchError(content.ErrNotFound))
		_, err = editorials.Vote(ctx, editorial.ID, owner, true)
		Expect(err).To(MatchError(content.ErrNotFound))

		posts := content.NewDiscussionStore(integrationDB)
		post, err := posts.CreateProblemPost(scoped, foreign.ID, owner, "Private discussion", nil)
		Expect(err).NotTo(HaveOccurred())
		_, err = posts.Get(ctx, post.ID, owner)
		Expect(err).To(MatchError(content.ErrNotFound))
		_, err = posts.Update(ctx, post.ID, owner, "Tampered")
		Expect(err).To(MatchError(content.ErrNotFound))
		list, err := posts.ListByProblem(ctx, foreign.ID, owner)
		Expect(err).To(MatchError(content.ErrNotFound))
		Expect(list.Posts).To(BeEmpty())
	})
})
