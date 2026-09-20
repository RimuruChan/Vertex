package postgres_test

import (
	"context"
	"errors"
	authoringfiles "github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/filesystem"
	authoringpg "github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres"
	evaluationpg "github.com/RimuruChan/Vertex/server/internal/workflows/evaluation/postgres"
	"io"
	"os"
	"path/filepath"
	"time"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	identitypg "github.com/RimuruChan/Vertex/server/internal/modules/identity/infrastructure/postgres"
	judgepg "github.com/RimuruChan/Vertex/server/internal/modules/judge/infrastructure/postgres"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
	problempg "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres"
	submissiondomain "github.com/RimuruChan/Vertex/server/internal/modules/submission/domain"
	submission "github.com/RimuruChan/Vertex/server/internal/modules/submission/infrastructure/postgres"
	tenancyapp "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/application"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/platform/database/dbtest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Independent copies against PostgreSQL", func() {
	var fixture *authoringFixture
	var spaces *tenancyapp.Service
	var writer *problempg.Repository
	var source, target tenancydomain.Scope
	var users map[string]string
	var group tenancydomain.Group
	var item *problemdomain.ProblemView
	var root string
	var artifactPath string
	as := func(ctx context.Context, space tenancydomain.Scope, actor string) context.Context {
		return tenancydomain.WithScope(ctx, tenancydomain.Scope{Domain: space.Domain, UserID: users[actor]})
	}
	input := func() authoringdomain.CopyInput {
		return authoringdomain.CopyInput{SourceDomain: source.Domain.Slug, SourceProblem: item.PublicID, SourceVersion: 1, Attribution: "Copy approved for training."}
	}
	BeforeEach(func(spec SpecContext) {
		ctx := dbtest.Context(spec)
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
		writer = problempg.NewRepository(integrationDB)
		item, err = writer.Create(as(ctx, source, "setter"), users["setter"], &problemdomain.CreateInput{Title: "Released title", StatementMD: "Released body", Visibility: "public", Tags: []string{"copied-tag"}})
		Expect(err).NotTo(HaveOccurred())
		Expect(writer.SetGrant(as(ctx, source, "setter"), item.ID, problemdomain.GrantInput{Group: group.ID, Role: problemdomain.AccessReader})).To(Succeed())
		fixture = newAuthoringFixture(as(ctx, source, "setter"), item.ID, root)
		fixture.save("unused", "solutions/unused.cpp", authoringdomain.EntrySource, "released unused source")
		checkID := fixture.checked()
		revision := fixture.commit("source-release")
		_, err = fixture.repo.PublishCommit(fixture.ctx, item.ID, authoringdomain.CommitPublication{Revision: revision, CheckID: checkID, ExpectedVersion: 0})
		Expect(err).NotTo(HaveOccurred())
		Expect(integrationDB.Pool.GetContext(ctx, &artifactPath, "SELECT package_path FROM problem_build_jobs WHERE id=$1", checkID)).To(Succeed())

	})
	It("copies a published tree without private edits and survives source deletion", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		fixture.ctx = as(ctx, source, "setter")
		fixture.save("main", "solutions/main.cpp", authoringdomain.EntrySource, "unpublished solution")
		copy, err := fixture.repo.CopyRelease(as(ctx, target, "copier"), input())
		Expect(err).NotTo(HaveOccurred())
		Expect(copy.DomainID).To(Equal(target.Domain.ID))
		Expect(copy.ProblemID).NotTo(Equal(item.ID))
		copiedContext := as(ctx, target, "copier")
		preview, err := problempg.NewQueries(integrationDB).GetWorkspace(copiedContext, copy.ProblemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(preview.OwnerID).To(Equal(users["copier"]))
		Expect(preview.Visibility).To(Equal("private"))
		Expect(preview.PublishedVersion).To(BeZero())
		Expect(preview.Title).To(Equal("Released title"))
		working, err := fixture.repo.WorkingCopy(copiedContext, copy.ProblemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(working.HeadRevision).To(BeNil())
		sources := ""
		for _, entry := range working.Tree.Entries {
			if entry.Kind != authoringdomain.EntrySource {
				continue
			}
			_, reader, err := fixture.repo.Blob(copiedContext, copy.ProblemID, entry.Blob.SHA256)
			Expect(err).NotTo(HaveOccurred())
			data, err := io.ReadAll(reader)
			Expect(reader.Close()).To(Succeed())
			Expect(err).NotTo(HaveOccurred())
			sources += string(data)
		}
		Expect(sources).NotTo(ContainSubstring("unpublished"))
		Expect(sources).To(ContainSubstring("released unused source"))
		grants, err := problempg.NewQueries(integrationDB).Grants(copiedContext, copy.ProblemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(grants).To(BeEmpty())
		checks, err := fixture.repo.Checks(copiedContext, copy.ProblemID, 10)
		Expect(err).NotTo(HaveOccurred())
		Expect(checks).To(HaveLen(1))
		Expect(checks[0].Stage).To(Equal("copied"))
		Expect(writer.Delete(as(ctx, source, "setter"), item.ID)).To(Succeed())
		commit, err := fixture.repo.Commit(copiedContext, copy.ProblemID, authoringdomain.CommitInput{ETag: working.ETag, Message: "Adopt source", RequestID: "copy-release"})
		Expect(err).NotTo(HaveOccurred())
		released, err := fixture.repo.PublishCommit(copiedContext, copy.ProblemID, authoringdomain.CommitPublication{Revision: commit.Commit.Revision, CheckID: checks[0].ID, ExpectedVersion: 0})
		Expect(err).NotTo(HaveOccurred())
		Expect(released.Version).To(Equal(1))
		sub, err := submission.NewRepository(integrationDB, evaluationpg.Rebuild).Create(copiedContext, &submissiondomain.Submission{UserID: users["copier"], ProblemID: copy.ProblemID, Language: "cpp", SourceCode: "int main(){}"})
		Expect(err).NotTo(HaveOccurred())
		job, err := judgepg.NewJobRepository(integrationDB, evaluationpg.Rebuild).Claim(ctx, "copy-judge-worker", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		Expect(job.SubmissionID).To(Equal(sub.ID))
		Expect(job.DomainID).To(Equal(target.Domain.ID))
		Expect(job.ProblemVersion).To(Equal(1))
		Expect(job.Testdata.StoragePath).To(HavePrefix(copy.ProblemID + "/"))
		origin, err := fixture.repo.Origin(copiedContext, copy.ProblemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(origin.SourceProblemID).To(Equal(item.ID))
		Expect(origin.SourceVersion).To(Equal(1))
		second, err := fixture.repo.CopyRelease(copiedContext, authoringdomain.CopyInput{SourceDomain: target.Domain.Slug, SourceProblem: copy.PublicID, SourceVersion: 1, Attribution: "Second training copy."})
		Expect(err).NotTo(HaveOccurred())
		Expect(second.PublicID).NotTo(Equal(copy.PublicID))
		Expect(second.Origin.Attribution).To(ContainSubstring("Copy approved for training."))
		Expect(second.Origin.Attribution).To(ContainSubstring("Second training copy."))
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE problem_origins SET attribution='tampered' WHERE problem_id=$1", copy.ProblemID)
		Expect(err).To(HaveOccurred())
	})
	It("requires package access plus destination creation and allows copying from an archived source", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		_, err := fixture.repo.CopyRelease(as(ctx, target, "outsider"), input())
		Expect(err).To(MatchError(tenancydomain.ErrForbidden))
		Expect(spaces.SetMember(ctx, "target", users["manager"], tenancydomain.MemberInput{Username: "copier", RoleKey: "viewer", Status: "active"})).To(Succeed())
		_, err = fixture.repo.CopyRelease(as(ctx, target, "copier"), input())
		Expect(err).To(MatchError(tenancydomain.ErrForbidden))
		Expect(spaces.SetMember(ctx, "target", users["manager"], tenancydomain.MemberInput{Username: "copier", RoleKey: "author", Status: "active"})).To(Succeed())
		Expect(spaces.Archive(ctx, "source", users["setter"], true)).To(Succeed())
		_, err = fixture.repo.CopyRelease(as(ctx, target, "copier"), input())
		Expect(err).NotTo(HaveOccurred())
		Expect(spaces.Archive(ctx, "target", users["manager"], true)).To(Succeed())
		_, err = fixture.repo.CopyRelease(as(ctx, target, "copier"), input())
		Expect(err).To(MatchError(tenancydomain.ErrForbidden))
	})
	It("rechecks target roles and source group grants after waiting on domain governance", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
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
			go func() { _, err := fixture.repo.CopyRelease(copyContext, input()); done <- err }()
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
	It("rejects corrupt artifacts and rolls back a later SQL failure", func(spec SpecContext) {
		ctx := dbtest.Context(spec)
		dataFile := filepath.Join(root, filepath.FromSlash(artifactPath), "1.out")
		Expect(os.WriteFile(dataFile, []byte("bad"), 0644)).To(Succeed())
		_, err := fixture.repo.CopyRelease(as(ctx, target, "copier"), input())
		Expect(err).To(MatchError(authoringdomain.ErrPackageTarget))
		Expect(os.WriteFile(dataFile, []byte("fixture answer"), 0644)).To(Succeed())
		_, err = integrationDB.Pool.ExecContext(ctx, `CREATE FUNCTION reject_copy_fixture() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'copy fixture failure'; END $$;
CREATE TRIGGER reject_copy_fixture BEFORE INSERT ON problem_origins FOR EACH ROW EXECUTE FUNCTION reject_copy_fixture();`)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_, err := integrationDB.Pool.ExecContext(dbtest.Context(), "DROP TRIGGER reject_copy_fixture ON problem_origins; DROP FUNCTION reject_copy_fixture();")
			Expect(err).NotTo(HaveOccurred())
		})
		_, err = fixture.repo.CopyRelease(as(ctx, target, "copier"), input())
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, authoringdomain.ErrPackageTarget)).To(BeFalse())
		var count int
		Expect(integrationDB.Pool.GetContext(ctx, &count, "SELECT count(*) FROM problems WHERE domain_id=$1", target.Domain.ID)).To(Succeed())
		Expect(count).To(BeZero())
		// Failed transactions leave no references. Deferred reclamation must
		// remove their independent files while preserving the released source.
		collector := authoringpg.NewGarbageRepository(integrationDB, authoringfiles.NewGarbageStorage(fixture.blobs, fixture.publisher))
		_, err = collector.Collect(ctx, time.Now().Add(time.Hour), "", 100)
		Expect(err).NotTo(HaveOccurred())
		entries, err := os.ReadDir(root)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
		Expect(entries[0].Name()).To(Equal(item.ID))
	})
})
