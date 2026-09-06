package submission_test

import (
	"context"
	"errors"
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

	It("allows an unpublished problem only through a validated contest", func() {
		contestID := "contest-1"
		problems.problem.Visibility = "draft"
		input := validInput()
		input.ContestID = &contestID

		created, err := service.Submit(ctx, "user-1", "user", input)
		Expect(err).NotTo(HaveOccurred())
		Expect(created.ID).To(Equal("submission-1"))
		Expect(contests.validatedRole).To(Equal("user"))
		Expect(contests.validatedProblem).To(Equal("p1"))
		Expect(problems.reads).To(BeZero())
	})

	It("does not let a contest ID bypass the contest problem boundary", func() {
		contestID := "contest-1"
		contests.err = contestapp.ErrProblemNotInContest
		input := validInput()
		input.ContestID = &contestID

		_, err := service.Submit(ctx, "user-1", "user", input)
		Expect(err).To(MatchError(contestapp.ErrProblemNotInContest))
		Expect(repository.created).To(BeNil())
		Expect(problems.reads).To(BeZero())
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

	It("passes the caller to the repository visibility boundary", func() {
		_, _, err := service.List(ctx, submissionapp.Filters{}, "viewer-1", "user")
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.listViewer).To(Equal(submissionapp.Viewer{UserID: "viewer-1"}))

		repository.item = &submissionapp.Submission{ID: "submission-1", UserID: "owner-1"}
		_, _, err = service.Get(ctx, "submission-1", "admin-1", "admin")
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.getViewer).To(Equal(submissionapp.Viewer{UserID: "admin-1", Admin: true}))
	})

	It("returns not found when the repository hides a detail", func() {
		repository.getErr = submissionapp.ErrNotFound
		item, includeSource, err := service.Get(ctx, "submission-1", "viewer-1", "user")
		Expect(err).To(MatchError(submissionapp.ErrNotFound))
		Expect(item).To(BeNil())
		Expect(includeSource).To(BeFalse())
	})

	It("does not expose contest results when the feedback policy lookup fails", func() {
		contestID := "contest-1"
		policyErr := errors.New("feedback policy unavailable")
		repository.item = &submissionapp.Submission{
			ID: "submission-1", UserID: "owner-1", ContestID: &contestID,
			Status: submissionapp.StatusWrongAnswer,
		}
		contests.feedbackErr = policyErr

		item, includeSource, err := service.Get(ctx, "submission-1", "owner-1", "user")
		Expect(err).To(MatchError(policyErr))
		Expect(item).To(BeNil())
		Expect(includeSource).To(BeFalse())
	})

	It("does not expose contest results when no feedback policy is configured", func() {
		contestID := "contest-1"
		repository.item = &submissionapp.Submission{
			ID: "submission-1", UserID: "owner-1", ContestID: &contestID,
			Status: submissionapp.StatusAccepted,
		}
		service = submissionapp.NewService(repository, problems, validationOnlyContests{}, limiter, nil)

		item, includeSource, err := service.Get(ctx, "submission-1", "owner-1", "user")
		Expect(err).To(MatchError(submissionapp.ErrContestUnavailable))
		Expect(item).To(BeNil())
		Expect(includeSource).To(BeFalse())
	})

	It("fails a contest submission list when viewer resolution is unavailable", func() {
		contestID := "contest-1"
		policyErr := errors.New("contest viewer unavailable")
		repository.items = []submissionapp.Submission{{
			ID: "submission-1", ContestID: &contestID, Status: submissionapp.StatusAccepted,
		}}
		contests.viewerErr = policyErr

		items, total, err := service.List(ctx, submissionapp.Filters{}, "user-1", "user")
		Expect(err).To(MatchError(policyErr))
		Expect(items).To(BeNil())
		Expect(total).To(Equal(0))
	})

	It("returns a visibility-filtered progress view with contest feedback applied", func() {
		contestID := "contest-1"
		repository.progress = &submissionapp.SubmissionProgress{
			ID: "submission-1", UserID: "owner-1", ContestID: &contestID,
			Status: submissionapp.StatusWrongAnswer, Score: 40,
			TotalTimeMs: 12, PeakMemoryKb: 1024, CompileResult: "compiler output",
			JudgedCases: 2, TotalCases: 3,
			CaseResults: []submissionapp.CaseResult{{CaseIndex: 1, Verdict: submissionapp.StatusAccepted}},
		}
		contests.feedback = contestapp.FeedbackNone

		item, err := service.Progress(ctx, "submission-1", "owner-1", "user")
		Expect(err).NotTo(HaveOccurred())
		Expect(item.Status).To(Equal(submissionapp.HiddenStatus))
		Expect(item.Score).To(BeZero())
		Expect(item.CaseResults).To(BeEmpty())
		Expect(item.JudgedCases).To(BeZero())
		Expect(item.TotalCases).To(BeZero())

		_, err = service.Progress(ctx, "submission-1", "other-1", "user")
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.progressViewer).To(Equal(submissionapp.Viewer{UserID: "other-1"}))
	})

	It("returns not found when the repository hides progress", func() {
		repository.progressErr = submissionapp.ErrNotFound
		item, err := service.Progress(ctx, "submission-1", "other-1", "user")
		Expect(err).To(MatchError(submissionapp.ErrNotFound))
		Expect(item).To(BeNil())
	})

	It("fails progress polling when contest feedback cannot be resolved", func() {
		contestID := "contest-1"
		policyErr := errors.New("feedback unavailable")
		repository.progress = &submissionapp.SubmissionProgress{
			ID: "submission-1", UserID: "owner-1", ContestID: &contestID,
			Status: submissionapp.StatusAccepted,
		}
		contests.feedbackErr = policyErr

		item, err := service.Progress(ctx, "submission-1", "owner-1", "user")
		Expect(err).To(MatchError(policyErr))
		Expect(item).To(BeNil())
	})

	It("allows an administrator to poll another user's submission", func() {
		repository.progress = &submissionapp.SubmissionProgress{
			ID: "submission-1", UserID: "owner-1", Status: submissionapp.StatusJudging,
		}

		item, err := service.Progress(ctx, "submission-1", "admin-1", "admin")
		Expect(err).NotTo(HaveOccurred())
		Expect(item.ID).To(Equal("submission-1"))
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
	created        *submissionapp.Submission
	item           *submissionapp.Submission
	items          []submissionapp.Submission
	progress       *submissionapp.SubmissionProgress
	getErr         error
	progressErr    error
	listViewer     submissionapp.Viewer
	getViewer      submissionapp.Viewer
	progressViewer submissionapp.Viewer
	rejudgedID     string
	selector       *submissionapp.RejudgeSelector
}

func (r *fakeRepository) Create(_ context.Context, item *submissionapp.Submission) (*submissionapp.Submission, error) {
	r.created = item
	created := *item
	created.ID = "submission-1"
	return &created, nil
}

func (r *fakeRepository) List(_ context.Context, _ submissionapp.Filters, viewer submissionapp.Viewer) ([]submissionapp.Submission, int, error) {
	r.listViewer = viewer
	return r.items, len(r.items), nil
}

func (r *fakeRepository) Get(_ context.Context, _ string, viewer submissionapp.Viewer) (*submissionapp.Submission, error) {
	r.getViewer = viewer
	return r.item, r.getErr
}

func (r *fakeRepository) Progress(_ context.Context, _ string, viewer submissionapp.Viewer) (*submissionapp.SubmissionProgress, error) {
	r.progressViewer = viewer
	return r.progress, r.progressErr
}

func (r *fakeRepository) CreateRejudging(_ context.Context, selector submissionapp.RejudgeSelector, _ string) (*submissionapp.Rejudging, error) {
	r.selector = &selector
	return &submissionapp.Rejudging{ID: "rejudging-1", State: submissionapp.RejudgingRunning, TotalCount: 3}, nil
}

func (r *fakeRepository) Rejudging(_ context.Context, id string) (*submissionapp.Rejudging, error) {
	return &submissionapp.Rejudging{ID: id}, nil
}

func (r *fakeRepository) ListRejudgings(_ context.Context, _ string, _ int) ([]submissionapp.Rejudging, error) {
	return nil, nil
}

func (r *fakeRepository) CancelRejudging(_ context.Context, _ string) error { return nil }

func (r *fakeRepository) RejudgingChanges(_ context.Context, _ string, _ int) ([]submissionapp.RejudgingChange, error) {
	return nil, nil
}

func (r *fakeRepository) Rejudge(_ context.Context, id string) error {
	r.rejudgedID = id
	return nil
}

type fakeProblems struct {
	problem *problemdomain.Problem
	reads   int
}

func (r *fakeProblems) Get(_ context.Context, _ string) (*problemdomain.Problem, error) {
	r.reads++
	return r.problem, nil
}

type fakeContests struct {
	err              error
	viewerErr        error
	feedbackErr      error
	feedback         string
	validatedRole    string
	validatedProblem string
}

func (r *fakeContests) ValidateSubmission(_ context.Context, _, _, role, problemID string) error {
	r.validatedRole = role
	r.validatedProblem = problemID
	return r.err
}

func (r *fakeContests) Viewer(_ context.Context, _, userID, role string) (contestapp.Viewer, error) {
	return contestapp.Viewer{UserID: userID, Role: role}, r.viewerErr
}

func (r *fakeContests) Feedback(_ context.Context, _ string, _ contestapp.Viewer) (string, error) {
	if r.feedback == "" {
		return contestapp.FeedbackFull, r.feedbackErr
	}
	return r.feedback, r.feedbackErr
}

type validationOnlyContests struct{}

func (validationOnlyContests) ValidateSubmission(context.Context, string, string, string, string) error {
	return nil
}

type fakeLimiter struct {
	allow bool
	key   string
}

func (l *fakeLimiter) Allow(key string) bool {
	l.key = key
	return l.allow
}
