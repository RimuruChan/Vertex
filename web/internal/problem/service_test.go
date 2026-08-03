package problem_test

import (
	"context"
	"errors"

	problemapp "github.com/RimuruChan/Vertex/web/internal/problem"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Service", func() {
	It("enforces public visibility for non-admin lists", func() {
		repository := &fakeProblemRepository{}
		service := problemapp.NewService(repository, repository)
		_, _, err := service.List(context.Background(), problemapp.Filters{Visibility: "private"}, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.filters.Visibility).To(Equal("public"))
	})

	It("hides unpublished problems from public callers", func() {
		repository := &fakeProblemRepository{problem: &problemapp.Problem{ID: "problem-1", Visibility: "draft"}}
		service := problemapp.NewService(repository, repository)
		_, err := service.Get(context.Background(), "problem-1", false)
		Expect(err).To(MatchError(problemapp.ErrNotFound))
	})

	It("normalizes defaults and tags before persistence", func() {
		repository := &fakeProblemRepository{}
		service := problemapp.NewService(repository, repository)
		_, err := service.Create(context.Background(), "author-1", problemapp.CreateInput{
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
		func(input problemapp.CreateInput) {
			repository := &fakeProblemRepository{}
			service := problemapp.NewService(repository, repository)
			_, err := service.Create(context.Background(), "author-1", input)
			Expect(errors.Is(err, problemapp.ErrInvalidInput)).To(BeTrue())
			Expect(repository.created).To(BeNil())
		},
		Entry("missing title", problemapp.CreateInput{}),
		Entry("difficulty", problemapp.CreateInput{Title: "x", Difficulty: 11}),
		Entry("time limit", problemapp.CreateInput{Title: "x", TimeLimitMs: -1}),
		Entry("memory limit", problemapp.CreateInput{Title: "x", MemoryLimitKb: -1}),
		Entry("visibility", problemapp.CreateInput{Title: "x", Visibility: "hidden"}),
	)

	It("rejects unsupported checkers before reading persistence", func() {
		repository := &fakeProblemRepository{}
		service := problemapp.NewService(repository, repository)
		_, _, err := service.SaveTestdata(context.Background(), "problem-1", []byte("zip"), "shell")
		Expect(errors.Is(err, problemapp.ErrInvalidInput)).To(BeTrue())
		Expect(repository.saved).To(BeFalse())
	})
})

type fakeProblemRepository struct {
	problem *problemapp.Problem
	filters problemapp.Filters
	created *problemapp.CreateInput
	saved   bool
}

func (f *fakeProblemRepository) List(_ context.Context, filters problemapp.Filters) ([]problemapp.Problem, int, error) {
	f.filters = filters
	return nil, 0, nil
}

func (f *fakeProblemRepository) Get(context.Context, string) (*problemapp.Problem, error) {
	if f.problem == nil {
		return &problemapp.Problem{ID: "problem-1", Visibility: "public"}, nil
	}
	return f.problem, nil
}

func (f *fakeProblemRepository) Create(_ context.Context, _ string, input *problemapp.CreateInput) (*problemapp.Problem, error) {
	f.created = input
	return &problemapp.Problem{ID: "problem-1", Title: input.Title}, nil
}

func (f *fakeProblemRepository) Update(context.Context, string, *problemapp.UpdateInput) (*problemapp.Problem, error) {
	return f.problem, nil
}

func (f *fakeProblemRepository) Delete(context.Context, string) error { return nil }

func (f *fakeProblemRepository) SaveTestdata(context.Context, string, []byte, string) (int, string, error) {
	f.saved = true
	return 1, "sha256", nil
}
