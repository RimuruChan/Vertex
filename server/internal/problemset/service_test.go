package problemset_test

import (
	"context"
	"strings"

	setapp "github.com/RimuruChan/Vertex/server/internal/problemset"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fakeRepository struct {
	set      *setapp.Set
	filters  setapp.Filters
	upserted *setapp.UpsertInput
	items    []setapp.ItemInput
	deleted  string
}

func (r *fakeRepository) List(_ context.Context, filters setapp.Filters) ([]setapp.Set, int, error) {
	r.filters = filters
	return []setapp.Set{*r.set}, 1, nil
}

func (r *fakeRepository) Get(context.Context, string, string, bool) (*setapp.Set, error) {
	if r.set == nil {
		return nil, setapp.ErrNotFound
	}
	copied := *r.set
	copied.Items = append([]setapp.Item(nil), r.set.Items...)
	return &copied, nil
}

func (r *fakeRepository) Create(_ context.Context, _ string, input setapp.UpsertInput) (*setapp.Set, error) {
	r.upserted = &input
	return r.set, nil
}

func (r *fakeRepository) Update(_ context.Context, _, _ string, _ bool, input setapp.UpsertInput) (*setapp.Set, error) {
	r.upserted = &input
	return r.set, nil
}

func (r *fakeRepository) Delete(_ context.Context, id string) error {
	r.deleted = id
	return nil
}

func (r *fakeRepository) SetItems(_ context.Context, _, _ string, _ bool, items []setapp.ItemInput) error {
	r.items = items
	return nil
}

func owned(userID string) *setapp.Set {
	return &setapp.Set{
		ID: "set-1", Title: "入门 DP", AuthorID: &userID,
		Visibility: setapp.VisibilityPublic,
	}
}

var _ = Describe("Problem set access", func() {
	ctx := context.Background()

	It("hides a private set from everyone but its curator", func() {
		author := "user-1"
		repository := &fakeRepository{set: owned(author)}
		repository.set.Visibility = setapp.VisibilityPrivate
		service := setapp.NewService(repository)

		_, err := service.Get(ctx, "set-1", "someone-else", false)
		Expect(err).To(MatchError(setapp.ErrNotFound))

		item, err := service.Get(ctx, "set-1", author, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.CanEdit(author, false)).To(BeTrue())
	})

	It("lets an administrator read and edit any set", func() {
		repository := &fakeRepository{set: owned("user-1")}
		repository.set.Visibility = setapp.VisibilityPrivate
		service := setapp.NewService(repository)

		item, err := service.Get(ctx, "set-1", "admin-1", true)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.CanEdit("admin-1", true)).To(BeTrue())
	})

	It("refuses edits from a signed-in reader who does not own the set", func() {
		repository := &fakeRepository{set: owned("user-1")}
		service := setapp.NewService(repository)

		_, err := service.Update(ctx, "set-1", "user-2", false, setapp.UpsertInput{Title: "x"})
		Expect(err).To(MatchError(setapp.ErrForbidden))
		Expect(service.Delete(ctx, "set-1", "user-2", false)).To(MatchError(setapp.ErrForbidden))
	})

	It("shows unpublished problems only to their own author or an administrator", func() {
		author := "user-1"
		otherAuthor := "user-3"
		repository := &fakeRepository{set: owned(author)}
		repository.set.Items = []setapp.Item{
			{ProblemID: "p1", Visibility: "public"},
			{ProblemID: "p2", OwnerID: &author, Visibility: "draft"},
			{ProblemID: "p3", OwnerID: &otherAuthor, Visibility: "private"},
		}
		repository.set.ProblemCount = 3
		service := setapp.NewService(repository)

		reader, err := service.Get(ctx, "set-1", "user-2", false)
		Expect(err).NotTo(HaveOccurred())
		Expect(reader.Items).To(HaveLen(1))
		Expect(reader.ProblemCount).To(Equal(1))

		curator, err := service.Get(ctx, "set-1", author, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(curator.Items).To(HaveLen(2))

		admin, err := service.Get(ctx, "set-1", "admin-1", true)
		Expect(err).NotTo(HaveOccurred())
		Expect(admin.Items).To(HaveLen(3))
	})
})

var _ = Describe("Problem set validation", func() {
	ctx := context.Background()

	It("requires a title", func() {
		service := setapp.NewService(&fakeRepository{set: owned("user-1")})
		_, err := service.Create(ctx, "user-1", setapp.UpsertInput{Title: "  "})
		Expect(err).To(MatchError(setapp.ErrInvalidInput))
	})

	It("defaults visibility to public", func() {
		repository := &fakeRepository{set: owned("user-1")}
		service := setapp.NewService(repository)
		_, err := service.Create(ctx, "user-1", setapp.UpsertInput{Title: "题单"})
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.upserted.Visibility).To(Equal(setapp.VisibilityPublic))
	})

	It("rejects an unknown visibility", func() {
		service := setapp.NewService(&fakeRepository{set: owned("user-1")})
		_, err := service.Create(ctx, "user-1", setapp.UpsertInput{Title: "t", Visibility: "secret"})
		Expect(err).To(MatchError(setapp.ErrInvalidInput))
	})

	It("rejects an over-long title", func() {
		service := setapp.NewService(&fakeRepository{set: owned("user-1")})
		_, err := service.Create(ctx, "user-1", setapp.UpsertInput{Title: strings.Repeat("a", 200)})
		Expect(err).To(MatchError(setapp.ErrInvalidInput))
	})

	It("rejects the same problem twice in one set", func() {
		service := setapp.NewService(&fakeRepository{set: owned("user-1")})
		err := service.SetItems(ctx, "set-1", "user-1", false, []setapp.ItemInput{
			{ProblemID: "p1"}, {ProblemID: "p1"},
		})
		Expect(err).To(MatchError(setapp.ErrInvalidInput))
	})

	It("keeps the curator's order and trims notes", func() {
		repository := &fakeRepository{set: owned("user-1")}
		service := setapp.NewService(repository)
		Expect(service.SetItems(ctx, "set-1", "user-1", false, []setapp.ItemInput{
			{ProblemID: "p2", Note: "  先做这题  "},
			{ProblemID: "p1"},
		})).To(Succeed())
		Expect(repository.items[0].ProblemID).To(Equal("p2"))
		Expect(repository.items[0].Note).To(Equal("先做这题"))
	})

	It("rejects a set larger than the curation limit", func() {
		service := setapp.NewService(&fakeRepository{set: owned("user-1")})
		items := make([]setapp.ItemInput, 501)
		for i := range items {
			items[i] = setapp.ItemInput{ProblemID: string(rune('a' + i%26))}
		}
		err := service.SetItems(ctx, "set-1", "user-1", false, items)
		Expect(err).To(MatchError(setapp.ErrInvalidInput))
	})
})

var _ = Describe("Problem set listing", func() {
	It("clamps the page size", func() {
		repository := &fakeRepository{set: owned("user-1")}
		service := setapp.NewService(repository)
		_, _, err := service.List(context.Background(), setapp.Filters{Limit: 5000})
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.filters.Limit).To(Equal(20))
	})
})
