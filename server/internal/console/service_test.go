package console_test

import (
	"context"
	"strings"

	consoleapp "github.com/RimuruChan/Vertex/server/internal/console"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fakeRepository struct {
	filters consoleapp.AccountFilters
	update  consoleapp.AccountUpdate
	updated string
	renamed string
	merged  [2]int64
	input   consoleapp.AnnouncementInput
	// publishedOnly records what the list call asked for.
	publishedOnly bool
}

func (r *fakeRepository) RequireResourceManagement(context.Context, bool) error { return nil }
func (r *fakeRepository) CreateTag(_ context.Context, name string) (*consoleapp.Tag, error) {
	return &consoleapp.Tag{Name: name}, nil
}
func (r *fakeRepository) Tag(context.Context, int64) (*consoleapp.Tag, error) {
	return &consoleapp.Tag{}, nil
}
func (r *fakeRepository) Announcement(context.Context, string, bool) (*consoleapp.Announcement, error) {
	return &consoleapp.Announcement{}, nil
}
func (r *fakeRepository) AnnouncementPage(_ context.Context, publishedOnly bool, _ consoleapp.AnnouncementFilters) ([]consoleapp.Announcement, int, error) {
	r.publishedOnly = publishedOnly
	return nil, 0, nil
}

func (r *fakeRepository) Stats(context.Context) (*consoleapp.Stats, error) {
	return &consoleapp.Stats{Users: 3}, nil
}

func (r *fakeRepository) ListAccounts(_ context.Context, filters consoleapp.AccountFilters) ([]consoleapp.AccountSummary, int, error) {
	r.filters = filters
	return []consoleapp.AccountSummary{{ID: "user-1", Username: "alice"}}, 1, nil
}

func (r *fakeRepository) UpdateAccount(_ context.Context, userID string, update consoleapp.AccountUpdate) (*consoleapp.AccountSummary, error) {
	r.updated, r.update = userID, update
	return &consoleapp.AccountSummary{ID: userID}, nil
}

func (r *fakeRepository) ListTags(context.Context) ([]consoleapp.Tag, error) { return nil, nil }

func (r *fakeRepository) RenameTag(_ context.Context, _ int64, name string) (*consoleapp.Tag, error) {
	r.renamed = name
	return &consoleapp.Tag{Name: name}, nil
}

func (r *fakeRepository) MergeTags(_ context.Context, sourceID, targetID int64) (*consoleapp.Tag, error) {
	r.merged = [2]int64{sourceID, targetID}
	return &consoleapp.Tag{ID: targetID}, nil
}

func (r *fakeRepository) DeleteTag(context.Context, int64) error { return nil }

func (r *fakeRepository) ListAnnouncements(_ context.Context, publishedOnly bool, _ int) ([]consoleapp.Announcement, error) {
	r.publishedOnly = publishedOnly
	return nil, nil
}

func (r *fakeRepository) CreateAnnouncement(_ context.Context, _ string, input consoleapp.AnnouncementInput) (*consoleapp.Announcement, error) {
	r.input = input
	return &consoleapp.Announcement{Title: input.Title}, nil
}

func (r *fakeRepository) UpdateAnnouncement(_ context.Context, _ string, input consoleapp.AnnouncementInput) (*consoleapp.Announcement, error) {
	r.input = input
	return &consoleapp.Announcement{Title: input.Title}, nil
}

func (r *fakeRepository) DeleteAnnouncement(context.Context, string) error { return nil }

var _ = Describe("Account moderation", func() {
	ctx := context.Background()

	It("refuses to let an administrator demote themselves", func() {
		service := consoleapp.NewService(&fakeRepository{})
		role := "user"
		_, err := service.UpdateAccount(ctx, "admin-1", "admin-1",
			consoleapp.AccountUpdate{Role: &role})
		Expect(err).To(MatchError(consoleapp.ErrForbidden))
	})

	It("refuses to let an administrator disable themselves", func() {
		service := consoleapp.NewService(&fakeRepository{})
		disabled := true
		_, err := service.UpdateAccount(ctx, "admin-1", "admin-1",
			consoleapp.AccountUpdate{Disabled: &disabled})
		Expect(err).To(MatchError(consoleapp.ErrForbidden))
	})

	It("allows an administrator to demote a different administrator", func() {
		repository := &fakeRepository{}
		service := consoleapp.NewService(repository)
		role := "user"
		_, err := service.UpdateAccount(ctx, "admin-1", "admin-2",
			consoleapp.AccountUpdate{Role: &role})
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.updated).To(Equal("admin-2"))
	})

	It("rejects an unknown role", func() {
		service := consoleapp.NewService(&fakeRepository{})
		role := "superuser"
		_, err := service.UpdateAccount(ctx, "admin-1", "user-1",
			consoleapp.AccountUpdate{Role: &role})
		Expect(err).To(MatchError(consoleapp.ErrInvalidInput))
	})

	It("rejects a rating outside the allowed range", func() {
		service := consoleapp.NewService(&fakeRepository{})
		rating := -5
		_, err := service.UpdateAccount(ctx, "admin-1", "user-1",
			consoleapp.AccountUpdate{Rating: &rating})
		Expect(err).To(MatchError(consoleapp.ErrInvalidInput))
	})

	It("leaves untouched fields alone", func() {
		repository := &fakeRepository{}
		service := consoleapp.NewService(repository)
		rating := 1500
		_, err := service.UpdateAccount(ctx, "admin-1", "user-1",
			consoleapp.AccountUpdate{Rating: &rating})
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.update.Role).To(BeNil())
		Expect(repository.update.Disabled).To(BeNil())
		Expect(*repository.update.Rating).To(Equal(1500))
	})

	It("clamps the account page size", func() {
		repository := &fakeRepository{}
		service := consoleapp.NewService(repository)
		_, _, err := service.ListAccounts(ctx, consoleapp.AccountFilters{Limit: 9999})
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.filters.Limit).To(Equal(20))
	})

	It("rejects an unknown role filter", func() {
		service := consoleapp.NewService(&fakeRepository{})
		_, _, err := service.ListAccounts(ctx, consoleapp.AccountFilters{Role: "root"})
		Expect(err).To(MatchError(consoleapp.ErrInvalidInput))
	})
})

var _ = Describe("Tag catalogue", func() {
	ctx := context.Background()

	It("requires a non-empty name", func() {
		service := consoleapp.NewService(&fakeRepository{})
		_, err := service.RenameTag(ctx, 1, "   ")
		Expect(err).To(MatchError(consoleapp.ErrInvalidInput))
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
		Expect(err).To(MatchError(consoleapp.ErrInvalidInput))
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
		_, err := service.CreateAnnouncement(ctx, "admin-1", consoleapp.AnnouncementInput{Title: " "})
		Expect(err).To(MatchError(consoleapp.ErrInvalidInput))
	})

	It("rejects an over-long title", func() {
		service := consoleapp.NewService(&fakeRepository{})
		_, err := service.CreateAnnouncement(ctx, "admin-1",
			consoleapp.AnnouncementInput{Title: strings.Repeat("a", 300)})
		Expect(err).To(MatchError(consoleapp.ErrInvalidInput))
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
