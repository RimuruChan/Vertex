package domain

import "context"

// Repository is the persistence boundary for the administration surface.
type Repository interface {
	RequireResourceManagement(ctx context.Context, write bool) error
	CreateTag(ctx context.Context, name string) (*Tag, error)
	Tag(ctx context.Context, id int64) (*Tag, error)
	AnnouncementPage(ctx context.Context, publishedOnly bool, filters AnnouncementFilters) ([]Announcement, int, error)
	Announcement(ctx context.Context, id string, manage bool) (*Announcement, error)
	Stats(ctx context.Context) (*Stats, error)
	ListAccounts(ctx context.Context, filters AccountFilters) ([]AccountSummary, int, error)
	UpdateAccount(ctx context.Context, userID string, update AccountUpdate) (*AccountSummary, error)
	ListTags(ctx context.Context) ([]Tag, error)
	RenameTag(ctx context.Context, id int64, name string) (*Tag, error)
	MergeTags(ctx context.Context, sourceID, targetID int64) (*Tag, error)
	DeleteTag(ctx context.Context, id int64) error
	ListAnnouncements(ctx context.Context, publishedOnly bool, limit int) ([]Announcement, error)
	CreateAnnouncement(ctx context.Context, authorID string, input AnnouncementInput) (*Announcement, error)
	UpdateAnnouncement(ctx context.Context, id string, input AnnouncementInput) (*Announcement, error)
	DeleteAnnouncement(ctx context.Context, id string) error
}
