package postgres_test

import (
	"context"

	contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	submissiondomain "github.com/RimuruChan/Vertex/server/internal/modules/submission/domain"
	submissionstore "github.com/RimuruChan/Vertex/server/internal/modules/submission/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/platform/database/dbtest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Submission visibility against PostgreSQL", func() {
	type fixture struct {
		users       map[string]string
		problems    map[string]string
		submissions map[string]string
		contests    map[string]string
	}

	var store *submissionstore.Repository
	var f fixture

	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		err := dbtest.Reset(ctx, integrationDB, `
			TRUNCATE rejudging_submissions, rejudgings, submission_cases, judge_jobs,
				submissions, contest_participants, contest_access, contest_problems,
				contests, problem_testdata, problems, users
			RESTART IDENTITY CASCADE`)
		Expect(err).NotTo(HaveOccurred())
		store = submissionstore.NewRepository(integrationDB)
		f = fixture{
			users:       map[string]string{},
			problems:    map[string]string{},
			submissions: map[string]string{},
			contests:    map[string]string{},
		}

		for _, name := range []string{"owner", "author", "creator", "participant", "private-participant", "staff", "outsider"} {
			var id string
			Expect(integrationDB.Pool.QueryRowContext(ctx,
				`INSERT INTO users (username, email, password_hash)
				 VALUES ($1, $1 || '@test.local', 'x') RETURNING id`, name).
				Scan(&id)).To(Succeed())
			f.users[name] = id
		}
		Expect(dbtest.OfficialMembers(ctx, integrationDB)).To(Succeed())

		problems := map[string]string{}
		for _, visibility := range []string{"public", "private", "draft"} {
			var id string
			Expect(integrationDB.Pool.QueryRowContext(ctx,
				`INSERT INTO problems (title, visibility, author_id, owner_id)
				 VALUES ($1, $1, $2, $2) RETURNING id`, visibility, f.users["author"]).
				Scan(&id)).To(Succeed())
			problems[visibility] = id
		}
		f.problems = problems
		Expect(dbtest.PublishedProblems(ctx, integrationDB)).To(Succeed())

		for _, visibility := range []string{"public", "password", "private"} {
			var id string
			Expect(integrationDB.Pool.QueryRowContext(ctx,
				`INSERT INTO contests (title, begin_at, end_at, visibility, created_by,owner_id)
				 VALUES ($1, now() - interval '1 hour', now() + interval '1 hour', $1, $2,$2)
				 RETURNING id`, visibility, f.users["creator"]).
				Scan(&id)).To(Succeed())
			f.contests[visibility] = id
		}
		_, err = integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO contest_participants (contest_id, user_id) VALUES ($1, $2), ($3, $4)`,
			f.contests["password"], f.users["participant"],
			f.contests["private"], f.users["private-participant"])
		Expect(err).NotTo(HaveOccurred())
		_, err = integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO contest_access (contest_id, user_id, role) VALUES ($1, $2, 'observer')`,
			f.contests["private"], f.users["staff"])
		Expect(err).NotTo(HaveOccurred())

		insertSubmission := func(name, problemID string, contestID *string) {
			var id string
			Expect(integrationDB.Pool.QueryRowContext(ctx,
				`INSERT INTO submissions (user_id, problem_id, language, source_code, status, contest_id,problem_version)
				 VALUES ($1, $2, 'cpp', 'secret', 'Accepted', $3,1) RETURNING id`,
				f.users["owner"], problemID, contestID).Scan(&id)).To(Succeed())
			f.submissions[name] = id
		}
		insertSubmission("practice-public", problems["public"], nil)
		insertSubmission("practice-private", problems["private"], nil)
		insertSubmission("practice-draft", problems["draft"], nil)
		publicContest := f.contests["public"]
		passwordContest := f.contests["password"]
		privateContest := f.contests["private"]
		insertSubmission("contest-public", problems["private"], &publicContest)
		insertSubmission("contest-password", problems["private"], &passwordContest)
		insertSubmission("contest-private", problems["private"], &privateContest)
	})

	visibleNames := func(ctx context.Context, viewer submissiondomain.Viewer) ([]string, int) {
		items, total, err := store.List(ctx, submissiondomain.Filters{Limit: 100}, viewer)
		Expect(err).NotTo(HaveOccurred())
		namesByID := make(map[string]string, len(f.submissions))
		for name, id := range f.submissions {
			namesByID[id] = name
		}
		names := make([]string, 0, len(items))
		for _, item := range items {
			names = append(names, namesByID[item.ID])
		}
		return names, total
	}

	It("filters list rows and totals before pagination", func(ctx SpecContext) {
		viewer := submissiondomain.Viewer{UserID: f.users["outsider"]}
		names, total := visibleNames(ctx, viewer)
		Expect(names).To(ConsistOf("practice-public"))
		Expect(total).To(Equal(1))

		items, total, err := store.List(ctx, submissiondomain.Filters{Limit: 1}, viewer)
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))
		Expect(total).To(Equal(1))

		items, total, err = store.List(ctx, submissiondomain.Filters{
			ContestID: f.contests["private"], Limit: 20,
		}, viewer)
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(BeEmpty())
		Expect(total).To(BeZero())
	})

	It("grants the established owner, author and contest-scoped readers", func(ctx SpecContext) {
		cases := []struct {
			viewer submissiondomain.Viewer
			want   []string
		}{
			{submissiondomain.Viewer{UserID: f.users["owner"]}, []string{
				"practice-public", "practice-private", "practice-draft", "contest-public", "contest-password", "contest-private",
			}},
			{submissiondomain.Viewer{UserID: f.users["author"]}, []string{
				"practice-public", "practice-private", "practice-draft",
			}},
			{submissiondomain.Viewer{UserID: f.users["participant"]}, []string{
				"practice-public",
			}},
			{submissiondomain.Viewer{UserID: f.users["private-participant"]}, []string{
				"practice-public",
			}},
			{submissiondomain.Viewer{UserID: f.users["staff"]}, []string{
				"practice-public", "contest-private",
			}},
			{submissiondomain.Viewer{UserID: f.users["creator"]}, []string{
				"practice-public", "contest-public", "contest-password", "contest-private",
			}},
			{submissiondomain.Viewer{UserID: f.users["outsider"]}, []string{
				"practice-public",
			}},
		}
		for _, tc := range cases {
			names, total := visibleNames(ctx, tc.viewer)
			Expect(names).To(ConsistOf(tc.want))
			Expect(total).To(Equal(len(tc.want)))
		}
		_, err := integrationDB.Pool.ExecContext(ctx, "UPDATE users SET role='admin' WHERE id=$1", f.users["outsider"])
		Expect(err).NotTo(HaveOccurred())
		names, total := visibleNames(ctx, submissiondomain.Viewer{UserID: f.users["outsider"]})
		Expect(total).To(Equal(6))
		Expect(names).To(HaveLen(6))
	})

	It("uses the same not-found boundary for detail and progress", func(ctx SpecContext) {
		outsider := submissiondomain.Viewer{UserID: f.users["outsider"]}
		_, err := store.Get(ctx, f.submissions["practice-private"], outsider)
		Expect(err).To(MatchError(submissiondomain.ErrNotFound))
		_, err = store.Progress(ctx, f.submissions["practice-private"], outsider)
		Expect(err).To(MatchError(submissiondomain.ErrNotFound))

		_, err = store.Get(ctx, f.submissions["contest-public"], outsider)
		Expect(err).To(MatchError(submissiondomain.ErrNotFound))
		_, err = store.Progress(ctx, f.submissions["contest-public"], outsider)
		Expect(err).To(MatchError(submissiondomain.ErrNotFound))

		privateParticipant := submissiondomain.Viewer{UserID: f.users["private-participant"]}
		_, err = store.Get(ctx, f.submissions["contest-private"], privateParticipant)
		Expect(err).To(MatchError(submissiondomain.ErrNotFound))
		_, err = store.Get(ctx, f.submissions["contest-private"], submissiondomain.Viewer{UserID: f.users["staff"]})
		Expect(err).NotTo(HaveOccurred())
	})

	It("rechecks contest membership and problem scope in the create transaction", func(ctx SpecContext) {
		passwordContest := f.contests["password"]
		input := &submissiondomain.Submission{
			UserID: f.users["participant"], ProblemID: f.problems["private"],
			Language: "cpp", SourceCode: "int main() {}", ContestID: &passwordContest,
		}

		_, err := store.Create(ctx, input)
		Expect(err).To(MatchError(contestdomain.ErrProblemNotInContest))

		_, err = integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO contest_problems (contest_id, problem_id, label)
			 VALUES ($1, $2, 'A')`, passwordContest, f.problems["private"])
		Expect(err).NotTo(HaveOccurred())
		created, err := store.Create(ctx, input)
		Expect(err).NotTo(HaveOccurred())
		Expect(created.ContestID).NotTo(BeNil())
		Expect(*created.ContestID).To(Equal(passwordContest))

		privateContest := f.contests["private"]
		_, err = integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO contest_problems (contest_id, problem_id, label)
			 VALUES ($1, $2, 'A')`, privateContest, f.problems["private"])
		Expect(err).NotTo(HaveOccurred())
		input.UserID = f.users["private-participant"]
		input.ContestID = &privateContest
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE contests SET admission='restricted' WHERE id=$1", privateContest)
		Expect(err).NotTo(HaveOccurred())
		_, err = store.Create(ctx, input)
		Expect(err).To(MatchError(submissiondomain.ErrNotFound))

		input.UserID = f.users["staff"]
		_, err = store.Create(ctx, input)
		Expect(err).To(MatchError(contestdomain.ErrNotParticipant))
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE contest_access SET role='jury' WHERE contest_id=$1 AND user_id=$2", privateContest, input.UserID)
		Expect(err).NotTo(HaveOccurred())
		created, err = store.Create(ctx, input)
		Expect(err).NotTo(HaveOccurred())
		Expect(created.UserID).To(Equal(f.users["staff"]))
	})

	It("respects configured post-contest sharing and hidden frozen records", func(ctx SpecContext) {
		publicContest := f.contests["public"]
		_, err := integrationDB.Pool.ExecContext(ctx,
			`UPDATE contests SET submission_visibility='after_end', frozen_submission_visibility='hidden', end_at = now() - interval '1 minute' WHERE id = $1`, publicContest)
		Expect(err).NotTo(HaveOccurred())

		// The contest problem is private, so an outsider still cannot learn its
		// title merely because the public contest has ended.
		outsider := submissiondomain.Viewer{UserID: f.users["outsider"]}
		_, err = store.Get(ctx, f.submissions["contest-public"], outsider)
		Expect(err).To(MatchError(submissiondomain.ErrNotFound))

		_, err = integrationDB.Pool.ExecContext(ctx,
			`INSERT INTO contest_participants (contest_id, user_id) VALUES ($1, $2)`,
			publicContest, f.users["participant"])
		Expect(err).NotTo(HaveOccurred())
		participant := submissiondomain.Viewer{UserID: f.users["participant"]}
		_, err = store.Get(ctx, f.submissions["contest-public"], participant)
		Expect(err).NotTo(HaveOccurred())

		// A freeze remains an information boundary after the scheduled end until
		// the jury explicitly reveals it.
		_, err = integrationDB.Pool.ExecContext(ctx,
			`UPDATE contests
			 SET freeze_at = now() - interval '30 minutes', unfreeze_at = NULL
			 WHERE id = $1`, publicContest)
		Expect(err).NotTo(HaveOccurred())
		_, err = store.Get(ctx, f.submissions["contest-public"], participant)
		Expect(err).To(MatchError(submissiondomain.ErrNotFound))

		_, err = integrationDB.Pool.ExecContext(ctx,
			`UPDATE contests SET unfreeze_at = now() - interval '1 second' WHERE id = $1`, publicContest)
		Expect(err).NotTo(HaveOccurred())
		_, err = store.Get(ctx, f.submissions["contest-public"], participant)
		Expect(err).NotTo(HaveOccurred())
	})
})
