package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
	submissionapp "github.com/RimuruChan/Vertex/server/internal/modules/submission/application"
	submissiondomain "github.com/RimuruChan/Vertex/server/internal/modules/submission/domain"
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
		_, err := service.Submit(ctx, "user-1", "user", submissiondomain.CreateInput{
			ProblemID: "p1", Language: "java", SourceCode: "class Main {}",
		})
		Expect(err).To(MatchError(ContainSubstring("unsupported language")))

		_, err = service.Submit(ctx, "user-1", "user", submissiondomain.CreateInput{
			ProblemID: "p1", Language: "cpp", SourceCode: strings.Repeat("x", submissiondomain.MaxSourceBytes+1),
		})
		Expect(err).To(MatchError(submissiondomain.ErrSourceTooLarge))
		Expect(repository.created).To(BeNil())
	})

	It("enforces the per-user rate limit", func() {
		limiter.allow = false
		_, err := service.Submit(ctx, "user-1", "user", validInput())
		Expect(err).To(MatchError(submissiondomain.ErrRateLimited))
		Expect(limiter.key).To(Equal("user-1"))
	})

	It("rejects non-public problems for regular users", func() {
		problems.problem.Visibility = "private"
		_, err := service.Submit(ctx, "user-1", "user", validInput())
		Expect(err).To(MatchError(submissiondomain.ErrProblemForbidden))
	})

	It("validates contest membership before creating the job", func() {
		contestID := "contest-1"
		contests.err = contestdomain.ErrNotParticipant
		input := validInput()
		input.ContestID = &contestID
		_, err := service.Submit(ctx, "user-1", "user", input)
		Expect(err).To(MatchError(contestdomain.ErrNotParticipant))
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
		contests.err = contestdomain.ErrProblemNotInContest
		input := validInput()
		input.ContestID = &contestID

		_, err := service.Submit(ctx, "user-1", "user", input)
		Expect(err).To(MatchError(contestdomain.ErrProblemNotInContest))
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

	It("requires a resolved source capability rather than an administrator claim", func() {
		repository.item = &submissiondomain.Submission{ID: "submission-1", UserID: "owner-1"}
		_, includeSource, err := service.Get(ctx, "submission-1", "other-1", "user")
		Expect(err).NotTo(HaveOccurred())
		Expect(includeSource).To(BeFalse())
		_, includeSource, err = service.Get(ctx, "submission-1", "other-1", "admin")
		Expect(err).NotTo(HaveOccurred())
		Expect(includeSource).To(BeFalse())
		repository.item.CanReadSource = true
		_, includeSource, err = service.Get(ctx, "submission-1", "other-1", "user")
		Expect(err).NotTo(HaveOccurred())
		Expect(includeSource).To(BeTrue())
	})

	It("passes the caller to the repository visibility boundary", func() {
		_, _, err := service.List(ctx, submissiondomain.Filters{}, "viewer-1", "user")
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.listViewer).To(Equal(submissiondomain.Viewer{UserID: "viewer-1"}))

		repository.item = &submissiondomain.Submission{ID: "submission-1", UserID: "owner-1"}
		_, _, err = service.Get(ctx, "submission-1", "admin-1", "admin")
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.getViewer).To(Equal(submissiondomain.Viewer{UserID: "admin-1"}))
	})

	It("returns not found when the repository hides a detail", func() {
		repository.getErr = submissiondomain.ErrNotFound
		item, includeSource, err := service.Get(ctx, "submission-1", "viewer-1", "user")
		Expect(err).To(MatchError(submissiondomain.ErrNotFound))
		Expect(item).To(BeNil())
		Expect(includeSource).To(BeFalse())
	})

	It("does not expose contest results when the feedback policy lookup fails", func() {
		contestID := "contest-1"
		policyErr := errors.New("feedback policy unavailable")
		repository.item = &submissiondomain.Submission{
			ID: "submission-1", UserID: "owner-1", ContestID: &contestID,
			Status: submissiondomain.StatusWrongAnswer,
		}
		contests.feedbackErr = policyErr

		item, includeSource, err := service.Get(ctx, "submission-1", "owner-1", "user")
		Expect(err).To(MatchError(policyErr))
		Expect(item).To(BeNil())
		Expect(includeSource).To(BeFalse())
	})

	It("does not expose contest results when no feedback policy is configured", func() {
		contestID := "contest-1"
		repository.item = &submissiondomain.Submission{
			ID: "submission-1", UserID: "owner-1", ContestID: &contestID,
			Status: submissiondomain.StatusAccepted,
		}
		service = submissionapp.NewService(repository, problems, validationOnlyContests{}, limiter, nil)

		item, includeSource, err := service.Get(ctx, "submission-1", "owner-1", "user")
		Expect(err).To(MatchError(submissiondomain.ErrContestUnavailable))
		Expect(item).To(BeNil())
		Expect(includeSource).To(BeFalse())
	})

	It("fails a contest submission list when viewer resolution is unavailable", func() {
		contestID := "contest-1"
		policyErr := errors.New("contest viewer unavailable")
		repository.items = []submissiondomain.Submission{{
			ID: "submission-1", ContestID: &contestID, Status: submissiondomain.StatusAccepted,
		}}
		contests.viewerErr = policyErr

		items, total, err := service.List(ctx, submissiondomain.Filters{}, "user-1", "user")
		Expect(err).To(MatchError(policyErr))
		Expect(items).To(BeNil())
		Expect(total).To(Equal(0))
	})

	It("returns a visibility-filtered progress view with contest feedback applied", func() {
		contestID := "contest-1"
		repository.progress = &submissiondomain.SubmissionProgress{
			ID: "submission-1", UserID: "owner-1", ContestID: &contestID,
			Status: submissiondomain.StatusWrongAnswer, Score: 40,
			TotalTimeMs: 12, PeakMemoryKb: 1024, CompileResult: "compiler output",
			JudgedCases: 2, TotalCases: 3,
			CaseResults: []submissiondomain.CaseResult{{CaseIndex: 1, Verdict: submissiondomain.StatusAccepted}},
		}
		contests.feedback = contestdomain.FeedbackNone

		item, err := service.Progress(ctx, "submission-1", "owner-1", "user")
		Expect(err).NotTo(HaveOccurred())
		Expect(item.Status).To(Equal(submissiondomain.HiddenStatus))
		Expect(item.Score).To(BeZero())
		Expect(item.CaseResults).To(BeEmpty())
		Expect(item.JudgedCases).To(BeZero())
		Expect(item.TotalCases).To(BeZero())

		_, err = service.Progress(ctx, "submission-1", "other-1", "user")
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.progressViewer).To(Equal(submissiondomain.Viewer{UserID: "other-1"}))
	})

	It("returns not found when the repository hides progress", func() {
		repository.progressErr = submissiondomain.ErrNotFound
		item, err := service.Progress(ctx, "submission-1", "other-1", "user")
		Expect(err).To(MatchError(submissiondomain.ErrNotFound))
		Expect(item).To(BeNil())
	})

	It("fails progress polling when contest feedback cannot be resolved", func() {
		contestID := "contest-1"
		policyErr := errors.New("feedback unavailable")
		repository.progress = &submissiondomain.SubmissionProgress{
			ID: "submission-1", UserID: "owner-1", ContestID: &contestID,
			Status: submissiondomain.StatusAccepted,
		}
		contests.feedbackErr = policyErr

		item, err := service.Progress(ctx, "submission-1", "owner-1", "user")
		Expect(err).To(MatchError(policyErr))
		Expect(item).To(BeNil())
	})

	It("allows an administrator to poll another user's submission", func() {
		repository.progress = &submissiondomain.SubmissionProgress{
			ID: "submission-1", UserID: "owner-1", Status: submissiondomain.StatusJudging,
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

func validInput() submissiondomain.CreateInput {
	return submissiondomain.CreateInput{ProblemID: "p1", Language: "cpp", SourceCode: "int main() {}"}
}

type fakeRepository struct {
	created        *submissiondomain.Submission
	item           *submissiondomain.Submission
	items          []submissiondomain.Submission
	progress       *submissiondomain.SubmissionProgress
	getErr         error
	progressErr    error
	listViewer     submissiondomain.Viewer
	getViewer      submissiondomain.Viewer
	progressViewer submissiondomain.Viewer
	rejudgedID     string
	selector       *submissiondomain.RejudgeSelector
}

func (r *fakeRepository) Create(_ context.Context, item *submissiondomain.Submission) (*submissiondomain.Submission, error) {
	r.created = item
	created := *item
	created.ID = "submission-1"
	return &created, nil
}

func (r *fakeRepository) List(_ context.Context, _ submissiondomain.Filters, viewer submissiondomain.Viewer) ([]submissiondomain.Submission, int, error) {
	r.listViewer = viewer
	return r.items, len(r.items), nil
}

func (r *fakeRepository) Get(_ context.Context, _ string, viewer submissiondomain.Viewer) (*submissiondomain.Submission, error) {
	r.getViewer = viewer
	return r.item, r.getErr
}

func (r *fakeRepository) Progress(_ context.Context, _ string, viewer submissiondomain.Viewer) (*submissiondomain.SubmissionProgress, error) {
	r.progressViewer = viewer
	return r.progress, r.progressErr
}

func (r *fakeRepository) CreateRejudging(_ context.Context, selector submissiondomain.RejudgeSelector, _ string) (*submissiondomain.Rejudging, error) {
	r.selector = &selector
	return &submissiondomain.Rejudging{ID: "rejudging-1", State: submissiondomain.RejudgingRunning, TotalCount: 3}, nil
}

func (r *fakeRepository) Rejudging(_ context.Context, id string) (*submissiondomain.Rejudging, error) {
	return &submissiondomain.Rejudging{ID: id}, nil
}

func (r *fakeRepository) ListRejudgings(_ context.Context, _ string, _ int) ([]submissiondomain.Rejudging, error) {
	return nil, nil
}

func (r *fakeRepository) CancelRejudging(_ context.Context, _ string) error { return nil }

func (r *fakeRepository) RejudgingChanges(_ context.Context, _ string, _ int) ([]submissiondomain.RejudgingChange, error) {
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

func (r *fakeProblems) Access(_ context.Context, _, _ string) (problemdomain.Access, error) {
	r.reads++
	return problemdomain.Access{Permissions: problemdomain.Permissions{View: r.problem.Visibility == "public"}}, nil
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

func (r *fakeContests) Viewer(_ context.Context, _, userID string) (contestdomain.Viewer, error) {
	return contestdomain.Viewer{UserID: userID}, r.viewerErr
}

func (r *fakeContests) Feedback(_ context.Context, _ string, _ contestdomain.Viewer) (string, error) {
	if r.feedback == "" {
		return contestdomain.FeedbackFull, r.feedbackErr
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

func TestService(t *testing.T) { RegisterFailHandler(Fail); RunSpecs(t, "Submission Service") }

var _ submissionapp.FeedbackReader = (*fakeContests)(nil)
