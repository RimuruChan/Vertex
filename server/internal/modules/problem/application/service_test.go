package application_test

import (
	"context"
	"errors"
	"testing"

	problemapp "github.com/RimuruChan/Vertex/server/internal/modules/problem/application"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Service", func() {
	It("enforces public visibility for non-admin lists", func() {
		repository := &fakeProblemRepository{}
		service := problemapp.NewService(repository, repository)
		_, _, err := service.List(context.Background(), problemdomain.Filters{Visibility: "private"}, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.filters.Visibility).To(Equal("public"))
	})

	It("hides unpublished problems from public callers", func() {
		repository := &fakeProblemRepository{problem: &problemdomain.Problem{ID: "problem-1", Visibility: "draft"}}
		service := problemapp.NewService(repository, repository)
		_, err := service.Get(context.Background(), "problem-1", "")
		Expect(err).To(MatchError(problemdomain.ErrNotFound))
	})

	It("annotates viewer progress on listed problems", func() {
		repository := &fakeProblemRepository{
			list: []problemdomain.Problem{{ID: "solved-1"}, {ID: "tried-1"}, {ID: "fresh-1"}},
			statuses: map[string]string{
				"solved-1": problemdomain.UserStatusSolved,
				"tried-1":  problemdomain.UserStatusAttempted,
			},
		}
		service := problemapp.NewService(repository, repository)
		list, _, err := service.List(context.Background(), problemdomain.Filters{ViewerID: "user-1"}, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.statusViewer).To(Equal("user-1"))
		Expect(repository.statusIDs).To(Equal([]string{"solved-1", "tried-1", "fresh-1"}))
		Expect(list[0].UserStatus).To(Equal(problemdomain.UserStatusSolved))
		Expect(list[1].UserStatus).To(Equal(problemdomain.UserStatusAttempted))
		Expect(list[2].UserStatus).To(Equal(problemdomain.UserStatusNone))
	})

	It("reports every problem as unattempted for anonymous viewers", func() {
		repository := &fakeProblemRepository{
			list:     []problemdomain.Problem{{ID: "solved-1"}},
			statuses: map[string]string{"solved-1": problemdomain.UserStatusSolved},
		}
		service := problemapp.NewService(repository, repository)
		list, _, err := service.List(context.Background(), problemdomain.Filters{}, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.statusViewer).To(BeEmpty())
		Expect(list[0].UserStatus).To(Equal(problemdomain.UserStatusNone))
	})

	It("drops progress filters that are not part of the contract", func() {
		repository := &fakeProblemRepository{}
		service := problemapp.NewService(repository, repository)
		_, _, err := service.List(context.Background(), problemdomain.Filters{ViewerID: "user-1", Status: "starred"}, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.filters.Status).To(BeEmpty())
	})

	It("normalizes defaults and tags before persistence", func() {
		repository := &fakeProblemRepository{}
		service := problemapp.NewService(repository, repository)
		_, err := service.Create(context.Background(), "author-1", problemdomain.CreateInput{
			Title: "  A + B  ", Tags: []string{" math ", "", "math"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.created.Title).To(Equal("A + B"))
		Expect(repository.created.Difficulty).To(Equal(1))
		Expect(repository.created.TimeLimitMs).To(Equal(1000))
		Expect(repository.created.MemoryLimitKb).To(Equal(262144))
		Expect(repository.created.Visibility).To(Equal("draft"))
		Expect(repository.created.Tags).To(Equal([]string{"math"}))
	})

	DescribeTable("rejects invalid problem input",
		func(input problemdomain.CreateInput) {
			repository := &fakeProblemRepository{}
			service := problemapp.NewService(repository, repository)
			_, err := service.Create(context.Background(), "author-1", input)
			Expect(errors.Is(err, problemdomain.ErrInvalidInput)).To(BeTrue())
			Expect(repository.created).To(BeNil())
		},
		Entry("missing title", problemdomain.CreateInput{}),
		Entry("difficulty", problemdomain.CreateInput{Title: "x", Difficulty: 11}),
		Entry("time limit", problemdomain.CreateInput{Title: "x", TimeLimitMs: -1}),
		Entry("memory limit", problemdomain.CreateInput{Title: "x", MemoryLimitKb: -1}),
		Entry("visibility", problemdomain.CreateInput{Title: "x", Visibility: "hidden"}),
	)

	It("rejects unsupported checkers before reading persistence", func() {
		repository := &fakeProblemRepository{}
		service := problemapp.NewService(repository, repository)
		_, _, err := service.SaveTestdata(context.Background(), "problem-1", []byte("zip"), "shell")
		Expect(errors.Is(err, problemdomain.ErrInvalidInput)).To(BeTrue())
		Expect(repository.saved).To(BeFalse())
	})
})

type fakeProblemRepository struct {
	problem      *problemdomain.Problem
	list         []problemdomain.Problem
	filters      problemdomain.Filters
	created      *problemdomain.CreateInput
	saved        bool
	statuses     map[string]string
	statusViewer string
	statusIDs    []string
	tags         []problemdomain.Tag
}

func (f *fakeProblemRepository) Access(ctx context.Context, id, _ string) (problemdomain.Access, error) {
	item, err := f.Get(ctx, id)
	if err != nil {
		return problemdomain.Access{}, err
	}
	return problemdomain.Access{Permissions: problemdomain.Permissions{View: item.Visibility == "public"}}, nil
}

func (*fakeProblemRepository) Grants(context.Context, string) ([]problemdomain.AccessGrant, error) {
	return nil, nil
}
func (*fakeProblemRepository) SetGrant(context.Context, string, problemdomain.GrantInput) error {
	return nil
}
func (*fakeProblemRepository) RemoveGrant(context.Context, string, int64) error { return nil }
func (*fakeProblemRepository) Transfer(context.Context, string, string) error   { return nil }

func (f *fakeProblemRepository) List(_ context.Context, filters problemdomain.Filters) ([]problemdomain.Problem, int, error) {
	f.filters = filters
	return append([]problemdomain.Problem(nil), f.list...), len(f.list), nil
}

func (f *fakeProblemRepository) UserStatuses(_ context.Context, viewerID string, problemIDs []string) (map[string]string, error) {
	f.statusViewer = viewerID
	f.statusIDs = problemIDs
	return f.statuses, nil
}

func (f *fakeProblemRepository) Tags(context.Context) ([]problemdomain.Tag, error) {
	return f.tags, nil
}

func (f *fakeProblemRepository) Get(context.Context, string) (*problemdomain.Problem, error) {
	if f.problem == nil {
		return &problemdomain.Problem{ID: "problem-1", Visibility: "public"}, nil
	}
	return f.problem, nil
}

func (f *fakeProblemRepository) GetWorkspace(ctx context.Context, id string) (*problemdomain.Problem, error) {
	return f.Get(ctx, id)
}

func (f *fakeProblemRepository) Create(_ context.Context, _ string, input *problemdomain.CreateInput) (*problemdomain.Problem, error) {
	f.created = input
	return &problemdomain.Problem{ID: "problem-1", Title: input.Title}, nil
}

func (f *fakeProblemRepository) Update(context.Context, string, *problemdomain.UpdateInput) (*problemdomain.Problem, error) {
	return f.problem, nil
}

func (f *fakeProblemRepository) Delete(context.Context, string) error { return nil }

func (f *fakeProblemRepository) SaveTestdata(context.Context, string, []byte, string) (int, string, error) {
	f.saved = true
	return 1, "sha256", nil
}

func TestService(t *testing.T) { RegisterFailHandler(Fail); RunSpecs(t, "Problem Service") }
