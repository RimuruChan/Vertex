package content

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrInvalidInput      = errors.New("invalid content input")
	ErrNotFound          = errors.New("content not found")
	ErrForbidden         = errors.New("content forbidden")
	ErrSpoilerLocked     = errors.New("solve the problem before opening this discussion")
	ErrAccessUnavailable = errors.New("content access policy unavailable")
)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
func (e *ValidationError) Unwrap() error { return ErrInvalidInput }

func invalid(message string) error { return &ValidationError{Message: message} }

const (
	maxEditorialTitle   = 200
	maxEditorialContent = 200_000
	maxPostContent      = 20_000
)

type EditorialRepository interface {
	List(ctx context.Context, filters EditorialFilters) ([]EditorialSummary, int, error)
	Get(ctx context.Context, id, viewerID string) (*Editorial, error)
	Create(ctx context.Context, authorID string, input EditorialInput) (*Editorial, error)
	Update(ctx context.Context, id, userID string, input EditorialInput) (*Editorial, error)
	Delete(ctx context.Context, id, userID string) error
	Vote(ctx context.Context, id, userID string, up bool) (int, error)
}

type DiscussionRepository interface {
	ListByProblem(ctx context.Context, problemID, userID string) (Thread, error)
	ListByEditorial(ctx context.Context, editorialID, userID string) (Thread, error)
	CreateProblemPost(ctx context.Context, problemID, authorID, contentMD string, parentID *int64) (*DiscussionPost, error)
	CreateEditorialPost(ctx context.Context, editorialID, authorID, contentMD string, parentID *int64) (*DiscussionPost, error)
	Update(ctx context.Context, postID int64, userID, contentMD string) (*DiscussionPost, error)
	Delete(ctx context.Context, postID int64, userID string) error
}

// ScopeAccess is the content domain's view of target visibility. Its concrete
// implementation may read other domains' tables, but content rules depend on
// this parent visibility check; persistence repeats it inside each mutation.
type ScopeAccess interface {
	CanViewProblem(ctx context.Context, problemID, userID string, admin bool) (bool, error)
}

type Service struct {
	editorials  EditorialRepository
	discussions DiscussionRepository
	access      ScopeAccess
}

func NewService(editorials EditorialRepository, discussions DiscussionRepository, access ScopeAccess) *Service {
	return &Service{editorials: editorials, discussions: discussions, access: access}
}

// ---------- editorials ----------

// ListEditorials returns body-free summaries with persistence-resolved capabilities.
func (s *Service) ListEditorials(ctx context.Context, filters EditorialFilters) ([]EditorialSummary, int, error) {
	if filters.Limit <= 0 || filters.Limit > 100 {
		filters.Limit = 20
	}
	filters.Keyword = strings.TrimSpace(filters.Keyword)
	return s.editorials.List(ctx, filters)
}

// ListByProblem is the problem page's editorial tab.
func (s *Service) ListByProblem(ctx context.Context, problemID, viewerID string, admin bool) ([]EditorialSummary, error) {
	items, _, err := s.ListEditorials(ctx, EditorialFilters{
		ProblemID: problemID, ViewerID: viewerID, Admin: admin, Sort: "votes", Limit: 100,
	})
	return items, err
}

// GetEditorial returns the repository's authorized, spoiler-redacted projection.
func (s *Service) GetEditorial(ctx context.Context, id, viewerID string, _ bool) (*Editorial, error) {
	return s.editorials.Get(ctx, id, viewerID)
}

func (s *Service) CreateEditorial(ctx context.Context, authorID string, admin bool, input EditorialInput) (*Editorial, error) {
	if strings.TrimSpace(authorID) == "" {
		return nil, invalid("an author is required")
	}
	prepared, err := prepareEditorial(input)
	if err != nil {
		return nil, err
	}
	if prepared.ProblemID == "" {
		return nil, invalid("a problem is required")
	}
	if err := s.requireProblemAccess(ctx, prepared.ProblemID, authorID, admin); err != nil {
		return nil, err
	}
	return s.editorials.Create(ctx, authorID, *prepared)
}

func (s *Service) UpdateEditorial(ctx context.Context, id, viewerID string, admin bool, input EditorialInput) (*Editorial, error) {
	current, err := s.editorials.Get(ctx, id, viewerID)
	if err != nil {
		return nil, err
	}
	if !current.Permissions.Edit {
		return nil, ErrForbidden
	}
	// The problem an editorial belongs to is fixed at creation: moving it
	// would silently relocate the discussion attached to it.
	input.ProblemID = current.ProblemID
	prepared, err := prepareEditorial(input)
	if err != nil {
		return nil, err
	}
	return s.editorials.Update(ctx, id, viewerID, *prepared)
}

func (s *Service) DeleteEditorial(ctx context.Context, id, viewerID string, _ bool) error {
	return s.editorials.Delete(ctx, id, viewerID)
}

// VoteEditorial records or withdraws one reader's upvote and returns the new
// total. Voting for your own editorial is allowed; it is not worth a rule.
func (s *Service) VoteEditorial(ctx context.Context, id, userID string, _ bool, up bool) (int, error) {
	if strings.TrimSpace(userID) == "" {
		return 0, ErrForbidden
	}
	return s.editorials.Vote(ctx, id, userID, up)
}
func prepareEditorial(input EditorialInput) (*EditorialInput, error) {
	input.ProblemID = strings.TrimSpace(input.ProblemID)
	input.Title = strings.TrimSpace(input.Title)
	input.ContentMD = strings.TrimSpace(input.ContentMD)
	if input.Title == "" {
		return nil, invalid("a title is required")
	}
	if len(input.Title) > maxEditorialTitle {
		return nil, invalid("title must be at most 200 characters")
	}
	if input.ContentMD == "" {
		return nil, invalid("content is required")
	}
	if len(input.ContentMD) > maxEditorialContent {
		return nil, invalid("content is too long")
	}
	if input.Visibility == "" {
		input.Visibility = VisibilityPublic
	}
	if input.Visibility != VisibilityPublic && input.Visibility != VisibilityPrivate {
		return nil, invalid("visibility must be public or private")
	}
	if input.Status == "" {
		input.Status = StatusPublished
	}
	if input.Status != StatusDraft && input.Status != StatusPublished {
		return nil, invalid("status must be draft or published")
	}
	return &input, nil
}

// ---------- discussions ----------

func (s *Service) ListProblemPosts(ctx context.Context, id, viewerID string, _ bool) (Thread, error) {
	return s.discussions.ListByProblem(ctx, strings.TrimSpace(id), viewerID)
}
func (s *Service) ListEditorialPosts(ctx context.Context, id, viewerID string, _ bool) (Thread, error) {
	return s.discussions.ListByEditorial(ctx, strings.TrimSpace(id), viewerID)
}

func (s *Service) CreateProblemPost(ctx context.Context, problemID, authorID string, admin bool, contentMD string, parentID *int64) (*DiscussionPost, error) {
	if err := checkPost(problemID, authorID, contentMD, parentID); err != nil {
		return nil, err
	}
	problemID = strings.TrimSpace(problemID)
	if err := s.requireProblemAccess(ctx, problemID, authorID, admin); err != nil {
		return nil, err
	}
	return s.discussions.CreateProblemPost(ctx, problemID,
		strings.TrimSpace(authorID), strings.TrimSpace(contentMD), parentID)
}

func (s *Service) CreateEditorialPost(ctx context.Context, editorialID, authorID string, _ bool, contentMD string, parentID *int64) (*DiscussionPost, error) {
	if err := checkPost(editorialID, authorID, contentMD, parentID); err != nil {
		return nil, err
	}
	return s.discussions.CreateEditorialPost(ctx, strings.TrimSpace(editorialID), strings.TrimSpace(authorID), strings.TrimSpace(contentMD), parentID)
}

func (s *Service) requireProblemAccess(ctx context.Context, problemID, userID string, admin bool) error {
	if s.access == nil {
		return ErrAccessUnavailable
	}
	visible, err := s.access.CanViewProblem(ctx, problemID, userID, admin)
	if err != nil {
		return err
	}
	if !visible {
		return ErrNotFound
	}
	return nil
}

// UpdatePost rewrites one's own comment. Administrators moderate by deleting,
// not by editing: rewriting someone's words under their name is worse than
// removing them.
func (s *Service) UpdatePost(ctx context.Context, postID int64, userID, contentMD string) (*DiscussionPost, error) {
	if postID <= 0 {
		return nil, invalid("post ID is required")
	}
	if err := checkPost("post", userID, contentMD, nil); err != nil {
		return nil, err
	}
	return s.discussions.Update(ctx, postID, userID, strings.TrimSpace(contentMD))
}
func (s *Service) DeletePost(ctx context.Context, postID int64, userID string, _ bool) error {
	if postID <= 0 || strings.TrimSpace(userID) == "" {
		return invalid("post and user are required")
	}
	return s.discussions.Delete(ctx, postID, userID)
}
func checkPost(scopeID, authorID, contentMD string, parentID *int64) error {
	if strings.TrimSpace(scopeID) == "" || strings.TrimSpace(authorID) == "" {
		return invalid("scope and author are required")
	}
	contentMD = strings.TrimSpace(contentMD)
	if contentMD == "" {
		return invalid("content is required")
	}
	if len(contentMD) > maxPostContent {
		return invalid("content is too long")
	}
	if parentID != nil && *parentID <= 0 {
		return invalid("parent ID must be positive")
	}
	return nil
}
