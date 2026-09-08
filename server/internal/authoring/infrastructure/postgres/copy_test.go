package postgres_test

import (
	"context"
	"errors"
	authoringpg "github.com/RimuruChan/Vertex/server/internal/authoring/infrastructure/postgres"
	"os"
	"path/filepath"
	"time"

	authoringapp "github.com/RimuruChan/Vertex/server/internal/authoring/application"
	authoringdomain "github.com/RimuruChan/Vertex/server/internal/authoring/domain"
	authoringfiles "github.com/RimuruChan/Vertex/server/internal/authoring/infrastructure/filesystem"
	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	identitypg "github.com/RimuruChan/Vertex/server/internal/identity/infrastructure/postgres"
	judgepg "github.com/RimuruChan/Vertex/server/internal/judge/infrastructure/postgres"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/problem/domain"
	problemfiles "github.com/RimuruChan/Vertex/server/internal/problem/infrastructure/filesystem"
	problempg "github.com/RimuruChan/Vertex/server/internal/problem/infrastructure/postgres"
	submissiondomain "github.com/RimuruChan/Vertex/server/internal/submission/domain"
	submission "github.com/RimuruChan/Vertex/server/internal/submission/infrastructure/postgres"
	tenancyapp "github.com/RimuruChan/Vertex/server/internal/tenancy/application"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/tenancy/infrastructure/postgres"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Independent copies against PostgreSQL", func() {
	var packages *authoringpg.PackageRepository
	var service *authoringapp.Service
	var spaces *tenancyapp.Service
	var writer *problempg.Repository
	var source, target tenancydomain.Scope
	var users map[string]string
	var group tenancydomain.Group
	var item *problemdomain.Problem
	var root string
	var artifact *authoringdomain.PackageUpload
	as := func(ctx context.Context, space tenancydomain.Scope, actor string) context.Context {
		return tenancydomain.WithScope(ctx, tenancydomain.Scope{Domain: space.Domain, UserID: users[actor]})
	}
	input := func() authoringdomain.CopyInput {
		return authoringdomain.CopyInput{SourceDomain: source.Domain.Slug, SourceProblem: item.PublicID, SourceVersion: 1, Attribution: "Copy approved for training."}
	}
	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		users = map[string]string{}
		for _, name := range []string{"setter", "manager", "copier", "outsider"} {
			u, err := identitypg.NewUserRepository(integrationDB).Create(ctx, name, name+"@example.test", "fixture")
			Expect(err).NotTo(HaveOccurred())
			users[name] = u.ID
		}
		spaces = tenancyapp.NewService(tenancypg.NewRepository(integrationDB))
		var err error
		source, err = spaces.Create(ctx, users["setter"], tenancydomain.CreateInput{Slug: "source", Name: "Source"})
		Expect(err).NotTo(HaveOccurred())
		target, err = spaces.Create(ctx, users["manager"], tenancydomain.CreateInput{Slug: "target", Name: "Target"})
		Expect(err).NotTo(HaveOccurred())
		for _, actor := range []string{"copier", "outsider"} {
			Expect(spaces.SetMember(ctx, "source", users["setter"], tenancydomain.MemberInput{Username: actor, RoleKey: "member", Status: "active"})).To(Succeed())
			Expect(spaces.SetMember(ctx, "target", users["manager"], tenancydomain.MemberInput{Username: actor, RoleKey: "author", Status: "active"})).To(Succeed())
		}
		group, err = spaces.CreateGroup(ctx, "source", users["setter"], tenancydomain.GroupInput{Name: "Reviewers"})
		Expect(err).NotTo(HaveOccurred())
		Expect(spaces.SetGroupMember(ctx, "source", users["setter"], group.ID, "copier", "member", false)).To(Succeed())
		root = GinkgoT().TempDir()
		writer = problempg.NewRepository(integrationDB, problemfiles.NewTestdataStorage(root))
		item, err = writer.Create(as(ctx, source, "setter"), users["setter"], &problemdomain.CreateInput{Title: "Released", Visibility: "public", Tags: []string{"copied-tag"}})
		Expect(err).NotTo(HaveOccurred())
		Expect(writer.SetGrant(as(ctx, source, "setter"), item.ID, problemdomain.GrantInput{Group: group.ID, Role: problemdomain.AccessReader})).To(Succeed())
		packages = authoringpg.NewPackageRepository(integrationDB)
		builds := authoringpg.NewBuildRepository(integrationDB)
		publisher := authoringfiles.NewTestdataPublisher(root)
		service, err = authoringapp.NewService(packages, builds, publisher, authoringapp.NewDispatcher(1), time.Minute, time.Second)
		Expect(err).NotTo(HaveOccurred())
		_, err = packages.SaveStatement(as(ctx, source, "setter"), authoringdomain.Statement{ProblemID: item.ID, Language: "zh", Name: "Released title", Legend: "Released body", Tutorial: "Internal explanation"})
		Expect(err).NotTo(HaveOccurred())
		for _, file := range []authoringdomain.File{
			{Kind: authoringdomain.KindSolution, Name: "main", Language: "cpp", SourceCode: "released solution", IsActive: true},
			{Kind: authoringdomain.KindValidator, Name: "inactive", Language: "cpp", SourceCode: "released inactive validator"},
		} {
			file.ProblemID = item.ID
			_, err = packages.SaveFile(as(ctx, source, "setter"), file)
			Expect(err).NotTo(HaveOccurred())
		}
		_, err = packages.CreateTest(as(ctx, source, "setter"), authoringdomain.Test{ProblemID: item.ID, Source: authoringdomain.TestManual, InputData: "3\n", IsSample: true})
		Expect(err).NotTo(HaveOccurred())
		_, err = builds.Enqueue(as(ctx, source, "setter"), item.ID, users["setter"])
		Expect(err).NotTo(HaveOccurred())
		job, _, err := builds.Claim(ctx, "copy-fixture-worker", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		artifact, err = publisher.Publish(item.ID, makePackage(map[string]string{"1.in": "3\n", "1.out": "4\n"}))
		Expect(err).NotTo(HaveOccurred())
		Expect(builds.RecordPackage(ctx, job.ID, item.ID, job.WorkerID, job.LeaseToken, *artifact)).To(Succeed())
		Expect(builds.Complete(ctx, authoringdomain.BuildResult{BuildID: job.ID, WorkerID: job.WorkerID, LeaseToken: job.LeaseToken, Success: true, Tests: []authoringdomain.TestOutcome{{Index: 1, Source: authoringdomain.TestManual, Status: "ok", IsSample: true, InputHead: "3\n", AnswerHead: "4\n"}}}, "diff")).To(Succeed())
		meta, err := packages.Meta(as(ctx, source, "setter"), item.ID)
		Expect(err).NotTo(HaveOccurred())
		_, err = packages.Publish(as(ctx, source, "setter"), item.ID, authoringdomain.PublishInput{Revision: meta.PackageRevision, ArtifactVersion: meta.TestdataVersion})
		Expect(err).NotTo(HaveOccurred())
	})
	It("copies the selected release, not later edits, and survives source deletion", func(ctx SpecContext) {
		_, err := packages.SaveStatement(as(ctx, source, "setter"), authoringdomain.Statement{ProblemID: item.ID, Language: "zh", Name: "Unpublished title", Legend: "Unpublished body"})
		Expect(err).NotTo(HaveOccurred())
		_, err = packages.SaveFile(as(ctx, source, "setter"), authoringdomain.File{ProblemID: item.ID, Kind: authoringdomain.KindSolution, Name: "main", Language: "cpp", SourceCode: "unpublished solution", IsActive: true})
		Expect(err).NotTo(HaveOccurred())
		copy, err := service.Copy(as(ctx, target, "copier"), input())
		Expect(err).NotTo(HaveOccurred())
		Expect(copy.DomainID).To(Equal(target.Domain.ID))
		Expect(copy.ProblemID).NotTo(Equal(item.ID))
		preview, err := problempg.NewQueries(integrationDB).GetWorkspace(as(ctx, target, "copier"), copy.ProblemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(preview.OwnerID).To(Equal(users["copier"]))
		Expect(preview.Visibility).To(Equal("draft"))
		Expect(preview.PublishedVersion).To(BeZero())
		Expect(preview.Title).To(Equal("Released title"))
		Expect(preview.Tags).To(ConsistOf("copied-tag"))
		Expect(preview.StatementMD).To(ContainSubstring("Released body"))
		files, err := packages.Files(as(ctx, target, "copier"), copy.ProblemID, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(files).To(HaveLen(2))
		Expect(files[0].SourceCode + files[1].SourceCode).NotTo(ContainSubstring("unpublished"))
		Expect(files[0].SourceCode + files[1].SourceCode).To(ContainSubstring("released inactive validator"))
		grants, err := problempg.NewQueries(integrationDB).Grants(as(ctx, target, "copier"), copy.ProblemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(grants).To(BeEmpty())
		meta, err := packages.Meta(as(ctx, target, "copier"), copy.ProblemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(meta.TestdataCases).To(Equal(1))
		Expect(meta.DataRevision).To(Equal(meta.BuiltRevision))
		Expect(writer.Delete(as(ctx, source, "setter"), item.ID)).To(Succeed())
		Expect(os.ReadFile(filepath.Join(root, copy.ProblemID, artifact.SHA256, "1.out"))).To(Equal([]byte("4\n")))
		released, err := packages.Publish(as(ctx, target, "copier"), copy.ProblemID, authoringdomain.PublishInput{Revision: meta.PackageRevision, ArtifactVersion: meta.TestdataVersion})
		Expect(err).NotTo(HaveOccurred())
		Expect(released.Version).To(Equal(1))
		live, err := problempg.NewQueries(integrationDB).Get(as(ctx, target, "copier"), copy.ProblemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(live.StatementMD).To(ContainSubstring("## 样例"))
		sub, err := submission.NewRepository(integrationDB).Create(as(ctx, target, "copier"), &submissiondomain.Submission{UserID: users["copier"], ProblemID: copy.ProblemID, Language: "cpp", SourceCode: "int main(){}"})
		Expect(err).NotTo(HaveOccurred())
		job, err := judgepg.NewJobRepository(integrationDB).Claim(ctx, "copy-judge-worker", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		Expect(job.SubmissionID).To(Equal(sub.ID))
		Expect(job.DomainID).To(Equal(target.Domain.ID))
		Expect(job.ProblemVersion).To(Equal(1))
		Expect(job.Testdata.StoragePath).To(Equal(copy.ProblemID + "/" + artifact.SHA256))
		origin, err := service.Origin(as(ctx, target, "copier"), copy.ProblemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(origin.SourceProblemID).To(Equal(item.ID))
		Expect(origin.SourceVersion).To(Equal(1))
		second, err := service.Copy(as(ctx, target, "copier"), authoringdomain.CopyInput{SourceDomain: target.Domain.Slug, SourceProblem: copy.PublicID, SourceVersion: 1, Attribution: "Second training copy."})
		Expect(err).NotTo(HaveOccurred())
		Expect(second.PublicID).NotTo(Equal(copy.PublicID))
		Expect(second.Origin.Attribution).To(ContainSubstring("Copy approved for training."))
		Expect(second.Origin.Attribution).To(ContainSubstring("Second training copy."))
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE problem_origins SET attribution='tampered' WHERE problem_id=$1", copy.ProblemID)
		Expect(err).To(HaveOccurred())
	})
	It("requires package access plus destination creation and allows copying from an archived source", func(ctx SpecContext) {
		_, err := service.Copy(as(ctx, target, "outsider"), input())
		Expect(err).To(MatchError(tenancydomain.ErrForbidden))
		Expect(spaces.SetMember(ctx, "target", users["manager"], tenancydomain.MemberInput{Username: "copier", RoleKey: "viewer", Status: "active"})).To(Succeed())
		_, err = service.Copy(as(ctx, target, "copier"), input())
		Expect(err).To(MatchError(tenancydomain.ErrForbidden))
		Expect(spaces.SetMember(ctx, "target", users["manager"], tenancydomain.MemberInput{Username: "copier", RoleKey: "author", Status: "active"})).To(Succeed())
		Expect(spaces.Archive(ctx, "source", users["setter"], true)).To(Succeed())
		_, err = service.Copy(as(ctx, target, "copier"), input())
		Expect(err).NotTo(HaveOccurred())
		Expect(spaces.Archive(ctx, "target", users["manager"], true)).To(Succeed())
		_, err = service.Copy(as(ctx, target, "copier"), input())
		Expect(err).To(MatchError(tenancydomain.ErrForbidden))
	})
	It("rechecks target roles and source group grants after waiting on domain governance", func(ctx SpecContext) {
		for _, revokeSource := range []bool{false, true} {
			lockedDomain := target.Domain.ID
			if revokeSource {
				lockedDomain = source.Domain.ID
			}
			blocker, err := integrationDB.Pool.BeginTxx(ctx, nil)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = blocker.Rollback() })
			var pid int
			Expect(blocker.GetContext(ctx, &pid, "SELECT pg_backend_pid()")).To(Succeed())
			_, err = blocker.ExecContext(ctx, "SELECT 1 FROM domains WHERE id=$1 FOR UPDATE", lockedDomain)
			Expect(err).NotTo(HaveOccurred())
			done := make(chan error, 1)
			copyContext, cancel := context.WithTimeout(as(ctx, target, "copier"), 5*time.Second)
			DeferCleanup(cancel)
			go func() { _, err := service.Copy(copyContext, input()); done <- err }()
			Eventually(func() bool {
				var waiting bool
				err := integrationDB.Pool.GetContext(ctx, &waiting, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND $1=ANY(pg_blocking_pids(pid)))", pid)
				return err == nil && waiting
			}).WithTimeout(3 * time.Second).Should(BeTrue())
			if revokeSource {
				_, err = blocker.ExecContext(ctx, "DELETE FROM domain_group_members WHERE group_id=$1 AND user_id=$2", group.ID, users["copier"])
			} else {
				_, err = blocker.ExecContext(ctx, "UPDATE domain_members SET role_key='viewer' WHERE domain_id=$1 AND user_id=$2", target.Domain.ID, users["copier"])
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(blocker.Commit()).To(Succeed())
			Eventually(done).WithTimeout(3 * time.Second).Should(Receive(MatchError(tenancydomain.ErrForbidden)))
			Expect(spaces.SetMember(ctx, "target", users["manager"], tenancydomain.MemberInput{Username: "copier", RoleKey: "author", Status: "active"})).To(Succeed())
		}
		var count int
		Expect(integrationDB.Pool.GetContext(ctx, &count, "SELECT count(*) FROM problems WHERE domain_id=$1", target.Domain.ID)).To(Succeed())
		Expect(count).To(BeZero())
	})
	It("rolls back corrupt file copies and removes files when a later SQL write fails", func(ctx SpecContext) {
		dataFile := filepath.Join(root, filepath.FromSlash(artifact.StoragePath), "1.out")
		Expect(os.WriteFile(dataFile, []byte("bad"), 0644)).To(Succeed())
		_, err := service.Copy(as(ctx, target, "copier"), input())
		Expect(err).To(MatchError(authoringdomain.ErrPackageTarget))
		Expect(os.WriteFile(dataFile, []byte("4\n"), 0644)).To(Succeed())
		_, err = integrationDB.Pool.ExecContext(ctx, `CREATE FUNCTION reject_copy_fixture() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'copy fixture failure'; END $$;
CREATE TRIGGER reject_copy_fixture BEFORE INSERT ON problem_origins FOR EACH ROW EXECUTE FUNCTION reject_copy_fixture();`)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_, err := integrationDB.Pool.ExecContext(context.Background(), "DROP TRIGGER reject_copy_fixture ON problem_origins; DROP FUNCTION reject_copy_fixture();")
			Expect(err).NotTo(HaveOccurred())
		})
		_, err = service.Copy(as(ctx, target, "copier"), input())
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, authoringdomain.ErrPackageTarget)).To(BeFalse())
		var count int
		Expect(integrationDB.Pool.GetContext(ctx, &count, "SELECT count(*) FROM problems WHERE domain_id=$1", target.Domain.ID)).To(Succeed())
		Expect(count).To(BeZero())
		entries, err := os.ReadDir(root)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
		Expect(entries[0].Name()).To(Equal(item.ID))
	})
})
