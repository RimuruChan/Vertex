package application_test

import (
	"context"
	"errors"
	contestapp "github.com/RimuruChan/Vertex/server/internal/modules/contest/application"
	contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	identitytoken "github.com/RimuruChan/Vertex/server/internal/modules/identity/infrastructure/token"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
	"time"
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
		repository = &fakeRepository{contest: &contestdomain.Contest{
			ID: "contest-1", Title: "Weekly", Rule: "acm", Visibility: "public",
			BeginAt: begin, EndAt: end, RankboardVisible: true,
			AllowSelfRegistration: true,
		}}
		passwords, err := identitytoken.NewManager("test-secret", time.Minute)
		Expect(err).NotTo(HaveOccurred())
		service = contestapp.NewService(repository, passwords)
	})

	DescribeTable("validates write input",
		func(input contestdomain.UpsertInput, message string) {
			_, err := service.Create(ctx, "admin-1", input)
			var validation *contestdomain.ValidationError
			Expect(errors.As(err, &validation)).To(BeTrue())
			Expect(validation.Message).To(Equal(message))
		},
		Entry("requires a title", contestdomain.UpsertInput{BeginAt: tableBegin, EndAt: tableEnd}, "title required"),
		Entry("rejects an unknown rule", contestdomain.UpsertInput{Title: "Weird", Rule: "swiss", BeginAt: tableBegin, EndAt: tableEnd}, "rule must be icpc, ioi or oi"),
		Entry("rejects an unknown feedback level", contestdomain.UpsertInput{Title: "Loud", Feedback: "verbose", BeginAt: tableBegin, EndAt: tableEnd}, "feedback must be full, summary or none"),
		Entry("requires a freeze time before an unfreeze time", contestdomain.UpsertInput{Title: "Thaw", BeginAt: tableBegin, EndAt: tableEnd, UnfreezeAt: &tableEnd}, "unfreeze time requires a freeze time"),
		Entry("requires an ordered time window", contestdomain.UpsertInput{Title: "Bad", BeginAt: tableEnd, EndAt: tableBegin}, "end time must be after begin time"),
		Entry("requires a password", contestdomain.UpsertInput{Title: "Private", Visibility: "password", BeginAt: tableBegin, EndAt: tableEnd}, "password required"),
	)

	It("hashes contest passwords before persistence", func() {
		_, err := service.Create(ctx, "admin-1", contestdomain.UpsertInput{
			Title: "Protected", Visibility: "password", Password: "contest-secret",
			BeginAt: begin, EndAt: end,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.persisted.PasswordHash).NotTo(Equal("contest-secret"))
		Expect(identitytoken.CheckPassword(repository.persisted.PasswordHash, "contest-secret")).To(BeTrue())
	})

	It("does not expose protected problems before registration", func() {
		repository.contest.Visibility = "password"
		repository.problems = []contestdomain.Problem{{ProblemID: "p1", Visibility: "public"}}
		details, err := service.Details(ctx, repository.contest.ID, "user-1", "user")
		Expect(err).NotTo(HaveOccurred())
		Expect(details.Problems).To(BeEmpty())
	})

	It("reveals an unpublished public-contest problem only to a participant after the start", func() {
		repository.contest.BeginAt = time.Now().Add(-time.Hour)
		repository.contest.EndAt = time.Now().Add(time.Hour)
		repository.problems = []contestdomain.Problem{{ProblemID: "p1", Visibility: "draft"}}

		details, err := service.Details(ctx, repository.contest.ID, "outsider-1", "user")
		Expect(err).NotTo(HaveOccurred())
		Expect(details.Problems).To(BeEmpty())

		repository.participant = true
		details, err = service.Details(ctx, repository.contest.ID, "participant-1", "user")
		Expect(err).NotTo(HaveOccurred())
		Expect(details.Problems).To(HaveLen(1))
	})

	It("enforces contest password registration", func() {
		repository.contest.Visibility = "password"
		repository.contest.PasswordHash, _ = identitytoken.HashPassword("contest-secret")
		Expect(service.Register(ctx, repository.contest.ID, "user-1", "user", "wrong")).To(MatchError(contestdomain.ErrInvalidPassword))
		Expect(service.Register(ctx, repository.contest.ID, "user-1", "user", "contest-secret")).To(Succeed())
		Expect(repository.registeredUser).To(Equal("user-1"))
	})

	It("rejects submissions from unregistered users", func() {
		repository.contest.BeginAt = time.Now().Add(-time.Hour)
		repository.contest.EndAt = time.Now().Add(time.Hour)
		err := service.ValidateSubmission(ctx, repository.contest.ID, "user-1", "user", "p1")
		Expect(err).To(MatchError(contestdomain.ErrNotParticipant))
	})

	It("accepts an unpublished contest problem for a registered participant", func() {
		repository.contest.BeginAt = time.Now().Add(-time.Hour)
		repository.contest.EndAt = time.Now().Add(time.Hour)
		repository.participant = true
		repository.hasProblem = true

		Expect(service.ValidateSubmission(ctx, repository.contest.ID,
			"user-1", "user", "private-problem")).To(Succeed())
	})

	It("lets contest staff submit during the round without registering as a participant", func() {
		repository.contest.BeginAt = time.Now().Add(-time.Hour)
		repository.contest.EndAt = time.Now().Add(time.Hour)
		repository.staffRole = contestdomain.StaffJury
		repository.hasProblem = true

		Expect(service.ValidateSubmission(ctx, repository.contest.ID,
			"jury-1", "user", "private-problem")).To(Succeed())
	})

	DescribeTable("enforces the contest window before accepting a submission",
		func(begin, end time.Time) {
			repository.contest.BeginAt = begin
			repository.contest.EndAt = end
			repository.participant = true
			repository.hasProblem = true
			Expect(service.ValidateSubmission(ctx, repository.contest.ID,
				"user-1", "user", "p1")).To(MatchError(contestdomain.ErrNotActive))
		},
		Entry("before the start", time.Now().Add(time.Hour), time.Now().Add(2*time.Hour)),
		Entry("after the end", time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)),
	)

	It("rejects a problem that is not part of the contest", func() {
		repository.contest.BeginAt = time.Now().Add(-time.Hour)
		repository.contest.EndAt = time.Now().Add(time.Hour)
		repository.participant = true
		repository.hasProblem = false

		Expect(service.ValidateSubmission(ctx, repository.contest.ID,
			"user-1", "user", "outside-problem")).To(MatchError(contestdomain.ErrProblemNotInContest))
	})

	It("hides private contests from non-admin users", func() {
		repository.contest.Visibility = "private"
		_, err := service.Details(ctx, repository.contest.ID, "user-1", "user")
		Expect(err).To(MatchError(contestdomain.ErrNotFound))
	})

	It("shows a private contest to its own jury", func() {
		repository.contest.Visibility = "private"
		repository.staffRole = contestdomain.StaffJury
		details, err := service.Details(ctx, repository.contest.ID, "jury-1", "user")
		Expect(err).NotTo(HaveOccurred())
		Expect(details.Staff).To(Equal(contestdomain.StaffJury))
	})

	It("defaults an OI contest to silent feedback", func() {
		_, err := service.Create(ctx, "admin-1", contestdomain.UpsertInput{
			Title: "OI Round", Rule: "oi", BeginAt: begin, EndAt: end,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.persisted.Feedback).To(Equal(contestdomain.FeedbackNone))
	})

	It("keeps the legacy acm rule working as icpc", func() {
		_, err := service.Create(ctx, "admin-1", contestdomain.UpsertInput{
			Title: "Legacy", Rule: "acm", BeginAt: begin, EndAt: end,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.persisted.Rule).To(Equal(contestdomain.FormatICPC))
		Expect(repository.persisted.PenaltyMinutes).To(Equal(20))
	})

	It("labels contest problems A, B, C by position", func() {
		Expect(service.SetProblems(ctx, "contest-1", []contestdomain.ProblemEntry{
			{ProblemID: "p1"}, {ProblemID: "p2"}, {ProblemID: "p3", Label: "X"},
		})).To(Succeed())
		Expect(repository.entries[0].Label).To(Equal("A"))
		Expect(repository.entries[1].Label).To(Equal("B"))
		Expect(repository.entries[2].Label).To(Equal("X"))
		Expect(repository.entries[0].Points).To(Equal(100))
	})

	It("rejects the same problem twice in one contest", func() {
		err := service.SetProblems(ctx, "contest-1", []contestdomain.ProblemEntry{
			{ProblemID: "p1"}, {ProblemID: "p1"},
		})
		Expect(err).To(MatchError(contestdomain.ErrInvalidInput))
	})
	It("rejects ambiguous and duplicate public problem labels", func() {
		for _, label := range []string{"1000", "A/B", "A?tab=X", "TOO_LONG_NAME"} {
			Expect(service.SetProblems(ctx, "contest-1", []contestdomain.ProblemEntry{{ProblemID: "p1", Label: label}})).To(MatchError(contestdomain.ErrInvalidInput))
		}
		Expect(service.SetProblems(ctx, "contest-1", []contestdomain.ProblemEntry{{ProblemID: "p1", Label: "A"}, {ProblemID: "p2", Label: "A"}})).To(MatchError(contestdomain.ErrInvalidInput))
	})

	It("gives contestants the frozen board and staff the jury board", func() {
		freeze := time.Now().Add(-time.Minute)
		repository.contest.BeginAt = time.Now().Add(-time.Hour)
		repository.contest.EndAt = time.Now().Add(time.Hour)
		repository.contest.FreezeAt = &freeze

		board, err := service.Rankboard(ctx, "contest-1", "user-1", "user", true)
		Expect(err).NotTo(HaveOccurred())
		Expect(board.JuryView).To(BeFalse())
		Expect(board.Frozen).To(BeTrue())

		repository.board = nil
		repository.staffRole = contestdomain.StaffObserver
		board, err = service.Rankboard(ctx, "contest-1", "observer-1", "user", true)
		Expect(err).NotTo(HaveOccurred())
		Expect(board.JuryView).To(BeTrue())
		Expect(board.Frozen).To(BeFalse())
	})

	It("reports the contest feedback level for contestants and staff", func() {
		repository.contest.BeginAt = time.Now().Add(-time.Hour)
		repository.contest.EndAt = time.Now().Add(time.Hour)
		repository.contest.Feedback = contestdomain.FeedbackNone

		level, err := service.Feedback(ctx, "contest-1", contestdomain.Viewer{UserID: "user-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(level).To(Equal(contestdomain.FeedbackNone))

		level, err = service.Feedback(ctx, "contest-1", contestdomain.Viewer{UserID: "j", Staff: contestdomain.StaffJury})
		Expect(err).NotTo(HaveOccurred())
		Expect(level).To(Equal(contestdomain.FeedbackFull))
	})

	It("refuses jury actions from ordinary contestants", func() {
		_, err := service.RequireJury(ctx, "contest-1", "user-1", "user")
		Expect(err).To(MatchError(contestdomain.ErrForbidden))

		repository.staffRole = contestdomain.StaffObserver
		_, err = service.RequireJury(ctx, "contest-1", "observer-1", "user")
		Expect(err).To(MatchError(contestdomain.ErrForbidden))

		repository.staffRole = contestdomain.StaffJury
		_, err = service.RequireJury(ctx, "contest-1", "jury-1", "user")
		Expect(err).NotTo(HaveOccurred())
	})
})

type fakeRepository struct {
	admin          bool
	accessGrants   contestdomain.Grants
	contest        *contestdomain.Contest
	problems       []contestdomain.Problem
	participant    bool
	hasProblem     bool
	persisted      *contestdomain.PersistInput
	registeredUser string
	board          *contestdomain.Rankboard
	entries        []contestdomain.ProblemEntry
	staffRole      string
	staff          []contestdomain.Staff
	problemDetail  *contestdomain.ProblemDetail
	problemErr     error
	problemReads   int
}

func (f *fakeRepository) UseProblemVersion(context.Context, string, string, int, int) error {
	return nil
}

func (*fakeRepository) Grants(context.Context, string) ([]contestdomain.AccessGrant, error) {
	return nil, nil
}
func (*fakeRepository) SetGrant(context.Context, string, contestdomain.GrantInput) error { return nil }
func (*fakeRepository) RemoveGrant(context.Context, string, int64) error                 { return nil }
func (*fakeRepository) Transfer(context.Context, string, string) error                   { return nil }
func (*fakeRepository) Delete(context.Context, string) error                             { return nil }

func (r *fakeRepository) Access(_ context.Context, id, userID string) (contestdomain.Access, error) {
	scope := tenancydomain.Scope{Domain: tenancydomain.Domain{Visibility: "public"}, UserID: userID, SiteAdmin: r.admin, MemberStatus: "active", RolePermissions: []tenancydomain.Permission{tenancydomain.CreateSubmission}}
	owner := r.contest.OwnerID
	if owner == "" && r.contest.CreatedBy != nil {
		owner = *r.contest.CreatedBy
	}
	admission := r.contest.Admission
	if admission == "" {
		admission = contestdomain.AdmissionMembers
	}
	grants := r.accessGrants
	grants.Jury = grants.Jury || r.staffRole == contestdomain.StaffJury
	grants.Observer = grants.Observer || r.staffRole == contestdomain.StaffObserver
	value := contestdomain.Access{Scope: scope, ContestID: id, OwnerID: owner, Visibility: r.contest.Visibility, Admission: admission, Grants: grants, Registered: r.participant}
	value.Permissions = contestdomain.EffectivePermissions(scope, owner, value.Visibility, admission, grants, r.participant)
	return value, nil
}

func (r *fakeRepository) Create(_ context.Context, _ string, input *contestdomain.PersistInput) (*contestdomain.Contest, error) {
	r.persisted = input
	return r.contest, nil
}

func (r *fakeRepository) Update(_ context.Context, _ string, input *contestdomain.PersistInput) (*contestdomain.Contest, error) {
	r.persisted = input
	return r.contest, nil
}

func (r *fakeRepository) List(_ context.Context, _, _ int, _ ...string) ([]contestdomain.Contest, int, error) {
	return []contestdomain.Contest{*r.contest}, 1, nil
}

func (r *fakeRepository) ListAdmin(ctx context.Context, limit, offset int, keyword ...string) ([]contestdomain.Contest, int, error) {
	return r.List(ctx, limit, offset)
}

func (r *fakeRepository) Get(_ context.Context, _ string) (*contestdomain.Contest, error) {
	return r.contest, nil
}

func (r *fakeRepository) Problems(_ context.Context, _ string) ([]contestdomain.Problem, error) {
	return r.problems, nil
}

func (r *fakeRepository) Problem(_ context.Context, contestID, problemID string) (*contestdomain.ProblemDetail, error) {
	r.problemReads++
	if r.problemErr != nil {
		return nil, r.problemErr
	}
	if r.problemDetail != nil {
		return r.problemDetail, nil
	}
	return &contestdomain.ProblemDetail{Problem: contestdomain.Problem{
		ContestID: contestID, ProblemID: problemID, Title: "Contest problem",
	}}, nil
}

func (r *fakeRepository) SetProblems(_ context.Context, _ string, entries []contestdomain.ProblemEntry) error {
	r.entries = entries
	return nil
}

func (r *fakeRepository) StaffRole(_ context.Context, _, _ string) (string, error) {
	return r.staffRole, nil
}

func (r *fakeRepository) ListStaff(_ context.Context, _ string) ([]contestdomain.Staff, error) {
	return r.staff, nil
}

func (r *fakeRepository) AddStaff(_ context.Context, contestID, username, role string) (*contestdomain.Staff, error) {
	added := contestdomain.Staff{ContestID: contestID, Username: username, Role: role}
	r.staff = append(r.staff, added)
	return &added, nil
}

func (r *fakeRepository) RemoveStaff(_ context.Context, _, _ string) error { return nil }

func (r *fakeRepository) IsParticipant(_ context.Context, _, _ string) (bool, error) {
	return r.participant, nil
}

func (r *fakeRepository) Register(_ context.Context, _, userID string, _ ...string) error {
	r.registeredUser = userID
	r.participant = true
	return nil
}

func (r *fakeRepository) HasProblem(_ context.Context, _, _ string) (bool, error) {
	return r.hasProblem, nil
}

func (r *fakeRepository) Rankboard(_ context.Context, _ string, jury bool) (*contestdomain.Rankboard, error) {
	if r.board == nil {
		r.board = &contestdomain.Rankboard{JuryView: jury}
	}
	return r.board, nil
}

type problemAccessCase struct {
	visibility  string
	participant bool
	staff       string
	creator     bool
	role        string
	beforeStart bool
	ended       bool
	wantErr     error
}

var _ = Describe("Contest problem access", func() {
	DescribeTable("keeps unpublished statements contest-scoped",
		func(test problemAccessCase) {
			const userID = "user-1"
			createdBy := "admin-1"
			if test.creator {
				createdBy = userID
			}
			begin := time.Now().Add(-time.Hour)
			end := time.Now().Add(time.Hour)
			if test.beforeStart {
				begin = time.Now().Add(time.Hour)
				end = begin.Add(time.Hour)
			}
			if test.ended {
				begin = time.Now().Add(-2 * time.Hour)
				end = time.Now().Add(-time.Hour)
			}
			repository := &fakeRepository{
				admin: test.role == "admin",
				contest: &contestdomain.Contest{
					ID: "contest-1", Visibility: test.visibility, CreatedBy: &createdBy,
					BeginAt: begin, EndAt: end,
				},
				participant: test.participant, staffRole: test.staff,
				problemDetail: &contestdomain.ProblemDetail{
					Problem: contestdomain.Problem{
						ContestID: "contest-1", ProblemID: "problem-1",
						Title: "Hidden", Visibility: "draft",
					},
					StatementMD: "secret statement",
				},
			}
			service := contestapp.NewService(repository, nil)

			item, err := service.Problem(context.Background(),
				"contest-1", "problem-1", userID, test.role)
			if test.wantErr != nil {
				Expect(err).To(MatchError(test.wantErr))
				Expect(item).To(BeNil())
				Expect(repository.problemReads).To(BeZero())
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(item.StatementMD).To(Equal("secret statement"))
			Expect(repository.problemReads).To(Equal(1))
		},
		Entry("allows a registered public-contest participant", problemAccessCase{
			visibility: "public", participant: true,
		}),
		Entry("rejects a public-contest outsider", problemAccessCase{
			visibility: "public", wantErr: contestdomain.ErrRegistrationNeeded,
		}),
		Entry("allows a registered password-contest participant", problemAccessCase{
			visibility: "password", participant: true,
		}),
		Entry("rejects a password-contest outsider", problemAccessCase{
			visibility: "password", wantErr: contestdomain.ErrRegistrationNeeded,
		}),
		Entry("hides a private contest from an outsider", problemAccessCase{
			visibility: "private", wantErr: contestdomain.ErrNotFound,
		}),
		Entry("allows private-contest jury", problemAccessCase{
			visibility: "private", staff: contestdomain.StaffJury,
		}),
		Entry("allows private-contest observers", problemAccessCase{
			visibility: "private", staff: contestdomain.StaffObserver,
		}),
		Entry("allows the private-contest creator", problemAccessCase{
			visibility: "private", creator: true,
		}),
		Entry("allows a global administrator", problemAccessCase{
			visibility: "private", role: "admin",
		}),
		Entry("hides a pre-start problem from a participant", problemAccessCase{
			visibility: "public", participant: true, beforeStart: true,
			wantErr: contestdomain.ErrNotFound,
		}),
		Entry("keeps the statement available to a participant after the contest", problemAccessCase{
			visibility: "public", participant: true, ended: true,
		}),
	)

	It("hides a problem that is not linked to the contest", func() {
		repository := &fakeRepository{
			contest: &contestdomain.Contest{
				ID: "contest-1", Visibility: "public",
				BeginAt: time.Now().Add(-time.Hour), EndAt: time.Now().Add(time.Hour),
			},
			participant: true, problemErr: contestdomain.ErrProblemNotInContest,
		}
		service := contestapp.NewService(repository, nil)
		_, err := service.Problem(context.Background(),
			"contest-1", "other-problem", "user-1", "user")
		Expect(err).To(MatchError(contestdomain.ErrNotFound))
	})
})

func TestService(t *testing.T) { RegisterFailHandler(Fail); RunSpecs(t, "Contest application") }
