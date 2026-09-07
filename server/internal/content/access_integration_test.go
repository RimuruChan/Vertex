package content

import (
	"context"
	"errors"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/identity"
	"github.com/RimuruChan/Vertex/server/internal/problem"
	"github.com/jackc/pgx/v5/pgconn"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Content parent authorization against PostgreSQL", func() {
	var spaces *domain.Service
	var editorials *EditorialStore
	var posts *DiscussionStore
	var writer *problem.ProblemAdminStore
	var users map[string]string
	var scope domain.Scope
	var task *problem.Problem
	var item *Editorial
	var post *DiscussionPost
	as := func(ctx context.Context, name string) context.Context {
		return domain.WithScope(ctx, domain.Scope{Domain: scope.Domain, UserID: users[name]})
	}
	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		spaces = domain.NewService(domain.NewStore(integrationDB))
		editorials = NewEditorialStore(integrationDB)
		posts = NewDiscussionStore(integrationDB)
		writer = problem.NewProblemAdminStore(integrationDB, GinkgoT().TempDir())
		users = map[string]string{}
		for _, name := range []string{"manager", "setter", "author", "reader", "outsider"} {
			u, err := identity.NewUserStore(integrationDB).Create(ctx, name, name+"@example.test", "fixture")
			Expect(err).NotTo(HaveOccurred())
			users[name] = u.ID
		}
		var err error
		scope, err = spaces.Create(ctx, users["manager"], domain.CreateInput{Slug: "team", Name: "Team"})
		Expect(err).NotTo(HaveOccurred())
		for _, name := range []string{"setter", "author", "reader"} {
			role := "member"
			if name == "setter" {
				role = "author"
			}
			Expect(spaces.SetMember(ctx, "team", users["manager"], domain.MemberInput{Username: name, RoleKey: role, Status: "active"})).To(Succeed())
		}
		task, err = writer.Create(as(ctx, "setter"), users["setter"], &problem.CreateInput{Title: "Parent", Visibility: "public"})
		Expect(err).NotTo(HaveOccurred())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, task.ID)).To(Succeed())
		item, err = editorials.Create(as(ctx, "author"), users["author"], EditorialInput{ProblemID: task.ID, Title: "Solution", ContentMD: "private body"})
		Expect(err).NotTo(HaveOccurred())
		post, err = posts.CreateProblemPost(as(ctx, "author"), task.ID, users["author"], "author comment", nil)
		Expect(err).NotTo(HaveOccurred())
	})

	It("requires current domain creation rights but preserves edits to accessible authored work", func(ctx SpecContext) {
		Expect(spaces.SetMember(ctx, "team", users["manager"], domain.MemberInput{Username: "author", RoleKey: "viewer", Status: "active"})).To(Succeed())
		_, err := editorials.Create(as(ctx, "author"), users["author"], EditorialInput{ProblemID: task.ID, Title: "Denied", ContentMD: "x"})
		Expect(err).To(MatchError(ErrForbidden))
		_, err = posts.CreateProblemPost(as(ctx, "author"), task.ID, users["author"], "Denied", nil)
		Expect(err).To(MatchError(ErrForbidden))
		_, err = editorials.Vote(as(ctx, "author"), item.ID, users["author"], true)
		Expect(err).To(MatchError(ErrForbidden))
		updated, err := editorials.Update(as(ctx, "author"), item.ID, users["author"], EditorialInput{Title: "Edited", ContentMD: "updated", ProblemID: "ignored"})
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.ProblemID).To(Equal(task.ID))
		_, err = posts.Update(as(ctx, "author"), post.ID, users["author"], "Updated own comment")
		Expect(err).NotTo(HaveOccurred())
		thread, err := posts.ListByProblem(as(ctx, "author"), task.ID, users["author"])
		Expect(err).NotTo(HaveOccurred())
		Expect(thread.CanPost).To(BeFalse())
	})

	It("does not let authors bypass a private parent and restores only live group access", func(ctx SpecContext) {
		_, err := integrationDB.Pool.ExecContext(ctx, "UPDATE problems SET visibility='private' WHERE id=$1", task.ID)
		Expect(err).NotTo(HaveOccurred())
		_, err = editorials.Get(as(ctx, "author"), item.ID, users["author"])
		Expect(err).To(MatchError(ErrNotFound))
		_, err = posts.Get(as(ctx, "author"), post.ID, users["author"])
		Expect(err).To(MatchError(ErrNotFound))
		Expect(posts.Delete(as(ctx, "author"), post.ID, users["author"])).To(MatchError(ErrNotFound))
		list, total, err := editorials.List(as(ctx, "author"), EditorialFilters{ViewerID: users["author"], Admin: true})
		Expect(err).NotTo(HaveOccurred())
		Expect(list).To(BeEmpty())
		Expect(total).To(BeZero())
		group, err := spaces.CreateGroup(ctx, "team", users["manager"], domain.GroupInput{Name: "Reviewers"})
		Expect(err).NotTo(HaveOccurred())
		Expect(spaces.SetGroupMember(ctx, "team", users["manager"], group.ID, "author", "member", false)).To(Succeed())
		Expect(writer.SetGrant(as(ctx, "setter"), task.ID, problem.GrantInput{Group: group.ID, Role: problem.AccessReader})).To(Succeed())
		visible, err := editorials.Get(as(ctx, "author"), item.ID, users["author"])
		Expect(err).NotTo(HaveOccurred())
		Expect(visible.ContentMD).To(Equal("private body"))
		Expect(visible.Permissions.Edit).To(BeTrue())
		Expect(spaces.SetGroupMember(ctx, "team", users["manager"], group.ID, "author", "member", true)).To(Succeed())
		_, err = editorials.Get(as(ctx, "author"), item.ID, users["author"])
		Expect(err).To(MatchError(ErrNotFound))
	})

	It("keeps moderator deletion separate from rewriting and private drafts", func(ctx SpecContext) {
		for _, actor := range []string{"setter", "manager"} {
			visible, err := editorials.Get(as(ctx, actor), item.ID, users[actor])
			Expect(err).NotTo(HaveOccurred())
			Expect(visible.Permissions.Delete).To(BeTrue())
			Expect(visible.Permissions.Edit).To(BeFalse())
			_, err = editorials.Update(as(ctx, actor), item.ID, users[actor], EditorialInput{Title: "Tampered", ContentMD: "no"})
			Expect(err).To(MatchError(ErrForbidden))
			_, err = posts.Update(as(ctx, actor), post.ID, users[actor], "Tampered")
			Expect(err).To(MatchError(ErrForbidden))
		}
		_, err := editorials.Update(as(ctx, "author"), item.ID, users["author"], EditorialInput{Title: "Draft", ContentMD: "private", Status: StatusDraft, Visibility: VisibilityPrivate})
		Expect(err).NotTo(HaveOccurred())
		_, err = editorials.Get(as(ctx, "setter"), item.ID, users["setter"])
		Expect(err).To(MatchError(ErrNotFound))
		visible, err := editorials.Get(as(ctx, "manager"), item.ID, users["manager"])
		Expect(err).NotTo(HaveOccurred())
		Expect(visible.ContentMD).To(Equal("private"))
		Expect(visible.Permissions.Edit).To(BeFalse())
		Expect(editorials.Delete(as(ctx, "manager"), item.ID, users["manager"])).To(Succeed())
		Expect(posts.Delete(as(ctx, "setter"), post.ID, users["setter"])).To(Succeed())
		var audits int
		Expect(integrationDB.Pool.GetContext(ctx, &audits, "SELECT count(*) FROM domain_audit_events WHERE domain_id=$1 AND action IN ('editorial.delete','discussion.delete')", scope.Domain.ID)).To(Succeed())
		Expect(audits).To(Equal(2))
	})

	It("gates bodies, votes and discussion even after a formerly accepted result changes", func(ctx SpecContext) {
		_, err := editorials.Update(as(ctx, "author"), item.ID, users["author"], EditorialInput{Title: "Spoiler", ContentMD: "hidden answer", SolvedOnly: true})
		Expect(err).NotTo(HaveOccurred())
		visible, err := editorials.Get(as(ctx, "reader"), item.ID, users["reader"])
		Expect(err).NotTo(HaveOccurred())
		Expect(visible.Locked).To(BeTrue())
		Expect(visible.ContentMD).To(BeEmpty())
		_, err = posts.CreateEditorialPost(as(ctx, "reader"), item.ID, users["reader"], "bypass", nil)
		Expect(err).To(MatchError(ErrSpoilerLocked))
		_, err = editorials.Vote(as(ctx, "reader"), item.ID, users["reader"], true)
		Expect(err).To(MatchError(ErrSpoilerLocked))
		var submissionID string
		Expect(integrationDB.Pool.GetContext(ctx, &submissionID, `INSERT INTO submissions(domain_id,user_id,problem_id,language,source_code,status) VALUES($1,$2,$3,'cpp','fixture','Accepted') RETURNING id`, scope.Domain.ID, users["reader"], task.ID)).To(Succeed())
		visible, err = editorials.Get(as(ctx, "reader"), item.ID, users["reader"])
		Expect(err).NotTo(HaveOccurred())
		Expect(visible.Locked).To(BeFalse())
		Expect(visible.ContentMD).To(Equal("hidden answer"))
		reply, err := posts.CreateEditorialPost(as(ctx, "reader"), item.ID, users["reader"], "now visible", nil)
		Expect(err).NotTo(HaveOccurred())
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE submissions SET status='Wrong Answer' WHERE id=$1", submissionID)
		Expect(err).NotTo(HaveOccurred())
		_, err = posts.Get(as(ctx, "reader"), reply.ID, users["reader"])
		Expect(err).To(MatchError(ErrSpoilerLocked))
		_, err = posts.Update(as(ctx, "reader"), reply.ID, users["reader"], "stale author bypass")
		Expect(err).To(MatchError(ErrSpoilerLocked))
		list, _, err := editorials.List(as(ctx, "reader"), EditorialFilters{ViewerID: users["reader"], Admin: true})
		Expect(err).NotTo(HaveOccurred())
		Expect(list[0].Locked).To(BeTrue())
	})

	It("revokes authored powers for suspended members and makes archived domains read-only", func(ctx SpecContext) {
		Expect(spaces.SetMember(ctx, "team", users["manager"], domain.MemberInput{Username: "author", RoleKey: "member", Status: "suspended"})).To(Succeed())
		_, err := editorials.Get(as(ctx, "author"), item.ID, users["author"])
		Expect(err).To(MatchError(ErrNotFound))
		Expect(spaces.SetMember(ctx, "team", users["manager"], domain.MemberInput{Username: "author", RoleKey: "member", Status: "active"})).To(Succeed())
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE domains SET archived=true WHERE id=$1", scope.Domain.ID)
		Expect(err).NotTo(HaveOccurred())
		visible, err := editorials.Get(as(ctx, "author"), item.ID, users["author"])
		Expect(err).NotTo(HaveOccurred())
		Expect(visible.Permissions.Edit || visible.Permissions.Delete || visible.Permissions.Comment).To(BeFalse())
		Expect(posts.Delete(as(ctx, "author"), post.ID, users["author"])).To(MatchError(ErrForbidden))
	})

	It("checks reply scope atomically and rejects same-domain cross-thread parent foreign keys", func(ctx SpecContext) {
		other, err := writer.Create(as(ctx, "setter"), users["setter"], &problem.CreateInput{Title: "Other", Visibility: "public"})
		Expect(err).NotTo(HaveOccurred())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, other.ID)).To(Succeed())
		_, err = posts.CreateProblemPost(as(ctx, "reader"), other.ID, users["reader"], "wrong thread", &post.ID)
		Expect(err).To(MatchError(ErrInvalidInput))
		_, err = integrationDB.Pool.ExecContext(ctx, `INSERT INTO discussion_posts(domain_id,problem_id,author_id,content_md,parent_id) VALUES($1,$2,$3,'wrong thread',$4)`, scope.Domain.ID, other.ID, users["reader"], post.ID)
		var constraint *pgconn.PgError
		Expect(errors.As(err, &constraint)).To(BeTrue())
		Expect(constraint.Code).To(Equal("23503"))
		root, err := posts.CreateEditorialPost(as(ctx, "author"), item.ID, users["author"], "editorial root", nil)
		Expect(err).NotTo(HaveOccurred())
		reply, err := posts.CreateEditorialPost(as(ctx, "reader"), item.ID, users["reader"], "threaded reply", &root.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(*reply.ParentID).To(Equal(root.ID))
		peer, err := editorials.Create(as(ctx, "author"), users["author"], EditorialInput{ProblemID: task.ID, Title: "Another editorial", ContentMD: "other body"})
		Expect(err).NotTo(HaveOccurred())
		_, err = posts.CreateEditorialPost(as(ctx, "reader"), peer.ID, users["reader"], "wrong editorial", &root.ID)
		Expect(err).To(MatchError(ErrInvalidInput))
		_, err = integrationDB.Pool.ExecContext(ctx, `INSERT INTO discussion_posts(domain_id,editorial_id,author_id,content_md,parent_id) VALUES($1,$2,$3,'wrong thread',$4)`, scope.Domain.ID, peer.ID, users["reader"], root.ID)
		Expect(errors.As(err, &constraint)).To(BeTrue())
		Expect(constraint.Code).To(Equal("23503"))
	})

	It("rechecks problem grants after waiting for their revocation", func(ctx SpecContext) {
		_, err := integrationDB.Pool.ExecContext(ctx, "UPDATE problems SET visibility='private' WHERE id=$1", task.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(writer.SetGrant(as(ctx, "setter"), task.ID, problem.GrantInput{Username: "reader", Role: problem.AccessReader})).To(Succeed())
		tx, err := integrationDB.Pool.BeginTxx(ctx, nil)
		Expect(err).NotTo(HaveOccurred())
		defer tx.Rollback()
		_, err = problem.LockAccess(as(ctx, "setter"), tx, task.ID, users["setter"])
		Expect(err).NotTo(HaveOccurred())
		_, err = tx.ExecContext(ctx, "DELETE FROM problem_access WHERE problem_id=$1", task.ID)
		Expect(err).NotTo(HaveOccurred())
		done := make(chan error, 1)
		go func() {
			_, err := posts.CreateProblemPost(as(ctx, "reader"), task.ID, users["reader"], "stale", nil)
			done <- err
		}()
		Eventually(func() (int, error) {
			var n int
			err := integrationDB.Pool.GetContext(ctx, &n, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT pg_advisory_xact_lock_shared%'`)
			return n, err
		}, 3*time.Second).Should(BeNumerically(">", 0))
		Expect(tx.Commit()).To(Succeed())
		Eventually(done, 5*time.Second).Should(Receive(MatchError(ErrNotFound)))
	})

	It("rechecks editorial visibility after a competing writer commits", func(ctx SpecContext) {
		tx, err := integrationDB.Pool.BeginTxx(ctx, nil)
		Expect(err).NotTo(HaveOccurred())
		defer tx.Rollback()
		_, err = lockEditorial(as(ctx, "author"), tx, item.ID, users["author"])
		Expect(err).NotTo(HaveOccurred())
		_, err = tx.ExecContext(ctx, "UPDATE editorials SET visibility='private' WHERE id=$1", item.ID)
		Expect(err).NotTo(HaveOccurred())
		done := make(chan error, 1)
		go func() { _, err := editorials.Vote(as(ctx, "reader"), item.ID, users["reader"], true); done <- err }()
		Eventually(func() (int, error) {
			var n int
			err := integrationDB.Pool.GetContext(ctx, &n, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT e.id,e.public_id,e.problem_id%'`)
			return n, err
		}, 3*time.Second).Should(BeNumerically(">", 0))
		Expect(tx.Commit()).To(Succeed())
		Eventually(done, 5*time.Second).Should(Receive(MatchError(ErrNotFound)))
		var votes int
		Expect(integrationDB.Pool.GetContext(ctx, &votes, "SELECT count(*) FROM editorial_votes WHERE editorial_id=$1", item.ID)).To(Succeed())
		Expect(votes).To(BeZero())
	})
})
