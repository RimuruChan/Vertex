package authoring

import (
	"time"

	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var integrationDB *database.DB
var releaseSuite = func() {}

var _ = BeforeSuite(func(ctx SpecContext) {
	var err error
	integrationDB, releaseSuite, err = dbtest.Shared(ctx)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	releaseSuite()
})

// The package and build stores carry the most SQL in this domain: revision
// bookkeeping, a fenced claim, and a publish that writes three tables at once.
var _ = Describe("Authoring stores against PostgreSQL", func() {
	var packages *PackageStore
	var builds *BuildStore
	var problemID, authorID string

	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		_, err := integrationDB.Pool.ExecContext(ctx, `
			TRUNCATE problem_build_jobs, problem_tests, problem_files, problem_statements,
				problem_testdata, submissions, problems, users
			RESTART IDENTITY CASCADE`)
		Expect(err).NotTo(HaveOccurred())
		packages = NewPackageStore(integrationDB)
		builds = NewBuildStore(integrationDB)

		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO users (username, email, password_hash) VALUES ('setter', 's@t.local', 'x')
			 RETURNING id`).Scan(&authorID)).To(Succeed())
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO problems (title, visibility) VALUES ('Sum', 'draft') RETURNING id`).
			Scan(&problemID)).To(Succeed())
	})

	It("bumps the package revision on every edit", func(ctx SpecContext) {
		meta, err := packages.Meta(ctx, problemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(meta.PackageRevision).To(Equal(0))
		Expect(meta.BuiltRevision).To(Equal(0))
		Expect(meta.StatementLanguage).To(Equal("zh"))

		_, err = packages.SaveStatement(ctx, Statement{
			ProblemID: problemID, Language: "zh", Name: "求和", Legend: "给定 n 个整数",
			InputFormat: "一行", OutputFormat: "一行",
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = packages.SaveFile(ctx, File{
			ProblemID: problemID, Kind: KindSolution, Name: "std", Language: "cpp",
			SourceCode: "int main(){}", IsActive: true, ExpectedVerdict: "Accepted",
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = packages.CreateTest(ctx, Test{
			ProblemID: problemID, Source: TestManual, InputData: "1\n", IsSample: true,
		})
		Expect(err).NotTo(HaveOccurred())

		meta, err = packages.Meta(ctx, problemID)
		Expect(err).NotTo(HaveOccurred())
		// Three edits, three revisions; the built revision has not moved.
		Expect(meta.PackageRevision).To(Equal(3))
		Expect(meta.BuiltRevision).To(Equal(0))

		// Saving a statement also rewrites the public Markdown.
		var statement string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`SELECT statement_md FROM problems WHERE id = $1`, problemID).Scan(&statement)).To(Succeed())
		Expect(statement).To(ContainSubstring("## 题目描述"))
		Expect(statement).To(ContainSubstring("给定 n 个整数"))
	})

	It("keeps exactly one active file per kind", func(ctx SpecContext) {
		for _, name := range []string{"std", "faster"} {
			_, err := packages.SaveFile(ctx, File{
				ProblemID: problemID, Kind: KindSolution, Name: name, Language: "cpp",
				SourceCode: "int main(){}", IsActive: true,
			})
			Expect(err).NotTo(HaveOccurred())
		}
		files, err := packages.Files(ctx, problemID, false)
		Expect(err).NotTo(HaveOccurred())
		active := 0
		for _, file := range files {
			if file.IsActive {
				active++
				// The most recent save wins.
				Expect(file.Name).To(Equal("faster"))
			}
		}
		Expect(active).To(Equal(1))
	})

	It("renumbers tests after a delete and reorders without collisions", func(ctx SpecContext) {
		for i := 0; i < 3; i++ {
			_, err := packages.CreateTest(ctx, Test{
				ProblemID: problemID, Source: TestManual, InputData: "x\n",
			})
			Expect(err).NotTo(HaveOccurred())
		}
		tests, err := packages.Tests(ctx, problemID, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(tests).To(HaveLen(3))
		Expect(tests[0].Index).To(Equal(1))

		Expect(packages.DeleteTest(ctx, problemID, tests[0].ID)).To(Succeed())
		tests, err = packages.Tests(ctx, problemID, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(tests).To(HaveLen(2))
		// The gap closed: the judge testdata layout requires 1..N.
		Expect(tests[0].Index).To(Equal(1))
		Expect(tests[1].Index).To(Equal(2))

		Expect(packages.ReorderTest(ctx, problemID, tests[1].ID, 1)).To(Succeed())
		tests, err = packages.Tests(ctx, problemID, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(tests[0].Index).To(Equal(1))
		Expect(tests[1].Index).To(Equal(2))
	})

	It("runs one build at a time and publishes only on success", func(ctx SpecContext) {
		_, err := packages.SaveFile(ctx, File{
			ProblemID: problemID, Kind: KindSolution, Name: "std", Language: "cpp",
			SourceCode: "int main(){}", IsActive: true,
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = packages.CreateTest(ctx, Test{
			ProblemID: problemID, Source: TestManual, InputData: "1\n", IsSample: true,
		})
		Expect(err).NotTo(HaveOccurred())

		queued, err := builds.Enqueue(ctx, problemID, authorID)
		Expect(err).NotTo(HaveOccurred())
		Expect(queued.State).To(Equal(BuildQueued))

		// A second request returns the running build instead of queueing another.
		existing, err := builds.Enqueue(ctx, problemID, authorID)
		Expect(err).To(MatchError(ErrBuildRunning))
		Expect(existing.ID).To(Equal(queued.ID))

		claimed, pkg, err := builds.Claim(ctx, "worker-1", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).NotTo(BeNil())
		Expect(claimed.State).To(Equal(BuildRunning))
		Expect(pkg.MainSolution()).NotTo(BeNil())
		Expect(pkg.Tests).To(HaveLen(1))

		// A stale worker cannot report progress.
		Expect(builds.Progress(ctx, Progress{
			BuildID: claimed.ID, WorkerID: "worker-2", LeaseToken: claimed.LeaseToken,
			Stage: StageGenerate, Done: 1, Total: 1,
		}, time.Minute)).To(MatchError(ErrStaleLease))
		Expect(builds.Progress(ctx, Progress{
			BuildID: claimed.ID, WorkerID: "worker-1", LeaseToken: claimed.LeaseToken,
			Stage: StageGenerate, Done: 1, Total: 1, Log: "生成中\n",
		}, time.Minute)).To(Succeed())

		target, err := builds.ResolvePackageTarget(ctx, claimed.ID, "worker-1", claimed.LeaseToken)
		Expect(err).NotTo(HaveOccurred())
		Expect(target).To(Equal(problemID))
		_, err = builds.ResolvePackageTarget(ctx, claimed.ID, "worker-2", claimed.LeaseToken)
		Expect(err).To(MatchError(ErrStaleLease))
		Expect(builds.RecordPackage(ctx, claimed.ID, problemID, "worker-1", claimed.LeaseToken,
			PackageUpload{StoragePath: "other-problem/hash", SHA256: "hash", CaseCount: 1})).
			To(MatchError(ErrPackageTarget))
		Expect(builds.RecordPackage(ctx, claimed.ID,
			"00000000-0000-0000-0000-000000000000", "worker-1", claimed.LeaseToken,
			PackageUpload{
				StoragePath: "00000000-0000-0000-0000-000000000000/hash",
				SHA256:      "hash", CaseCount: 1,
			})).
			To(MatchError(ErrStaleLease))
		Expect(builds.RecordPackage(ctx, claimed.ID, problemID, "worker-1", claimed.LeaseToken,
			PackageUpload{StoragePath: problemID + "/hash", SHA256: "hash", CaseCount: 1})).To(Succeed())

		sampleInput, sampleAnswer := "1\n", "1\n"
		Expect(builds.Complete(ctx, BuildResult{
			BuildID: claimed.ID, WorkerID: "worker-1", LeaseToken: claimed.LeaseToken,
			Success: true, Stage: StageDone,
			Tests: []TestOutcome{{
				Index: 1, Status: "ok", IsSample: true,
				InputHead: sampleInput, AnswerHead: sampleAnswer,
			}},
		}, "testlib")).To(Succeed())

		// Publishing wrote the testdata row, the built revision and the samples.
		meta, err := packages.Meta(ctx, problemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(meta.TestdataCases).To(Equal(1))
		Expect(meta.TestdataChecker).To(Equal("testlib"))
		Expect(meta.BuiltRevision).To(Equal(meta.PackageRevision))
		Expect(meta.LastBuiltAt).NotTo(BeNil())

		settled, err := builds.Get(ctx, problemID, claimed.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(settled.State).To(Equal(BuildSucceeded))
		Expect(settled.Log).To(ContainSubstring("生成中"))
		Expect(settled.Tests).To(HaveLen(1))

		// A later statement save re-renders using the samples this build produced.
		_, err = packages.SaveStatement(ctx, Statement{
			ProblemID: problemID, Language: "zh", Name: "求和", Legend: "正文",
		})
		Expect(err).NotTo(HaveOccurred())
		var statement string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`SELECT statement_md FROM problems WHERE id = $1`, problemID).Scan(&statement)).To(Succeed())
		Expect(statement).To(ContainSubstring("## 样例"))
	})

	It("does not publish testdata for a failed build", func(ctx SpecContext) {
		_, err := packages.CreateTest(ctx, Test{
			ProblemID: problemID, Source: TestManual, InputData: "1\n",
		})
		Expect(err).NotTo(HaveOccurred())
		queued, err := builds.Enqueue(ctx, problemID, authorID)
		Expect(err).NotTo(HaveOccurred())
		claimed, _, err := builds.Claim(ctx, "worker-1", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed.ID).To(Equal(queued.ID))

		Expect(builds.Complete(ctx, BuildResult{
			BuildID: claimed.ID, WorkerID: "worker-1", LeaseToken: claimed.LeaseToken,
			Success: false, ErrorMessage: "checker 编译失败",
		}, "diff")).To(Succeed())

		meta, err := packages.Meta(ctx, problemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(meta.TestdataCases).To(Equal(0))
		Expect(meta.BuiltRevision).To(Equal(0))

		settled, err := builds.Get(ctx, problemID, claimed.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(settled.State).To(Equal(BuildFailed))
		Expect(settled.ErrorMessage).To(Equal("checker 编译失败"))
	})

	It("treats a success without an artifact as a failure", func(ctx SpecContext) {
		_, err := packages.CreateTest(ctx, Test{
			ProblemID: problemID, Source: TestManual, InputData: "1\n",
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = builds.Enqueue(ctx, problemID, authorID)
		Expect(err).NotTo(HaveOccurred())
		claimed, _, err := builds.Claim(ctx, "worker-1", time.Minute)
		Expect(err).NotTo(HaveOccurred())

		// The worker never uploaded, so there is nothing to publish.
		Expect(builds.Complete(ctx, BuildResult{
			BuildID: claimed.ID, WorkerID: "worker-1", LeaseToken: claimed.LeaseToken,
			Success: true,
		}, "diff")).To(Succeed())

		settled, err := builds.Get(ctx, problemID, claimed.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(settled.State).To(Equal(BuildFailed))
		Expect(settled.ErrorMessage).To(ContainSubstring("没有上传测试数据"))
	})
})
