package contest_test

import (
	"context"
	"errors"
	"time"

	contestapp "github.com/RimuruChan/Vertex/web/internal/contest"
	"github.com/RimuruChan/Vertex/web/internal/identity"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Service", func() {
	var (
		ctx        context.Context
		repository *fakeRepository
		service    *contestapp.Service
		begin      time.Time
		end        time.Time
	)
	tableBegin := time.Unix(1_800_000_000, 0)
	tableEnd := tableBegin.Add(2 * time.Hour)

	BeforeEach(func() {
		ctx = context.Background()
		begin = time.Now().Add(time.Hour)
		end = begin.Add(2 * time.Hour)
		repository = &fakeRepository{contest: &contestapp.Contest{
			ID: "contest-1", Title: "Weekly", Rule: "acm", Visibility: "public",
			BeginAt: begin, EndAt: end, RankboardVisible: true,
		}}
		passwords, err := identity.NewManager("test-secret", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		service = contestapp.NewService(repository, passwords)
	})

	DescribeTable("validates write input",
		func(input contestapp.UpsertInput, message string) {
			_, err := service.Create(ctx, "admin-1", input)
			var validation *contestapp.ValidationError
			Expect(errors.As(err, &validation)).To(BeTrue())
			Expect(validation.Message).To(Equal(message))
		},
		Entry("requires a title", contestapp.UpsertInput{BeginAt: tableBegin, EndAt: tableEnd}, "title required"),
		Entry("supports only ACM", contestapp.UpsertInput{Title: "IOI", Rule: "ioi", BeginAt: tableBegin, EndAt: tableEnd}, "only ACM rule is currently supported"),
		Entry("requires an ordered time window", contestapp.UpsertInput{Title: "Bad", BeginAt: tableEnd, EndAt: tableBegin}, "end time must be after begin time"),
		Entry("requires a password", contestapp.UpsertInput{Title: "Private", Visibility: "password", BeginAt: tableBegin, EndAt: tableEnd}, "password required"),
	)

	It("hashes contest passwords before persistence", func() {
		_, err := service.Create(ctx, "admin-1", contestapp.UpsertInput{
			Title: "Protected", Visibility: "password", Password: "contest-secret",
			BeginAt: begin, EndAt: end,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.persisted.PasswordHash).NotTo(Equal("contest-secret"))
		Expect(identity.CheckPassword(repository.persisted.PasswordHash, "contest-secret")).To(BeTrue())
	})

	It("does not expose protected problems before registration", func() {
		repository.contest.Visibility = "password"
		repository.problems = []contestapp.Problem{{ProblemID: "p1", Visibility: "public"}}
		details, err := service.Details(ctx, repository.contest.ID, "user-1", "user", false)
		Expect(err).NotTo(HaveOccurred())
		Expect(details.Problems).To(BeEmpty())
	})

	It("enforces contest password registration", func() {
		repository.contest.Visibility = "password"
		repository.contest.PasswordHash, _ = identity.HashPassword("contest-secret")
		Expect(service.Register(ctx, repository.contest.ID, "user-1", "user", "wrong")).To(MatchError(contestapp.ErrInvalidPassword))
		Expect(service.Register(ctx, repository.contest.ID, "user-1", "user", "contest-secret")).To(Succeed())
		Expect(repository.registeredUser).To(Equal("user-1"))
	})

	It("rejects submissions from unregistered users", func() {
		repository.contest.BeginAt = time.Now().Add(-time.Hour)
		repository.contest.EndAt = time.Now().Add(time.Hour)
		err := service.ValidateSubmission(ctx, repository.contest.ID, "user-1", "p1")
		Expect(err).To(MatchError(contestapp.ErrNotParticipant))
	})

	It("hides private contests from non-admin users", func() {
		repository.contest.Visibility = "private"
		_, err := service.Details(ctx, repository.contest.ID, "user-1", "user", false)
		Expect(err).To(MatchError(contestapp.ErrNotFound))
	})
})

type fakeRepository struct {
	contest        *contestapp.Contest
	problems       []contestapp.Problem
	participant    bool
	hasProblem     bool
	persisted      *contestapp.PersistInput
	registeredUser string
	board          *contestapp.Rankboard
}

func (r *fakeRepository) Create(_ context.Context, _ string, input *contestapp.PersistInput) (*contestapp.Contest, error) {
	r.persisted = input
	return r.contest, nil
}

func (r *fakeRepository) Update(_ context.Context, _ string, input *contestapp.PersistInput) (*contestapp.Contest, error) {
	r.persisted = input
	return r.contest, nil
}

func (r *fakeRepository) List(_ context.Context, _, _ int) ([]contestapp.Contest, int, error) {
	return []contestapp.Contest{*r.contest}, 1, nil
}

func (r *fakeRepository) ListAdmin(ctx context.Context, limit, offset int) ([]contestapp.Contest, int, error) {
	return r.List(ctx, limit, offset)
}

func (r *fakeRepository) Get(_ context.Context, _ string) (*contestapp.Contest, error) {
	return r.contest, nil
}

func (r *fakeRepository) Problems(_ context.Context, _ string) ([]contestapp.Problem, error) {
	return r.problems, nil
}

func (r *fakeRepository) SetProblems(_ context.Context, _ string, _ []string) error { return nil }

func (r *fakeRepository) IsParticipant(_ context.Context, _, _ string) (bool, error) {
	return r.participant, nil
}

func (r *fakeRepository) Register(_ context.Context, _, userID string) error {
	r.registeredUser = userID
	r.participant = true
	return nil
}

func (r *fakeRepository) HasProblem(_ context.Context, _, _ string) (bool, error) {
	return r.hasProblem, nil
}

func (r *fakeRepository) Rankboard(_ context.Context, _ string, frozen bool) (*contestapp.Rankboard, error) {
	if r.board == nil {
		r.board = &contestapp.Rankboard{Frozen: frozen}
	}
	return r.board, nil
}
