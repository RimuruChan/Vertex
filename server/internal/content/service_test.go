package content

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Service", func() {
	It("validates and trims editorial content before persistence", func() {
		editorials := &fakeEditorialRepository{}
		service := NewService(editorials, &fakeDiscussionRepository{})
		_, err := service.CreateEditorial(context.Background(), " problem-1 ", " user-1 ", " title ", " body ")
		Expect(err).NotTo(HaveOccurred())
		Expect(editorials.input).To(Equal([]string{"problem-1", "user-1", "title", "body"}))

		_, err = service.CreateEditorial(context.Background(), "problem-1", "user-1", "", "body")
		Expect(errors.Is(err, ErrInvalidInput)).To(BeTrue())
	})

	It("validates discussion parent identifiers", func() {
		service := NewService(&fakeEditorialRepository{}, &fakeDiscussionRepository{})
		parentID := int64(0)
		_, err := service.CreateProblemPost(context.Background(), "problem-1", "user-1", "body", &parentID)
		Expect(errors.Is(err, ErrInvalidInput)).To(BeTrue())
	})

	It("allows owners and administrators to delete posts", func() {
		repository := &fakeDiscussionRepository{owner: false}
		service := NewService(&fakeEditorialRepository{}, repository)
		Expect(service.DeletePost(context.Background(), 1, "user-1", false)).To(MatchError(ErrForbidden))
		Expect(repository.deleted).To(BeFalse())

		Expect(service.DeletePost(context.Background(), 1, "admin-1", true)).To(Succeed())
		Expect(repository.deleted).To(BeTrue())
	})
})

type fakeEditorialRepository struct {
	input []string
}

func (f *fakeEditorialRepository) ListByProblem(context.Context, string) ([]Editorial, error) {
	return nil, nil
}

func (f *fakeEditorialRepository) Get(context.Context, string) (*Editorial, error) {
	return nil, nil
}

func (f *fakeEditorialRepository) Create(_ context.Context, problemID, authorID, title, body string) (*Editorial, error) {
	f.input = []string{problemID, authorID, title, body}
	return &Editorial{ID: "editorial-1"}, nil
}

type fakeDiscussionRepository struct {
	owner   bool
	deleted bool
}

func (f *fakeDiscussionRepository) ListByProblem(context.Context, string) ([]DiscussionPost, error) {
	return nil, nil
}

func (f *fakeDiscussionRepository) ListByEditorial(context.Context, string) ([]DiscussionPost, error) {
	return nil, nil
}

func (f *fakeDiscussionRepository) CreateProblemPost(context.Context, string, string, string, *int64) (*DiscussionPost, error) {
	return &DiscussionPost{ID: 1}, nil
}

func (f *fakeDiscussionRepository) CreateEditorialPost(context.Context, string, string, string) (*DiscussionPost, error) {
	return &DiscussionPost{ID: 1}, nil
}

func (f *fakeDiscussionRepository) IsPostOwner(context.Context, int64, string) (bool, error) {
	return f.owner, nil
}

func (f *fakeDiscussionRepository) Delete(context.Context, int64) error {
	f.deleted = true
	return nil
}
