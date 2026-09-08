package application

import (
	"context"
	"strings"

	consoledomain "github.com/RimuruChan/Vertex/server/internal/console/domain"
)

type Service struct{ repository consoledomain.Repository }

func NewService(repository consoledomain.Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Stats(ctx context.Context) (*consoledomain.Stats, error) {
	return s.repository.Stats(ctx)
}

// ---------- accounts ----------

func (s *Service) ListAccounts(ctx context.Context, filters consoledomain.AccountFilters) ([]consoledomain.AccountSummary, int, error) {
	if filters.Limit <= 0 || filters.Limit > 100 {
		filters.Limit = 20
	}
	filters.Keyword = strings.TrimSpace(filters.Keyword)
	if filters.Role != "" && filters.Role != "user" && filters.Role != "admin" {
		return nil, 0, consoledomain.Invalid("role must be user or admin")
	}
	return s.repository.ListAccounts(ctx, filters)
}

// UpdateAccount applies moderation changes. The acting administrator is passed
// in so the service can refuse the two edits that would lock an installation
// out of its own administration.
func (s *Service) UpdateAccount(
	ctx context.Context, actorID, userID string, update consoledomain.AccountUpdate,
) (*consoledomain.AccountSummary, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, consoledomain.Invalid("a user is required")
	}
	if update.Role != nil {
		role := strings.TrimSpace(*update.Role)
		if role != "user" && role != "admin" {
			return nil, consoledomain.Invalid("role must be user or admin")
		}
		update.Role = &role
		if userID == actorID && role != "admin" {
			return nil, consoledomain.ErrForbidden
		}
	}
	if update.Rating != nil && (*update.Rating < 0 || *update.Rating > 10000) {
		return nil, consoledomain.Invalid("rating must be between 0 and 10000")
	}
	if update.Disabled != nil && *update.Disabled && userID == actorID {
		// Disabling yourself would end the session that is doing it.
		return nil, consoledomain.ErrForbidden
	}
	update.Reason = strings.TrimSpace(update.Reason)
	if len(update.Reason) > 500 {
		return nil, consoledomain.Invalid("reason must be at most 500 characters")
	}
	return s.repository.UpdateAccount(ctx, userID, update)
}

// ---------- tags ----------

func (s *Service) ListTags(ctx context.Context) ([]consoledomain.Tag, error) {
	return s.repository.ListTags(ctx)
}

func (s *Service) RequireResourceManagement(ctx context.Context, write bool) error {
	return s.repository.RequireResourceManagement(ctx, write)
}

func (s *Service) CreateTag(ctx context.Context, name string) (*consoledomain.Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 64 {
		return nil, consoledomain.Invalid("tag name must contain 1 to 64 bytes")
	}
	return s.repository.CreateTag(ctx, name)
}

func (s *Service) Tag(ctx context.Context, id int64) (*consoledomain.Tag, error) {
	if id <= 0 {
		return nil, consoledomain.Invalid("positive tag ID required")
	}
	return s.repository.Tag(ctx, id)
}

// RenameTag renames a catalogue entry. Renaming onto an existing name is a
// merge, which the store performs atomically.
func (s *Service) RenameTag(ctx context.Context, id int64, name string) (*consoledomain.Tag, error) {
	name = strings.TrimSpace(name)
	if id <= 0 {
		return nil, consoledomain.Invalid("a tag is required")
	}
	if name == "" {
		return nil, consoledomain.Invalid("a tag name is required")
	}
	if len(name) > 64 {
		return nil, consoledomain.Invalid("tag name must be at most 64 characters")
	}
	return s.repository.RenameTag(ctx, id, name)
}

// MergeTags moves every problem from the source tag onto the target and drops
// the source, which is how a duplicate like "dp" and "DP" gets cleaned up.
func (s *Service) MergeTags(ctx context.Context, sourceID, targetID int64) (*consoledomain.Tag, error) {
	if sourceID <= 0 || targetID <= 0 {
		return nil, consoledomain.Invalid("both tags are required")
	}
	if sourceID == targetID {
		return nil, consoledomain.Invalid("a tag cannot be merged into itself")
	}
	return s.repository.MergeTags(ctx, sourceID, targetID)
}

func (s *Service) DeleteTag(ctx context.Context, id int64) error {
	if id <= 0 {
		return consoledomain.Invalid("a tag is required")
	}
	return s.repository.DeleteTag(ctx, id)
}

// ---------- announcements ----------

// ListAnnouncements is a bounded feed. Drafts require domain resource management.
func (s *Service) ListAnnouncements(ctx context.Context, manage bool, limit int) ([]consoledomain.Announcement, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repository.ListAnnouncements(ctx, !manage, limit)
}

func (s *Service) AnnouncementPage(ctx context.Context, manage bool, f consoledomain.AnnouncementFilters) ([]consoledomain.Announcement, int, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 20
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	f.Keyword = strings.TrimSpace(f.Keyword)
	if len(f.Keyword) > 200 {
		return nil, 0, consoledomain.Invalid("search keyword is too long")
	}
	return s.repository.AnnouncementPage(ctx, !manage, f)
}

func (s *Service) Announcement(ctx context.Context, id string, manage bool) (*consoledomain.Announcement, error) {
	return s.repository.Announcement(ctx, id, manage)
}

func (s *Service) CreateAnnouncement(ctx context.Context, authorID string, input consoledomain.AnnouncementInput) (*consoledomain.Announcement, error) {
	prepared, err := consoledomain.PrepareAnnouncement(input)
	if err != nil {
		return nil, err
	}
	return s.repository.CreateAnnouncement(ctx, authorID, *prepared)
}

func (s *Service) UpdateAnnouncement(ctx context.Context, id string, input consoledomain.AnnouncementInput) (*consoledomain.Announcement, error) {
	prepared, err := consoledomain.PrepareAnnouncement(input)
	if err != nil {
		return nil, err
	}
	return s.repository.UpdateAnnouncement(ctx, id, *prepared)
}

func (s *Service) DeleteAnnouncement(ctx context.Context, id string) error {
	return s.repository.DeleteAnnouncement(ctx, id)
}
