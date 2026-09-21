package postgres_test

import (
	"context"
	"encoding/json"
	authoringpg "github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres"
	evaluationpg "github.com/RimuruChan/Vertex/server/internal/workflows/evaluation/postgres"
	"strings"
	"time"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	contestpg "github.com/RimuruChan/Vertex/server/internal/modules/contest/infrastructure/postgres"
	identitypg "github.com/RimuruChan/Vertex/server/internal/modules/identity/infrastructure/postgres"
	judgepg "github.com/RimuruChan/Vertex/server/internal/modules/judge/infrastructure/postgres"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
	problempg "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres"
	submissiondomain "github.com/RimuruChan/Vertex/server/internal/modules/submission/domain"
	submission "github.com/RimuruChan/Vertex/server/internal/modules/submission/infrastructure/postgres"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	"github.com/RimuruChan/Vertex/server/internal/platform/database/dbtest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Explicit releases against PostgreSQL", func() {
	var writer *problempg.Repository
	var reader *problempg.Queries
	var owner, editor string
	var item *problemdomain.ProblemView
	as := func(ctx context.Context, user string) context.Context {
		return tenancydomain.WithScope(ctx, tenancydomain.Scope{Domain: tenancydomain.Domain{ID: tenancydomain.OfficialID}, UserID: user})
	}
	BeforeEach(func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		users := identitypg.NewUserRepository(integrationDB)
		u, err := users.Create(ctx, "publisher", "publisher@example.test", "fixture")
		Expect(err).NotTo(HaveOccurred())
		owner = u.ID
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE users SET role='admin' WHERE id=$1", owner)
		Expect(err).NotTo(HaveOccurred())
		u, err = users.Create(ctx, "editor", "editor@example.test", "fixture")
		Expect(err).NotTo(HaveOccurred())
		editor = u.ID
		writer = problempg.NewRepository(integrationDB)
		reader = problempg.NewQueries(integrationDB)
		item, err = writer.Create(as(ctx, owner), owner, &problemdomain.CreateInput{Title: "Initial", StatementMD: "Initial statement", Visibility: "private"})
		Expect(err).NotTo(HaveOccurred())
		Expect(writer.SetGrant(as(ctx, owner), item.ID, problemdomain.GrantInput{Username: "editor", Role: problemdomain.AccessEditor})).To(Succeed())
	})
	ready := func(ctx context.Context) *authoringFixture {
		return newAuthoringFixture(as(ctx, owner), item.ID, GinkgoT().TempDir())
	}
	It("requires matching self-test evidence even for a succeeded check", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		fixture := ready(ctx)
		fixture.document("selftest", "vertex/validation/positive.json", authoringdomain.EntryValidation, authoringdomain.ValidationMaterial{SchemaVersion: 1, Name: "correct candidate", Mode: "valid_output", Input: "input", Answer: "answer", Output: "answer"})
		check := fixture.checked()
		revision := fixture.commit("selftest-gate")
		publication := authoringdomain.CommitPublication{Revision: revision, CheckID: check}
		_, err := fixture.repo.PublishCommit(fixture.ctx, item.ID, publication)
		Expect(err).To(MatchError(authoringdomain.ErrNotPublished))
		for _, actual := range []string{"rejected", "accepted"} {
			data, err := json.Marshal([]authoringdomain.ValidationOutcome{{ID: "selftest", Name: "correct candidate", Mode: "valid_output", Status: "ok", Actual: actual}})
			Expect(err).NotTo(HaveOccurred())
			_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE problem_build_jobs SET validation_json=$2 WHERE id=$1", check, data)
			Expect(err).NotTo(HaveOccurred())
			_, err = fixture.repo.PublishCommit(fixture.ctx, item.ID, publication)
			if actual == "accepted" {
				Expect(err).NotTo(HaveOccurred())
			} else {
				Expect(err).To(MatchError(authoringdomain.ErrNotPublished))
			}
		}
	})
	It("keeps saved content private until explicit commit and publication", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		fixture := ready(ctx)
		meta, err := fixture.service.Material(fixture.ctx, item.ID, "problem", 0)
		Expect(err).NotTo(HaveOccurred())
		meta.Metadata.Title = "Private draft title"
		fixture.document("problem", "vertex/problem.json", authoringdomain.EntryMetadata, meta.Metadata)
		public, err := reader.Get(fixture.ctx, item.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(public.Title).To(Equal("Initial"))
		history, err := fixture.repo.History(fixture.ctx, item.ID, 0, 20)
		Expect(err).NotTo(HaveOccurred())
		Expect(history).To(BeEmpty())
		check := fixture.checked()
		revision := fixture.commit("release")
		_, err = fixture.repo.PublishCommit(as(ctx, editor), item.ID, authoringdomain.CommitPublication{Revision: revision, CheckID: check})
		Expect(err).To(MatchError(tenancydomain.ErrForbidden))
		first, err := fixture.repo.PublishCommit(fixture.ctx, item.ID, authoringdomain.CommitPublication{Revision: revision, CheckID: check})
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Version).To(Equal(1))
		meta.Metadata.Title = "Later private title"
		fixture.document("problem", "vertex/problem.json", authoringdomain.EntryMetadata, meta.Metadata)
		public, err = reader.Get(fixture.ctx, item.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(public.Title).To(Equal("Private draft title"))
	})
	It("never reuses an earlier lease artifact and rejects success without a current upload", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		fixture := ready(ctx)
		check, err := fixture.repo.StartCheck(fixture.ctx, item.ID, authoringdomain.CheckSelection{ETag: fixture.copy.ETag})
		Expect(err).NotTo(HaveOccurred())
		builds := authoringpg.NewBuildRepository(integrationDB)
		workerCtx := authoringdomain.WithCheckProtocol(ctx, authoringdomain.CheckPolicyVersion)
		first, pkg, err := builds.Claim(workerCtx, "first", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		Expect(first.ID).To(Equal(check.ID))
		artifact := authoringdomain.CheckArtifact{SchemaVersion: 1, ToolchainKey: strings.Repeat("a", 64), Snapshot: *pkg.Check, Tests: []authoringdomain.ArtifactTest{{ID: "case", Input: *pkg.Check.Tests[0].Input, Answer: *pkg.Check.Tests[0].Answer}}}
		upload := authoringdomain.PackageUpload{StoragePath: item.ID + "/" + strings.Repeat("b", 64), SHA256: strings.Repeat("b", 64), CaseCount: 1, Artifact: &artifact}
		Expect(builds.RecordPackage(ctx, first.ID, item.ID, "first", first.LeaseToken, upload)).To(Succeed())
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE problem_build_jobs SET lease_expires_at=now()-interval '1 second' WHERE id=$1", first.ID)
		Expect(err).NotTo(HaveOccurred())
		second, _, err := builds.Claim(workerCtx, "second", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		Expect(second.ID).To(Equal(first.ID))
		Expect(second.LeaseToken).NotTo(Equal(first.LeaseToken))
		Expect(builds.RecordPackage(ctx, first.ID, item.ID, "first", first.LeaseToken, upload)).To(MatchError(authoringdomain.ErrStaleLease))
		Expect(builds.Complete(ctx, authoringdomain.BuildResult{BuildID: second.ID, WorkerID: "second", LeaseToken: second.LeaseToken, Success: true, ToolchainKey: artifact.ToolchainKey}, "diff")).To(Succeed())
		finished, err := fixture.repo.Check(fixture.ctx, item.ID, check.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(finished.State).To(Equal(authoringdomain.BuildFailed))
		releases, err := fixture.repo.CommitReleases(fixture.ctx, item.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(releases).To(BeEmpty())
	})
	It("pins a queued judge generation to the submitted release after a new publication", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		fixture := ready(ctx)
		check := fixture.checked()
		revision := fixture.commit("first")
		_, err := fixture.repo.PublishCommit(fixture.ctx, item.ID, authoringdomain.CommitPublication{Revision: revision, CheckID: check})
		Expect(err).NotTo(HaveOccurred())
		var firstSHA string
		Expect(integrationDB.Pool.GetContext(ctx, &firstSHA, "SELECT sha256 FROM problem_versions WHERE problem_id=$1 AND version_no=1", item.ID)).To(Succeed())
		sub, err := submission.NewRepository(integrationDB, evaluationpg.Rebuild).Create(fixture.ctx, &submissiondomain.Submission{UserID: owner, ProblemID: item.ID, Language: "cpp", SourceCode: "int main(){}"})
		Expect(err).NotTo(HaveOccurred())
		fixture.save("answer", "data/1.ans", authoringdomain.EntryAnswer, "different answer")
		check = fixture.checked()
		revision = fixture.commit("second")
		_, err = fixture.repo.PublishCommit(fixture.ctx, item.ID, authoringdomain.CommitPublication{Revision: revision, CheckID: check, ExpectedVersion: 1})
		Expect(err).NotTo(HaveOccurred())
		job, err := judgepg.NewJobRepository(integrationDB, evaluationpg.Rebuild).Claim(ctx, "worker", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		Expect(job.SubmissionID).To(Equal(sub.ID))
		Expect(job.ProblemVersion).To(Equal(1))
		Expect(job.Testdata.SHA256).To(Equal(firstSHA))
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE judgements SET problem_version=2 WHERE (submission_id,generation)=(SELECT submission_id,generation FROM judge_jobs WHERE id=$1)", job.ID)
		Expect(err).To(HaveOccurred())
	})
	It("keeps contest slots pinned across rearrangement and restores versions on cancelled rejudging", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		fixture := ready(ctx)
		publish := func(answer string) *authoringdomain.CommitRelease {
			fixture.save("answer", "data/1.ans", authoringdomain.EntryAnswer, answer)
			check := fixture.checked()
			revision := fixture.commit(fixture.copy.ETag)
			releases, err := fixture.repo.CommitReleases(fixture.ctx, item.ID)
			Expect(err).NotTo(HaveOccurred())
			version := 0
			if len(releases) > 0 {
				version = releases[0].Version
			}
			result, err := fixture.repo.PublishCommit(fixture.ctx, item.ID, authoringdomain.CommitPublication{Revision: revision, CheckID: check, ExpectedVersion: version})
			Expect(err).NotTo(HaveOccurred())
			return result
		}
		publish("1\n")
		contests := contestpg.NewRepository(integrationDB)
		event, err := contests.Create(as(ctx, owner), owner, &contestdomain.PersistInput{Title: "Pinned", Rule: "icpc", Visibility: "private", Feedback: "full", BeginAt: time.Now().Add(-time.Hour), EndAt: time.Now().Add(time.Hour)})
		Expect(err).NotTo(HaveOccurred())
		entries := []contestdomain.ProblemEntry{{ProblemID: item.ID, Label: "A", Points: 100}}
		Expect(contests.SetProblems(as(ctx, owner), event.ID, entries)).To(Succeed())
		submissions := submission.NewRepository(integrationDB, evaluationpg.Rebuild)
		sub, err := submissions.Create(as(ctx, owner), &submissiondomain.Submission{UserID: owner, ProblemID: item.ID, ContestID: &event.ID, Language: "cpp", SourceCode: "int main(){}"})
		Expect(err).NotTo(HaveOccurred())
		Expect(sub.ProblemVersion).To(Equal(1))
		meta, err := fixture.service.Material(fixture.ctx, item.ID, "problem", 0)
		Expect(err).NotTo(HaveOccurred())
		meta.Metadata.Title = "New title"
		meta.Metadata.TimeLimitMs = 2000
		fixture.document("problem", "vertex/problem.json", authoringdomain.EntryMetadata, meta.Metadata)
		publish("2\n")
		Expect(contests.SetProblems(as(ctx, owner), event.ID, entries)).To(Succeed())
		pinned, err := contests.Problem(ctx, event.ID, "A")
		Expect(err).NotTo(HaveOccurred())
		Expect(pinned.Version).To(Equal(1))
		Expect(pinned.TimeLimitMs).To(Equal(1000))
		Expect(pinned.Title).To(Equal("Initial"))
		Expect(contests.UseProblemVersion(as(ctx, editor), event.ID, item.ID, 2, 1)).To(MatchError(contestdomain.ErrForbidden))
		Expect(contests.UseProblemVersion(as(ctx, owner), event.ID, item.ID, 2, 1)).To(Succeed())
		Expect(contests.UseProblemVersion(as(ctx, owner), event.ID, item.ID, 1, 1)).To(MatchError(contestdomain.ErrVersionConflict))
		pinned, err = contests.Problem(ctx, event.ID, "A")
		Expect(err).NotTo(HaveOccurred())
		Expect(pinned.Version).To(Equal(2))
		Expect(pinned.TimeLimitMs).To(Equal(2000))
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE judgements j SET status='Accepted',score=100,judged_at=now() FROM submissions s WHERE j.submission_id=s.id AND j.generation=s.result_generation AND s.id=$1", sub.ID)
		Expect(err).NotTo(HaveOccurred())
		batch, err := submissions.CreateRejudging(as(ctx, owner), submissiondomain.RejudgeSelector{ContestID: event.ID}, owner)
		Expect(err).NotTo(HaveOccurred())
		current, err := submissions.Get(as(ctx, owner), sub.ID, submissiondomain.Viewer{UserID: owner})
		Expect(err).NotTo(HaveOccurred())
		Expect(current.ProblemVersion).To(Equal(2))
		Expect(submissions.CancelRejudging(as(ctx, owner), batch.ID)).To(Succeed())
		current, err = submissions.Get(as(ctx, owner), sub.ID, submissiondomain.Viewer{UserID: owner})
		Expect(err).NotTo(HaveOccurred())
		Expect(current.ProblemVersion).To(Equal(1))
		Expect(current.Status).To(Equal("Accepted"))
		Expect(writer.Delete(as(ctx, owner), item.ID)).To(MatchError(problemdomain.ErrReferenced))
	})
})
