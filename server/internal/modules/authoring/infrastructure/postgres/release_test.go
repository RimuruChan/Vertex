package postgres_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	authoringpg "github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres"
	"path"
	"strings"
	"time"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	contestpg "github.com/RimuruChan/Vertex/server/internal/modules/contest/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/platform/database/dbtest"
	identitypg "github.com/RimuruChan/Vertex/server/internal/modules/identity/infrastructure/postgres"
	judgepg "github.com/RimuruChan/Vertex/server/internal/modules/judge/infrastructure/postgres"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
	problemfiles "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/filesystem"
	problempg "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres"
	submissiondomain "github.com/RimuruChan/Vertex/server/internal/modules/submission/domain"
	submission "github.com/RimuruChan/Vertex/server/internal/modules/submission/infrastructure/postgres"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Explicit releases against PostgreSQL", func() {
	var packages *authoringpg.PackageRepository
	var builds *authoringpg.BuildRepository
	var writer *problempg.Repository
	var reader *problempg.Queries
	var owner, editor string
	var item *problemdomain.Problem
	as := func(ctx context.Context, user string) context.Context {
		return tenancydomain.WithScope(ctx, tenancydomain.Scope{Domain: tenancydomain.Domain{ID: tenancydomain.OfficialID}, UserID: user})
	}
	BeforeEach(func(ctx SpecContext) {
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
		writer = problempg.NewRepository(integrationDB, problemfiles.NewTestdataStorage(GinkgoT().TempDir()))
		reader = problempg.NewQueries(integrationDB)
		packages = authoringpg.NewPackageRepository(integrationDB)
		builds = authoringpg.NewBuildRepository(integrationDB)
		item, err = writer.Create(as(ctx, owner), owner, &problemdomain.CreateInput{Title: "Initial", StatementMD: "Initial statement", Visibility: "private"})
		Expect(err).NotTo(HaveOccurred())
		Expect(writer.SetGrant(as(ctx, owner), item.ID, problemdomain.GrantInput{Username: "editor", Role: problemdomain.AccessEditor})).To(Succeed())
	})
	It("separates working statements and uploaded candidates from explicit publication", func(ctx SpecContext) {
		_, err := packages.SaveStatement(as(ctx, editor), authoringdomain.Statement{ProblemID: item.ID, Language: "zh", Name: "Draft name", Legend: "Draft statement"})
		Expect(err).NotTo(HaveOccurred())
		live, err := reader.Get(ctx, item.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(live.Title).To(Equal("Initial"))
		Expect(live.StatementMD).To(Equal("Initial statement"))
		_, _, err = writer.SaveTestdata(as(ctx, editor), item.ID, makePackage(map[string]string{"1.in": "1\n", "1.out": "1\n"}), "diff")
		Expect(err).NotTo(HaveOccurred())
		meta, err := packages.Meta(as(ctx, owner), item.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(meta.PublishedVersion).To(BeZero())
		input := authoringdomain.PublishInput{Revision: meta.PackageRevision, ArtifactVersion: meta.TestdataVersion}
		_, err = packages.Publish(as(ctx, editor), item.ID, input)
		Expect(err).To(MatchError(tenancydomain.ErrForbidden))
		release, err := packages.Publish(as(ctx, owner), item.ID, input)
		Expect(err).NotTo(HaveOccurred())
		Expect(release.Version).To(Equal(1))
		live, err = reader.Get(ctx, item.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(live.Title).To(Equal("Draft name"))
		Expect(live.StatementMD).To(ContainSubstring("Draft statement"))
		repeat, err := packages.Publish(as(ctx, owner), item.ID, input)
		Expect(err).NotTo(HaveOccurred())
		Expect(repeat.Version).To(Equal(1))
		_, err = packages.SaveStatement(as(ctx, editor), authoringdomain.Statement{ProblemID: item.ID, Language: "zh", Name: "Second name", Legend: "Second statement"})
		Expect(err).NotTo(HaveOccurred())
		_, err = packages.Publish(as(ctx, owner), item.ID, input)
		Expect(err).To(MatchError(authoringdomain.ErrRevisionConflict))
		meta, err = packages.Meta(as(ctx, owner), item.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(meta.DataRevision).To(Equal(meta.BuiltRevision))
		release, err = packages.Publish(as(ctx, owner), item.ID, authoringdomain.PublishInput{Revision: meta.PackageRevision, ArtifactVersion: meta.TestdataVersion})
		Expect(err).NotTo(HaveOccurred())
		Expect(release.Version).To(Equal(2))
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE problem_versions SET statement_md='tampered' WHERE problem_id=$1 AND version_no=1", item.ID)
		Expect(err).To(HaveOccurred())
	})
	It("reads full test inputs for collaborators instead of editing the list preview", func(ctx SpecContext) {
		input := strings.Repeat("1234567890\n", 150)
		test, err := packages.CreateTest(as(ctx, owner), authoringdomain.Test{ProblemID: item.ID, Source: authoringdomain.TestManual, InputData: input})
		Expect(err).NotTo(HaveOccurred())
		Expect(writer.SetGrant(as(ctx, owner), item.ID, problemdomain.GrantInput{Username: "editor", Role: problemdomain.AccessReader})).To(Succeed())
		preview, err := packages.Tests(as(ctx, editor), item.ID, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(preview[0].InputData).To(HaveLen(512))
		full, err := packages.Test(as(ctx, editor), item.ID, test.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(full.InputData).To(Equal(input))
		full.Description = "Only metadata changed"
		_, err = packages.UpdateTest(as(ctx, editor), *full)
		Expect(err).To(MatchError(tenancydomain.ErrForbidden))
		_, err = packages.UpdateTest(as(ctx, owner), *full)
		Expect(err).NotTo(HaveOccurred())
		full, err = packages.Test(as(ctx, owner), item.ID, test.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(full.InputData).To(Equal(input))
		_, err = packages.Test(ctx, item.ID, test.ID)
		Expect(err).To(HaveOccurred())
	})
	It("seals build inputs before queueing and does not reuse an old attempt's artifact", func(ctx SpecContext) {
		file, err := packages.SaveFile(as(ctx, editor), authoringdomain.File{ProblemID: item.ID, Kind: authoringdomain.KindSolution, Name: "main.cpp", Language: "cpp", SourceCode: "original", IsActive: true})
		Expect(err).NotTo(HaveOccurred())
		_, err = packages.CreateTest(as(ctx, editor), authoringdomain.Test{ProblemID: item.ID, Source: authoringdomain.TestManual, InputData: "1\n"})
		Expect(err).NotTo(HaveOccurred())
		queued, err := builds.Enqueue(as(ctx, editor), item.ID, editor)
		Expect(err).NotTo(HaveOccurred())
		file.SourceCode = "later edit"
		_, err = packages.SaveFile(as(ctx, editor), *file)
		Expect(err).NotTo(HaveOccurred())
		first, input, err := builds.Claim(ctx, "worker-a", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Revision).To(Equal(queued.Revision))
		Expect(input.MainSolution().SourceCode).To(Equal("original"))
		Expect(builds.RecordPackage(ctx, first.ID, item.ID, first.WorkerID, first.LeaseToken, authoringdomain.PackageUpload{StoragePath: path.Join(item.ID, "oldhash"), SHA256: "oldhash", CaseCount: 1, Checker: "diff"})).To(Succeed())
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE problem_build_jobs SET lease_expires_at=now()-interval '1 second' WHERE id=$1", first.ID)
		Expect(err).NotTo(HaveOccurred())
		second, input, err := builds.Claim(ctx, "worker-b", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		Expect(input.MainSolution().SourceCode).To(Equal("original"))
		Expect(second.PackagePath).To(BeEmpty())
		Expect(builds.Complete(ctx, authoringdomain.BuildResult{BuildID: second.ID, WorkerID: second.WorkerID, LeaseToken: second.LeaseToken, Success: true}, "diff")).To(Succeed())
		finished, err := builds.Get(as(ctx, owner), item.ID, second.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(finished.State).To(Equal(authoringdomain.BuildFailed))
	})
	It("rejects a sealed build whose domain disagrees with its parent resource", func(ctx SpecContext) {
		queued, err := builds.Enqueue(as(ctx, owner), item.ID, owner)
		Expect(err).NotTo(HaveOccurred())
		var encoded []byte
		Expect(integrationDB.Pool.GetContext(ctx, &encoded, "SELECT input_json FROM problem_build_jobs WHERE id=$1", queued.ID)).To(Succeed())
		var input authoringdomain.Package
		Expect(json.Unmarshal(encoded, &input)).To(Succeed())
		input.DomainID = "wrong-domain"
		encoded, err = json.Marshal(input)
		Expect(err).NotTo(HaveOccurred())
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE problem_build_jobs SET input_json=$2 WHERE id=$1", queued.ID, encoded)
		Expect(err).NotTo(HaveOccurred())
		_, _, err = builds.Claim(ctx, "worker-a", time.Minute)
		Expect(err).To(MatchError(authoringdomain.ErrPackageTarget))
		var state string
		Expect(integrationDB.Pool.GetContext(ctx, &state, "SELECT state FROM problem_build_jobs WHERE id=$1", queued.ID)).To(Succeed())
		Expect(state).To(Equal(authoringdomain.BuildQueued))
	})
	It("keeps a queued judge generation on its submitted release after a new publication", func(ctx SpecContext) {
		_, _, err := writer.SaveTestdata(as(ctx, owner), item.ID, makePackage(map[string]string{"1.in": "1\n", "1.out": "1\n"}), "diff")
		Expect(err).NotTo(HaveOccurred())
		meta, err := packages.Meta(as(ctx, owner), item.ID)
		Expect(err).NotTo(HaveOccurred())
		first, err := packages.Publish(as(ctx, owner), item.ID, authoringdomain.PublishInput{Revision: meta.PackageRevision, ArtifactVersion: meta.TestdataVersion})
		Expect(err).NotTo(HaveOccurred())
		sub, err := submission.NewRepository(integrationDB).Create(as(ctx, owner), &submissiondomain.Submission{UserID: owner, ProblemID: item.ID, Language: "cpp", SourceCode: "int main(){}"})
		Expect(err).NotTo(HaveOccurred())
		_, _, err = writer.SaveTestdata(as(ctx, owner), item.ID, makePackage(map[string]string{"1.in": "2\n", "1.out": "2\n"}), "diff")
		Expect(err).NotTo(HaveOccurred())
		meta, err = packages.Meta(as(ctx, owner), item.ID)
		Expect(err).NotTo(HaveOccurred())
		second, err := packages.Publish(as(ctx, owner), item.ID, authoringdomain.PublishInput{Revision: meta.PackageRevision, ArtifactVersion: meta.TestdataVersion})
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Version).To(Equal(2))
		job, err := judgepg.NewJobRepository(integrationDB).Claim(ctx, "worker", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		Expect(job.SubmissionID).To(Equal(sub.ID))
		Expect(job.ProblemVersion).To(Equal(1))
		Expect(job.DomainID).To(Equal(tenancydomain.OfficialID))
		Expect(job.Testdata.SHA256).To(Equal(first.SHA256))
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE judge_jobs SET problem_version=2 WHERE id=$1", job.ID)
		Expect(err).To(HaveOccurred())
	})
	It("keeps contest slots pinned across rearrangement and restores versions on cancelled rejudging", func(ctx SpecContext) {
		publish := func(answer string) *authoringdomain.Release {
			_, _, err := writer.SaveTestdata(as(ctx, owner), item.ID, makePackage(map[string]string{"1.in": "1\n", "1.out": answer}), "diff")
			Expect(err).NotTo(HaveOccurred())
			meta, err := packages.Meta(as(ctx, owner), item.ID)
			Expect(err).NotTo(HaveOccurred())
			release, err := packages.Publish(as(ctx, owner), item.ID, authoringdomain.PublishInput{Revision: meta.PackageRevision, ArtifactVersion: meta.TestdataVersion})
			Expect(err).NotTo(HaveOccurred())
			return release
		}
		publish("1\n")
		contests := contestpg.NewRepository(integrationDB)
		event, err := contests.Create(as(ctx, owner), owner, &contestdomain.PersistInput{Title: "Pinned", Rule: "icpc", Visibility: "private", Feedback: "full", BeginAt: time.Now().Add(-time.Hour), EndAt: time.Now().Add(time.Hour)})
		Expect(err).NotTo(HaveOccurred())
		entries := []contestdomain.ProblemEntry{{ProblemID: item.ID, Label: "A", Points: 100}}
		Expect(contests.SetProblems(as(ctx, owner), event.ID, entries)).To(Succeed())
		submissions := submission.NewRepository(integrationDB)
		sub, err := submissions.Create(as(ctx, owner), &submissiondomain.Submission{UserID: owner, ProblemID: item.ID, ContestID: &event.ID, Language: "cpp", SourceCode: "int main(){}"})
		Expect(err).NotTo(HaveOccurred())
		Expect(sub.ProblemVersion).To(Equal(1))
		_, err = writer.Update(as(ctx, owner), item.ID, &problemdomain.UpdateInput{CreateInput: problemdomain.CreateInput{Title: "New title", StatementMD: "New statement", Visibility: "private", TimeLimitMs: 2000, MemoryLimitKb: 65536}})
		Expect(err).NotTo(HaveOccurred())
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
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE submissions SET status='Accepted',score=100,judged_at=now() WHERE id=$1", sub.ID)
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
	It("searches working metadata without exposing it in the public catalogue", func(ctx SpecContext) {
		_, err := writer.Update(as(ctx, owner), item.ID, &problemdomain.UpdateInput{CreateInput: problemdomain.CreateInput{Title: "Unpublished search", StatementMD: "Work", Visibility: "public", TimeLimitMs: 1000, MemoryLimitKb: 65536, Tags: []string{"working-tag"}}})
		Expect(err).NotTo(HaveOccurred())
		found, total, err := reader.List(as(ctx, owner), problemdomain.Filters{Workspace: true, ViewerID: owner, Keyword: "Unpublished", Tag: "working-tag"})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(found[0].Title).To(Equal("Unpublished search"))
		Expect(found[0].Tags).To(ConsistOf("working-tag"))
		found, total, err = reader.List(ctx, problemdomain.Filters{Visibility: "public"})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(BeZero())
		Expect(found).To(BeEmpty())
	})
	It("keeps overview title edits aligned with the selected structured statement", func(ctx SpecContext) {
		_, err := packages.SaveStatement(as(ctx, owner), authoringdomain.Statement{ProblemID: item.ID, Language: "zh", Name: "Statement name", Legend: "Body"})
		Expect(err).NotTo(HaveOccurred())
		_, err = writer.Update(as(ctx, owner), item.ID, &problemdomain.UpdateInput{CreateInput: problemdomain.CreateInput{Title: "Overview name", Visibility: "private", TimeLimitMs: 1000, MemoryLimitKb: 262144}})
		Expect(err).NotTo(HaveOccurred())
		_, _, err = writer.SaveTestdata(as(ctx, owner), item.ID, makePackage(map[string]string{"1.in": "1\n", "1.out": "1\n"}), "diff")
		Expect(err).NotTo(HaveOccurred())
		meta, err := packages.Meta(as(ctx, owner), item.ID)
		Expect(err).NotTo(HaveOccurred())
		_, err = packages.Publish(as(ctx, owner), item.ID, authoringdomain.PublishInput{Revision: meta.PackageRevision, ArtifactVersion: meta.TestdataVersion})
		Expect(err).NotTo(HaveOccurred())
		public, err := reader.Get(ctx, item.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(public.Title).To(Equal("Overview name"))
	})
	It("allows deleting an unreferenced publication without orphaning its version records", func(ctx SpecContext) {
		_, _, err := writer.SaveTestdata(as(ctx, owner), item.ID, makePackage(map[string]string{"1.in": "1\n", "1.out": "1\n"}), "diff")
		Expect(err).NotTo(HaveOccurred())
		meta, err := packages.Meta(as(ctx, owner), item.ID)
		Expect(err).NotTo(HaveOccurred())
		_, err = packages.Publish(as(ctx, owner), item.ID, authoringdomain.PublishInput{Revision: meta.PackageRevision, ArtifactVersion: meta.TestdataVersion})
		Expect(err).NotTo(HaveOccurred())
		Expect(writer.Delete(as(ctx, owner), item.ID)).To(Succeed())
		_, err = reader.Get(ctx, item.ID)
		Expect(err).To(MatchError(problemdomain.ErrNotFound))
		var versions int
		Expect(integrationDB.Pool.GetContext(ctx, &versions, "SELECT count(*) FROM problem_versions WHERE problem_id=$1", item.ID)).To(Succeed())
		Expect(versions).To(BeZero())
	})
})

func makePackage(entries map[string]string) []byte {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, body := range entries {
		file, err := writer.Create(name)
		Expect(err).NotTo(HaveOccurred())
		_, err = file.Write([]byte(body))
		Expect(err).NotTo(HaveOccurred())
	}
	Expect(writer.Close()).To(Succeed())
	return buffer.Bytes()
}
