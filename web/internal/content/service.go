package content

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrInvalidInput = errors.New("invalid content input")
	ErrNotFound     = errors.New("content not found")
	ErrForbidden    = errors.New("content forbidden")
)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
func (e *ValidationError) Unwrap() error { return ErrInvalidInput }

type EditorialRepository interface {
	ListByProblem(ctx context.Context, problemID string) ([]Editorial, error)
	Get(ctx context.Context, id string) (*Editorial, error)
	Create(ctx context.Context, problemID, authorID, title, contentMD string) (*Editorial, error)
}

type DiscussionRepository interface {
	ListByProblem(ctx context.Context, problemID string) ([]DiscussionPost, error)
	ListByEditorial(ctx context.Context, editorialID string) ([]DiscussionPost, error)
	CreateProblemPost(ctx context.Context, problemID, authorID, contentMD string, parentID *int64) (*DiscussionPost, error)
	CreateEditorialPost(ctx context.Context, editorialID, authorID, contentMD string) (*DiscussionPost, error)
	IsPostOwner(ctx context.Context, postID int64, userID string) (bool, error)
	Delete(ctx context.Context, postID int64) error
}

type Service struct {
	editorials  EditorialRepository
	discussions DiscussionRepository
}

func NewService(editorials EditorialRepository, discussions DiscussionRepository) *Service {
	return &Service{editorials: editorials, discussions: discussions}
}

func (s *Service) ListEditorials(ctx context.Context, problemID string) ([]Editorial, error) {
	return s.editorials.ListByProblem(ctx, problemID)
}

func (s *Service) GetEditorial(ctx context.Context, id string) (*Editorial, error) {
	return s.editorials.Get(ctx, id)
}

func (s *Service) CreateEditorial(ctx context.Context, problemID, authorID, title, contentMD string) (*Editorial, error) {
	problemID, authorID, title, contentMD = strings.TrimSpace(problemID), strings.TrimSpace(authorID), strings.TrimSpace(title), strings.TrimSpace(contentMD)
	if problemID == "" || authorID == "" || title == "" || contentMD == "" {
		return nil, &ValidationError{Message: "problem, author, title and content are required"}
	}
	return s.editorials.Create(ctx, problemID, authorID, title, contentMD)
}

func (s *Service) ListProblemPosts(ctx context.Context, problemID string) ([]DiscussionPost, error) {
	return s.discussions.ListByProblem(ctx, problemID)
}

func (s *Service) ListEditorialPosts(ctx context.Context, editorialID string) ([]DiscussionPost, error) {
	return s.discussions.ListByEditorial(ctx, editorialID)
}

func (s *Service) CreateProblemPost(ctx context.Context, problemID, authorID, contentMD string, parentID *int64) (*DiscussionPost, error) {
	problemID, authorID, contentMD = strings.TrimSpace(problemID), strings.TrimSpace(authorID), strings.TrimSpace(contentMD)
	if problemID == "" || authorID == "" || contentMD == "" {
		return nil, &ValidationError{Message: "problem, author and content are required"}
	}
	if parentID != nil && *parentID <= 0 {
		return nil, &ValidationError{Message: "parent ID must be positive"}
	}
	return s.discussions.CreateProblemPost(ctx, problemID, authorID, contentMD, parentID)
}

func (s *Service) CreateEditorialPost(ctx context.Context, editorialID, authorID, contentMD string) (*DiscussionPost, error) {
	editorialID, authorID, contentMD = strings.TrimSpace(editorialID), strings.TrimSpace(authorID), strings.TrimSpace(contentMD)
	if editorialID == "" || authorID == "" || contentMD == "" {
		return nil, &ValidationError{Message: "editorial, author and content are required"}
	}
	return s.discussions.CreateEditorialPost(ctx, editorialID, authorID, contentMD)
}

func (s *Service) DeletePost(ctx context.Context, postID int64, userID string, admin bool) error {
	if postID <= 0 || strings.TrimSpace(userID) == "" {
		return &ValidationError{Message: "post and user are required"}
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
