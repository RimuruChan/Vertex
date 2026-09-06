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
	Update(ctx context.Context, id string, input EditorialInput) (*Editorial, error)
	Delete(ctx context.Context, id string) error
	Vote(ctx context.Context, id, userID string, up bool) (int, error)
	// HasSolved reports whether the viewer ever had an accepted submission for
	// the problem, which is what the spoiler gate turns on.
	HasSolved(ctx context.Context, problemID, userID string) (bool, error)
}

type DiscussionRepository interface {
	ListByProblem(ctx context.Context, problemID string) ([]DiscussionPost, error)
	ListByEditorial(ctx context.Context, editorialID string) ([]DiscussionPost, error)
	ListByContest(ctx context.Context, contestID string) ([]DiscussionPost, error)
	Get(ctx context.Context, postID int64) (*DiscussionPost, error)
	CreateProblemPost(ctx context.Context, problemID, authorID, contentMD string, parentID *int64) (*DiscussionPost, error)
	CreateEditorialPost(ctx context.Context, editorialID, authorID, contentMD string) (*DiscussionPost, error)
	CreateContestPost(ctx context.Context, contestID, authorID, contentMD string, parentID *int64) (*DiscussionPost, error)
	Update(ctx context.Context, postID int64, contentMD string) (*DiscussionPost, error)
	IsPostOwner(ctx context.Context, postID int64, userID string) (bool, error)
	Delete(ctx context.Context, postID int64) error
}

// ScopeAccess is the content domain's view of target visibility. Its concrete
// implementation may read other domains' tables, but content rules depend on
// only these two yes/no questions.
type ScopeAccess interface {
	CanViewProblem(ctx context.Context, problemID, userID string, admin bool) (bool, error)
	CanViewContest(ctx context.Context, contestID, userID string, admin bool) (bool, error)
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

// ListEditorials returns published editorials, applying the spoiler gate to
// every body it hands back.
func (s *Service) ListEditorials(ctx context.Context, filters EditorialFilters) ([]EditorialSummary, int, error) {
	if filters.Limit <= 0 || filters.Limit > 100 {
		filters.Limit = 20
	}
	filters.Keyword = strings.TrimSpace(filters.Keyword)
	items, total, err := s.editorials.List(ctx, filters)
	if err != nil {
		return nil, 0, err
	}
	for index := range items {
		if err := s.applySummaryGate(ctx, &items[index], filters.ViewerID, filters.Admin); err != nil {
			return nil, 0, err
		}
	}
	return items, total, nil
}

// ListByProblem is the problem page's editorial tab.
func (s *Service) ListByProblem(ctx context.Context, problemID, viewerID string, admin bool) ([]EditorialSummary, error) {
	items, _, err := s.ListEditorials(ctx, EditorialFilters{
		ProblemID: problemID, ViewerID: viewerID, Admin: admin, Sort: "votes", Limit: 100,
	})
	return items, err
}

func (s *Service) applySummaryGate(
	ctx context.Context, item *EditorialSummary, viewerID string, admin bool,
) error {
	if !item.SolvedOnly || item.CanEdit(viewerID, admin) {
		return nil
	}
	if viewerID != "" {
		solved, err := s.editorials.HasSolved(ctx, item.ProblemID, viewerID)
		if err != nil {
			return err
		}
		if solved {
			return nil
		}
	}
	item.Locked = true
	return nil
}

// GetEditorial returns one editorial, hiding a draft from everyone but its
// author and withholding the body when the spoiler gate applies.
func (s *Service) GetEditorial(ctx context.Context, id, viewerID string, admin bool) (*Editorial, error) {
	item, err := s.editorials.Get(ctx, id, viewerID)
	if err != nil {
		return nil, err
	}
	if item.Status != StatusPublished && !item.CanEdit(viewerID, admin) {
		return nil, ErrNotFound
	}
	if item.Visibility == VisibilityPrivate && !item.CanEdit(viewerID, admin) {
		return nil, ErrNotFound
	}
	if !item.CanEdit(viewerID, admin) {
		if err := s.requireProblemAccess(ctx, item.ProblemID, viewerID, admin); err != nil {
			return nil, err
		}
	}
	if err := s.applyGate(ctx, item, viewerID, admin); err != nil {
		return nil, err
	}
	return item, nil
}

// applyGate withholds the body of a solved-only editorial from readers who
// have not solved the problem. The metadata stays visible so the reader knows
// an editorial exists and what unlocks it.
func (s *Service) applyGate(ctx context.Context, item *Editorial, viewerID string, admin bool) error {
	if !item.SolvedOnly || item.CanEdit(viewerID, admin) {
		return nil
	}
	if viewerID != "" {
		solved, err := s.editorials.HasSolved(ctx, item.ProblemID, viewerID)
		if err != nil {
			return err
		}
		if solved {
			return nil
		}
	}
	item.ContentMD = ""
	item.Locked = true
	return nil
}

func (s *Service) CreateEditorial(ctx context.Context, authorID string, admin bool, input EditorialInput) (*Editorial, error) {
	if strings.TrimSpace(authorID) == "" {
		return nil, invalid("an author is required")
	}
	prepared, err := prepareEditorial(input)
	if err != nil {
		return nil, err
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
	if !current.CanEdit(viewerID, admin) {
		return nil, ErrForbidden
	}
	// The problem an editorial belongs to is fixed at creation: moving it
	// would silently relocate the discussion attached to it.
	input.ProblemID = current.ProblemID
	prepared, err := prepareEditorial(input)
	if err != nil {
		return nil, err
	}
	return s.editorials.Update(ctx, id, *prepared)
}

func (s *Service) DeleteEditorial(ctx context.Context, id, viewerID string, admin bool) error {
	current, err := s.editorials.Get(ctx, id, viewerID)
	if err != nil {
		return err
	}
	if !current.CanEdit(viewerID, admin) {
		return ErrForbidden
	}
	return s.editorials.Delete(ctx, id)
}

// VoteEditorial records or withdraws one reader's upvote and returns the new
// total. Voting for your own editorial is allowed; it is not worth a rule.
func (s *Service) VoteEditorial(ctx context.Context, id, userID string, admin, up bool) (int, error) {
	if strings.TrimSpace(userID) == "" {
		return 0, ErrForbidden
	}
	item, err := s.GetEditorial(ctx, id, userID, admin)
	if err != nil {
		return 0, err
	}
	if item.Status != StatusPublished {
		return 0, ErrNotFound
	}
	return s.editorials.Vote(ctx, id, userID, up)
}

func prepareEditorial(input EditorialInput) (*EditorialInput, error) {
	input.ProblemID = strings.TrimSpace(input.ProblemID)
	input.Title = strings.TrimSpace(input.Title)
	input.ContentMD = strings.TrimSpace(input.ContentMD)
	if input.ProblemID == "" {
		return nil, invalid("a problem is required")
	}
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

func (s *Service) ListProblemPosts(ctx context.Context, problemID, viewerID string, admin bool) ([]DiscussionPost, error) {
	problemID = strings.TrimSpace(problemID)
	if err := s.requireProblemAccess(ctx, problemID, viewerID, admin); err != nil {
		return nil, err
	}
	return s.discussions.ListByProblem(ctx, problemID)
}

func (s *Service) ListEditorialPosts(ctx context.Context, editorialID, viewerID string, admin bool) ([]DiscussionPost, error) {
	editorialID = strings.TrimSpace(editorialID)
	item, err := s.GetEditorial(ctx, editorialID, viewerID, admin)
	if err != nil {
		return nil, err
	}
	if item.Locked {
		return nil, ErrSpoilerLocked
	}
	return s.discussions.ListByEditorial(ctx, editorialID)
}

func (s *Service) ListContestPosts(ctx context.Context, contestID, viewerID string, admin bool) ([]DiscussionPost, error) {
	contestID = strings.TrimSpace(contestID)
	if err := s.requireContestAccess(ctx, contestID, viewerID, admin); err != nil {
		return nil, err
	}
	return s.discussions.ListByContest(ctx, contestID)
}

func (s *Service) CreateProblemPost(ctx context.Context, problemID, authorID string, admin bool, contentMD string, parentID *int64) (*DiscussionPost, error) {
	if err := checkPost(problemID, authorID, contentMD, parentID); err != nil {
		return nil, err
	}
	problemID = strings.TrimSpace(problemID)
	if err := s.requireProblemAccess(ctx, problemID, authorID, admin); err != nil {
		return nil, err
	}
	if err := s.validateParentScope(ctx, parentID, "problem", problemID); err != nil {
		return nil, err
	}
	return s.discussions.CreateProblemPost(ctx, problemID,
		strings.TrimSpace(authorID), strings.TrimSpace(contentMD), parentID)
}

func (s *Service) CreateEditorialPost(ctx context.Context, editorialID, authorID string, admin bool, contentMD string) (*DiscussionPost, error) {
	if err := checkPost(editorialID, authorID, contentMD, nil); err != nil {
		return nil, err
	}
	editorialID = strings.TrimSpace(editorialID)
	item, err := s.GetEditorial(ctx, editorialID, authorID, admin)
	if err != nil {
		return nil, err
	}
	if item.Locked {
		return nil, ErrSpoilerLocked
	}
	return s.discussions.CreateEditorialPost(ctx, editorialID,
		strings.TrimSpace(authorID), strings.TrimSpace(contentMD))
}

func (s *Service) CreateContestPost(ctx context.Context, contestID, authorID string, admin bool, contentMD string, parentID *int64) (*DiscussionPost, error) {
	if err := checkPost(contestID, authorID, contentMD, parentID); err != nil {
		return nil, err
	}
	contestID = strings.TrimSpace(contestID)
	if err := s.requireContestAccess(ctx, contestID, authorID, admin); err != nil {
		return nil, err
	}
	if err := s.validateParentScope(ctx, parentID, "contest", contestID); err != nil {
		return nil, err
	}
	return s.discussions.CreateContestPost(ctx, contestID,
		strings.TrimSpace(authorID), strings.TrimSpace(contentMD), parentID)
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

func (s *Service) requireContestAccess(ctx context.Context, contestID, userID string, admin bool) error {
	if s.access == nil {
		return ErrAccessUnavailable
	}
	visible, err := s.access.CanViewContest(ctx, contestID, userID, admin)
	if err != nil {
		return err
	}
	if !visible {
		return ErrNotFound
	}
	return nil
}

func (s *Service) validateParentScope(ctx context.Context, parentID *int64, scope, scopeID string) error {
	if parentID == nil {
		return nil
	}
	parent, err := s.discussions.Get(ctx, *parentID)
	if errors.Is(err, ErrNotFound) {
		return invalid("parent post does not exist")
	}
	if err != nil {
		return err
	}

	var matches bool
	switch scope {
	case "problem":
		matches = parent.ProblemID != nil && *parent.ProblemID == scopeID
	case "contest":
		matches = parent.ContestID != nil && *parent.ContestID == scopeID
	}
	if !matches {
		return invalid("parent post belongs to a different discussion")
	}
	return nil
}

// UpdatePost rewrites one's own comment. Administrators moderate by deleting,
// not by editing: rewriting someone's words under their name is worse than
// removing them.
func (s *Service) UpdatePost(ctx context.Context, postID int64, userID, contentMD string) (*DiscussionPost, error) {
	contentMD = strings.TrimSpace(contentMD)
	if postID <= 0 || strings.TrimSpace(userID) == "" {
		return nil, invalid("post and user are required")
	}
	if contentMD == "" {
		return nil, invalid("content is required")
	}
	if len(contentMD) > maxPostContent {
		return nil, invalid("content is too long")
	}
	current, err := s.discussions.Get(ctx, postID)
	if err != nil {
		return nil, err
	}
	if !current.CanEdit(userID) {
		return nil, ErrForbidden
	}
	return s.discussions.Update(ctx, postID, contentMD)
}

func (s *Service) DeletePost(ctx context.Context, postID int64, userID string, admin bool) error {
	if postID <= 0 || strings.TrimSpace(userID) == "" {
		return invalid("post and user are required")
	}
	if !admin {
		owner, err := s.discussions.IsPostOwner(ctx, postID, userID)
		if err != nil {
			return err
		}
		if !owner {
			return ErrForbidden
		}
	}
	return s.discussions.Delete(ctx, postID)
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
