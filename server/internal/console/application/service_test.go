package application_test

import (
	"context"
	"strings"
	"testing"

	consoleapp "github.com/RimuruChan/Vertex/server/internal/console/application"
	consoledomain "github.com/RimuruChan/Vertex/server/internal/console/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fakeRepository struct {
	filters consoledomain.AccountFilters
	update  consoledomain.AccountUpdate
	updated string
	renamed string
	merged  [2]int64
	input   consoledomain.AnnouncementInput
	// publishedOnly records what the list call asked for.
	publishedOnly bool
}

func (r *fakeRepository) RequireResourceManagement(context.Context, bool) error { return nil }
func (r *fakeRepository) CreateTag(_ context.Context, name string) (*consoledomain.Tag, error) {
	return &consoledomain.Tag{Name: name}, nil
}
func (r *fakeRepository) Tag(context.Context, int64) (*consoledomain.Tag, error) {
	return &consoledomain.Tag{}, nil
}
func (r *fakeRepository) Announcement(context.Context, string, bool) (*consoledomain.Announcement, error) {
	return &consoledomain.Announcement{}, nil
}
func (r *fakeRepository) AnnouncementPage(_ context.Context, publishedOnly bool, _ consoledomain.AnnouncementFilters) ([]consoledomain.Announcement, int, error) {
	r.publishedOnly = publishedOnly
	return nil, 0, nil
}

func (r *fakeRepository) Stats(context.Context) (*consoledomain.Stats, error) {
	return &consoledomain.Stats{Users: 3}, nil
}

func (r *fakeRepository) ListAccounts(_ context.Context, filters consoledomain.AccountFilters) ([]consoledomain.AccountSummary, int, error) {
	r.filters = filters
	return []consoledomain.AccountSummary{{ID: "user-1", Username: "alice"}}, 1, nil
}

func (r *fakeRepository) UpdateAccount(_ context.Context, userID string, update consoledomain.AccountUpdate) (*consoledomain.AccountSummary, error) {
	r.updated, r.update = userID, update
	return &consoledomain.AccountSummary{ID: userID}, nil
}

func (r *fakeRepository) ListTags(context.Context) ([]consoledomain.Tag, error) { return nil, nil }

func (r *fakeRepository) RenameTag(_ context.Context, _ int64, name string) (*consoledomain.Tag, error) {
	r.renamed = name
	return &consoledomain.Tag{Name: name}, nil
}

func (r *fakeRepository) MergeTags(_ context.Context, sourceID, targetID int64) (*consoledomain.Tag, error) {
	r.merged = [2]int64{sourceID, targetID}
	return &consoledomain.Tag{ID: targetID}, nil
}

func (r *fakeRepository) DeleteTag(context.Context, int64) error { return nil }

func (r *fakeRepository) ListAnnouncements(_ context.Context, publishedOnly bool, _ int) ([]consoledomain.Announcement, error) {
	r.publishedOnly = publishedOnly
	return nil, nil
}

func (r *fakeRepository) CreateAnnouncement(_ context.Context, _ string, input consoledomain.AnnouncementInput) (*consoledomain.Announcement, error) {
	r.input = input
	return &consoledomain.Announcement{Title: input.Title}, nil
}

func (r *fakeRepository) UpdateAnnouncement(_ context.Context, _ string, input consoledomain.AnnouncementInput) (*consoledomain.Announcement, error) {
	r.input = input
	return &consoledomain.Announcement{Title: input.Title}, nil
}

func (r *fakeRepository) DeleteAnnouncement(context.Context, string) error { return nil }

var _ = Describe("Account moderation", func() {
	ctx := context.Background()

	It("refuses to let an administrator demote themselves", func() {
		service := consoleapp.NewService(&fakeRepository{})
		role := "user"
		_, err := service.UpdateAccount(ctx, "admin-1", "admin-1", consoledomain.AccountUpdate{Role: &role})
		Expect(err).To(MatchError(consoledomain.ErrForbidden))
	})

	It("refuses to let an administrator disable themselves", func() {
		service := consoleapp.NewService(&fakeRepository{})
		disabled := true
		_, err := service.UpdateAccount(ctx, "admin-1", "admin-1", consoledomain.AccountUpdate{Disabled: &disabled})
		Expect(err).To(MatchError(consoledomain.ErrForbidden))
	})

	It("allows an administrator to demote a different administrator", func() {
		repository := &fakeRepository{}
		service := consoleapp.NewService(repository)
		role := "user"
		_, err := service.UpdateAccount(ctx, "admin-1", "admin-2", consoledomain.AccountUpdate{Role: &role})
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.updated).To(Equal("admin-2"))
	})

	It("rejects an unknown role", func() {
		service := consoleapp.NewService(&fakeRepository{})
		role := "superuser"
		_, err := service.UpdateAccount(ctx, "admin-1", "user-1", consoledomain.AccountUpdate{Role: &role})
		Expect(err).To(MatchError(consoledomain.ErrInvalidInput))
	})

	It("rejects a rating outside the allowed range", func() {
		service := consoleapp.NewService(&fakeRepository{})
		rating := -5
		_, err := service.UpdateAccount(ctx, "admin-1", "user-1", consoledomain.AccountUpdate{Rating: &rating})
		Expect(err).To(MatchError(consoledomain.ErrInvalidInput))
	})

	It("leaves untouched fields alone", func() {
		repository := &fakeRepository{}
		service := consoleapp.NewService(repository)
		rating := 1500
		_, err := service.UpdateAccount(ctx, "admin-1", "user-1", consoledomain.AccountUpdate{Rating: &rating})
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.update.Role).To(BeNil())
		Expect(repository.update.Disabled).To(BeNil())
		Expect(*repository.update.Rating).To(Equal(1500))
	})

	It("clamps the account page size", func() {
		repository := &fakeRepository{}
		service := consoleapp.NewService(repository)
		_, _, err := service.ListAccounts(ctx, consoledomain.AccountFilters{Limit: 9999})
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.filters.Limit).To(Equal(20))
	})

	It("rejects an unknown role filter", func() {
		service := consoleapp.NewService(&fakeRepository{})
		_, _, err := service.ListAccounts(ctx, consoledomain.AccountFilters{Role: "root"})
		Expect(err).To(MatchError(consoledomain.ErrInvalidInput))
	})
})

var _ = Describe("Tag catalogue", func() {
	ctx := context.Background()

	It("requires a non-empty name", func() {
		service := consoleapp.NewService(&fakeRepository{})
		_, err := service.RenameTag(ctx, 1, "   ")
		Expect(err).To(MatchError(consoledomain.ErrInvalidInput))
	})

	It("trims a renamed tag", func() {
		repository := &fakeRepository{}
		service := consoleapp.NewService(repository)
		_, err := service.RenameTag(ctx, 1, "  动态规划  ")
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.renamed).To(Equal("动态规划"))
	})

	It("refuses to merge a tag into itself", func() {
		service := consoleapp.NewService(&fakeRepository{})
		_, err := service.MergeTags(ctx, 5, 5)
		Expect(err).To(MatchError(consoledomain.ErrInvalidInput))
	})

	It("passes source and target through in order", func() {
		repository := &fakeRepository{}
		service := consoleapp.NewService(repository)
		_, err := service.MergeTags(ctx, 5, 9)
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.merged).To(Equal([2]int64{5, 9}))
	})
})

var _ = Describe("Announcements", func() {
	ctx := context.Background()

	It("requires a title", func() {
		service := consoleapp.NewService(&fakeRepository{})
		_, err := service.CreateAnnouncement(ctx, "admin-1", consoledomain.AnnouncementInput{Title: " "})
		Expect(err).To(MatchError(consoledomain.ErrInvalidInput))
	})

	It("rejects an over-long title", func() {
		service := consoleapp.NewService(&fakeRepository{})
		_, err := service.CreateAnnouncement(ctx, "admin-1", consoledomain.AnnouncementInput{Title: strings.Repeat("a", 300)})
		Expect(err).To(MatchError(consoledomain.ErrInvalidInput))
	})

	It("shows drafts to administrators only", func() {
		repository := &fakeRepository{}
		service := consoleapp.NewService(repository)

		_, err := service.ListAnnouncements(ctx, false, 10)
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.publishedOnly).To(BeTrue())

		_, err = service.ListAnnouncements(ctx, true, 10)
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.publishedOnly).To(BeFalse())
	})
})

func TestService(t *testing.T) { RegisterFailHandler(Fail); RunSpecs(t, "Console service") }
