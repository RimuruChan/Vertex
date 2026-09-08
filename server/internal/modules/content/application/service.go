package application

import (
	"context"
	"strings"

	contentdomain "github.com/RimuruChan/Vertex/server/internal/modules/content/domain"
)

type Service struct {
	editorials  contentdomain.EditorialRepository
	discussions contentdomain.DiscussionRepository
	access      contentdomain.ScopeAccess
}

func NewService(editorials contentdomain.EditorialRepository, discussions contentdomain.DiscussionRepository, access contentdomain.ScopeAccess) *Service {
	return &Service{editorials: editorials, discussions: discussions, access: access}
}

// ---------- editorials ----------

// ListEditorials returns body-free summaries with persistence-resolved capabilities.
func (s *Service) ListEditorials(ctx context.Context, filters contentdomain.EditorialFilters) ([]contentdomain.EditorialSummary, int, error) {
	if filters.Limit <= 0 || filters.Limit > 100 {
		filters.Limit = 20
	}
	filters.Keyword = strings.TrimSpace(filters.Keyword)
	return s.editorials.List(ctx, filters)
}

// ListByProblem is the problem page's editorial tab.
func (s *Service) ListByProblem(ctx context.Context, problemID string, viewerID string) ([]contentdomain.EditorialSummary, error) {
	items, _, err := s.ListEditorials(ctx, contentdomain.EditorialFilters{
		ProblemID: problemID, ViewerID: viewerID, Sort: "votes", Limit: 100,
	})
	return items, err
}

// GetEditorial returns the repository's authorized, spoiler-redacted projection.
func (s *Service) GetEditorial(ctx context.Context, id string, viewerID string) (*contentdomain.Editorial, error) {
	return s.editorials.Get(ctx, id, viewerID)
}

func (s *Service) CreateEditorial(ctx context.Context, authorID string, input contentdomain.EditorialInput) (*contentdomain.Editorial, error) {
	if strings.TrimSpace(authorID) == "" {
		return nil, contentdomain.Invalid("an author is required")
	}
	prepared, err := contentdomain.PrepareEditorial(input)
	if err != nil {
		return nil, err
	}
	if prepared.ProblemID == "" {
		return nil, contentdomain.Invalid("a problem is required")
	}
	if err := s.requireProblemAccess(ctx, prepared.ProblemID, authorID); err != nil {
		return nil, err
	}
	return s.editorials.Create(ctx, authorID, *prepared)
}

func (s *Service) UpdateEditorial(ctx context.Context, id string, viewerID string, input contentdomain.EditorialInput) (*contentdomain.Editorial, error) {
	current, err := s.editorials.Get(ctx, id, viewerID)
	if err != nil {
		return nil, err
	}
	if !current.Permissions.Edit {
		return nil, contentdomain.ErrForbidden
	}
	// The problem an editorial belongs to is fixed at creation: moving it
	// would silently relocate the discussion attached to it.
	input.ProblemID = current.ProblemID
	prepared, err := contentdomain.PrepareEditorial(input)
	if err != nil {
		return nil, err
	}
	return s.editorials.Update(ctx, id, viewerID, *prepared)
}

func (s *Service) DeleteEditorial(ctx context.Context, id string, viewerID string) error {
	return s.editorials.Delete(ctx, id, viewerID)
}

// VoteEditorial records or withdraws one reader's upvote and returns the new
// total. Voting for your own editorial is allowed; it is not worth a rule.
func (s *Service) VoteEditorial(ctx context.Context, id string, userID string, up bool) (int, error) {
	if strings.TrimSpace(userID) == "" {
		return 0, contentdomain.ErrForbidden
	}
	return s.editorials.Vote(ctx, id, userID, up)
}

// ---------- discussions ----------

func (s *Service) ListProblemPosts(ctx context.Context, id string, viewerID string) (contentdomain.Thread, error) {
	return s.discussions.ListByProblem(ctx, strings.TrimSpace(id), viewerID)
}
func (s *Service) ListEditorialPosts(ctx context.Context, id string, viewerID string) (contentdomain.Thread, error) {
	return s.discussions.ListByEditorial(ctx, strings.TrimSpace(id), viewerID)
}

func (s *Service) CreateProblemPost(ctx context.Context, problemID string, authorID string, contentMD string, parentID *int64) (*contentdomain.DiscussionPost, error) {
	if err := contentdomain.CheckPost(problemID, authorID, contentMD, parentID); err != nil {
		return nil, err
	}
	problemID = strings.TrimSpace(problemID)
	if err := s.requireProblemAccess(ctx, problemID, authorID); err != nil {
		return nil, err
	}
	return s.discussions.CreateProblemPost(ctx, problemID,
		strings.TrimSpace(authorID), strings.TrimSpace(contentMD), parentID)
}

func (s *Service) CreateEditorialPost(ctx context.Context, editorialID string, authorID string, contentMD string, parentID *int64) (*contentdomain.DiscussionPost, error) {
	if err := contentdomain.CheckPost(editorialID, authorID, contentMD, parentID); err != nil {
		return nil, err
	}
	return s.discussions.CreateEditorialPost(ctx, strings.TrimSpace(editorialID), strings.TrimSpace(authorID), strings.TrimSpace(contentMD), parentID)
}

func (s *Service) requireProblemAccess(ctx context.Context, problemID string, userID string) error {
	if s.access == nil {
		return contentdomain.ErrAccessUnavailable
	}
	visible, err := s.access.CanViewProblem(ctx, problemID, userID)
	if err != nil {
		return err
	}
	if !visible {
		return contentdomain.ErrNotFound
	}
	return nil
}

// UpdatePost rewrites one's own comment. Administrators moderate by deleting,
// not by editing: rewriting someone's words under their name is worse than
// removing them.
func (s *Service) UpdatePost(ctx context.Context, postID int64, userID, contentMD string) (*contentdomain.DiscussionPost, error) {
	if postID <= 0 {
		return nil, contentdomain.Invalid("post ID is required")
	}
	if err := contentdomain.CheckPost("post", userID, contentMD, nil); err != nil {
		return nil, err
	}
	return s.discussions.Update(ctx, postID, userID, strings.TrimSpace(contentMD))
}
func (s *Service) DeletePost(ctx context.Context, postID int64, userID string) error {
	if postID <= 0 || strings.TrimSpace(userID) == "" {
		return contentdomain.Invalid("post and user are required")
	}
	return s.discussions.Delete(ctx, postID, userID)
}
