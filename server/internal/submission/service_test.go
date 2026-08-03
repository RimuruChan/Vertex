package submission_test

import (
	"context"
	"strings"
	"time"

	contestapp "github.com/RimuruChan/Vertex/server/internal/contest"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/problem"
	submissionapp "github.com/RimuruChan/Vertex/server/internal/submission"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Service", func() {
	var (
		ctx         context.Context
		repository  *fakeRepository
		problems    *fakeProblems
		contests    *fakeContests
		limiter     *fakeLimiter
		notifiedIDs []string
		service     *submissionapp.Service
	)

	BeforeEach(func() {
		ctx = context.Background()
		repository = &fakeRepository{}
		problems = &fakeProblems{problem: &problemdomain.Problem{ID: "p1", Visibility: "public"}}
		contests = &fakeContests{}
		limiter = &fakeLimiter{allow: true}
		notifiedIDs = nil
		service = submissionapp.NewService(repository, problems, contests, limiter, func(id string) {
			notifiedIDs = append(notifiedIDs, id)
		})
	})

	It("validates language and source limits before persistence", func() {
		_, err := service.Submit(ctx, "user-1", "user", submissionapp.CreateInput{
			ProblemID: "p1", Language: "java", SourceCode: "class Main {}",
		})
		Expect(err).To(MatchError(ContainSubstring("unsupported language")))

		_, err = service.Submit(ctx, "user-1", "user", submissionapp.CreateInput{
			ProblemID: "p1", Language: "cpp", SourceCode: strings.Repeat("x", submissionapp.MaxSourceBytes+1),
		})
		Expect(err).To(MatchError(submissionapp.ErrSourceTooLarge))
		Expect(repository.created).To(BeNil())
	})

	It("enforces the per-user rate limit", func() {
		limiter.allow = false
		_, err := service.Submit(ctx, "user-1", "user", validInput())
		Expect(err).To(MatchError(submissionapp.ErrRateLimited))
		Expect(limiter.key).To(Equal("user-1"))
	})

	It("rejects non-public problems for regular users", func() {
		problems.problem.Visibility = "private"
		_, err := service.Submit(ctx, "user-1", "user", validInput())
		Expect(err).To(MatchError(submissionapp.ErrProblemForbidden))
	})

	It("validates contest membership before creating the job", func() {
		contestID := "contest-1"
		contests.err = contestapp.ErrNotParticipant
		input := validInput()
		input.ContestID = &contestID
		_, err := service.Submit(ctx, "user-1", "user", input)
		Expect(err).To(MatchError(contestapp.ErrNotParticipant))
		Expect(repository.created).To(BeNil())
	})

	It("persists and notifies after a successful submission", func() {
		created, err := service.Submit(ctx, "user-1", "user", validInput())
		Expect(err).NotTo(HaveOccurred())
		Expect(created.ID).To(Equal("submission-1"))
		Expect(repository.created.UserID).To(Equal("user-1"))
		Expect(notifiedIDs).To(Equal([]string{"submission-1"}))
	})

	It("only exposes source to its owner or an administrator", func() {
		repository.item = &submissionapp.Submission{ID: "submission-1", UserID: "owner-1"}
		_, includeSource, err := service.Get(ctx, "submission-1", "other-1", "user")
		Expect(err).NotTo(HaveOccurred())
		Expect(includeSource).To(BeFalse())
		_, includeSource, err = service.Get(ctx, "submission-1", "other-1", "admin")
		Expect(err).NotTo(HaveOccurred())
		Expect(includeSource).To(BeTrue())
	})

	It("notifies only after a rejudge transaction succeeds", func() {
		Expect(service.Rejudge(ctx, "submission-1")).To(Succeed())
		Expect(repository.rejudgedID).To(Equal("submission-1"))
		Expect(notifiedIDs).To(Equal([]string{"submission-1"}))
	})
})

var _ = Describe("SlidingWindowLimiter", func() {
	It("isolates quotas by user", func() {
		limiter := submissionapp.NewSlidingWindowLimiter(time.Minute, 2)
		Expect(limiter.Allow("user-1")).To(BeTrue())
		Expect(limiter.Allow("user-1")).To(BeTrue())
		Expect(limiter.Allow("user-1")).To(BeFalse())
		Expect(limiter.Allow("user-2")).To(BeTrue())
	})
})

func validInput() submissionapp.CreateInput {
	return submissionapp.CreateInput{ProblemID: "p1", Language: "cpp", SourceCode: "int main() {}"}
}

type fakeRepository struct {
	created    *submissionapp.Submission
	item       *submissionapp.Submission
	rejudgedID string
}

func (r *fakeRepository) Create(_ context.Context, item *submissionapp.Submission) (*submissionapp.Submission, error) {
	r.created = item
	created := *item
	created.ID = "submission-1"
	return &created, nil
}

func (r *fakeRepository) List(_ context.Context, _ submissionapp.Filters) ([]submissionapp.Submission, int, error) {
	return nil, 0, nil
}

func (r *fakeRepository) Get(_ context.Context, _ string) (*submissionapp.Submission, error) {
	return r.item, nil
}

func (r *fakeRepository) Rejudge(_ context.Context, id string) error {
	r.rejudgedID = id
	return nil
}

type fakeProblems struct{ problem *problemdomain.Problem }

func (r *fakeProblems) Get(_ context.Context, _ string) (*problemdomain.Problem, error) {
	return r.problem, nil
}

type fakeContests struct{ err error }

func (r *fakeContests) ValidateSubmission(_ context.Context, _, _, _ string) error { return r.err }

type fakeLimiter struct {
	allow bool
	key   string
}

func (l *fakeLimiter) Allow(key string) bool {
	l.key = key
	return l.allow
}
