package problemset

import (
	"context"
	"strings"
)

// Repository is the persistence boundary for curated sets.
type Repository interface {
	List(ctx context.Context, filters Filters) ([]Set, int, error)
	Get(ctx context.Context, id, viewerID string, admin bool) (*Set, error)
	Create(ctx context.Context, authorID string, input UpsertInput) (*Set, error)
	Update(ctx context.Context, id, viewerID string, admin bool, input UpsertInput) (*Set, error)
	Delete(ctx context.Context, id, viewerID string) error
	SetItems(ctx context.Context, id, viewerID string, admin bool, items []ItemInput) error
	Grants(ctx context.Context, id string) ([]AccessGrant, error)
	SetGrant(ctx context.Context, id string, input GrantInput) error
	RemoveGrant(ctx context.Context, id string, grantID int64) error
	Transfer(ctx context.Context, id, username string) error
}

// Service holds the curation rules: what a valid set looks like and who may
// read or change one.
type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }

const (
	maxTitleLength       = 120
	maxDescriptionLength = 20000
	maxNoteLength        = 500
	maxItems             = 500
)

// List returns sets the viewer may see. Private sets of other users are
// filtered in SQL rather than after the fact, so paging stays correct.
func (s *Service) List(ctx context.Context, filters Filters) ([]Set, int, error) {
	if filters.Limit <= 0 || filters.Limit > 100 {
		filters.Limit = 20
	}
	filters.Keyword = strings.TrimSpace(filters.Keyword)
	return s.repository.List(ctx, filters)
}

// Get returns one set with its items and the viewer's progress.
func (s *Service) Get(ctx context.Context, id, viewerID string, admin bool) (*Set, error) {
	return s.repository.Get(ctx, id, viewerID, admin)
}

func (s *Service) Create(ctx context.Context, authorID string, input UpsertInput) (*Set, error) {
	prepared, err := prepare(input)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(authorID) == "" {
		return nil, invalid("an author is required")
	}
	return s.repository.Create(ctx, authorID, *prepared)
}

// Mutation authorization belongs in the repository transaction, not a
// separate read susceptible to concurrent revocation.
func (s *Service) Update(ctx context.Context, id, viewerID string, admin bool, input UpsertInput) (*Set, error) {
	prepared, err := prepare(input)
	if err != nil {
		return nil, err
	}
	return s.repository.Update(ctx, id, viewerID, admin, *prepared)
}

func (s *Service) Delete(ctx context.Context, id, viewerID string, admin bool) error {
	return s.repository.Delete(ctx, id, viewerID)
}

// SetItems replaces the curated list. Order is the order given.
func (s *Service) SetItems(ctx context.Context, id, viewerID string, admin bool, items []ItemInput) error {
	if len(items) > maxItems {
		return invalid("a problem set may contain at most 500 problems")
	}
	prepared := make([]ItemInput, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, entry := range items {
		entry.ProblemID = strings.TrimSpace(entry.ProblemID)
		if entry.ProblemID == "" {
			return invalid("problem ID is required")
		}
		if _, duplicate := seen[entry.ProblemID]; duplicate {
			return invalid("a problem may only appear once in a set")
		}
		seen[entry.ProblemID] = struct{}{}
		entry.Note = strings.TrimSpace(entry.Note)
		if len(entry.Note) > maxNoteLength {
			return invalid("a problem note must be at most 500 characters")
		}
		prepared = append(prepared, entry)
	}
	return s.repository.SetItems(ctx, id, viewerID, admin, prepared)
}

func (s *Service) Grants(ctx context.Context, id string) ([]AccessGrant, error) {
	return s.repository.Grants(ctx, id)
}

func (s *Service) SetGrant(ctx context.Context, id string, input GrantInput) error {
	input.Username, input.Group = strings.TrimSpace(input.Username), strings.TrimSpace(input.Group)
	if (input.Username == "") == (input.Group == "") {
		return invalid("select exactly one user or group")
	}
	if input.Role != AccessReader && input.Role != AccessEditor {
		return invalid("role must be reader or editor")
	}
	return s.repository.SetGrant(ctx, id, input)
}

func (s *Service) RemoveGrant(ctx context.Context, id string, grantID int64) error {
	if grantID <= 0 {
		return invalid("invalid grant ID")
	}
	return s.repository.RemoveGrant(ctx, id, grantID)
}

func (s *Service) Transfer(ctx context.Context, id, username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return invalid("target username is required")
	}
	return s.repository.Transfer(ctx, id, username)
}

func prepare(input UpsertInput) (*UpsertInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	if input.Title == "" {
		return nil, invalid("title is required")
	}
	if len(input.Title) > maxTitleLength {
		return nil, invalid("title must be at most 120 characters")
	}
	if len(input.Description) > maxDescriptionLength {
		return nil, invalid("description is too long")
	}
	if input.Visibility == "" {
		input.Visibility = VisibilityPublic
	}
	if input.Visibility != VisibilityPublic && input.Visibility != VisibilityPrivate {
		return nil, invalid("visibility must be public or private")
	}
	return &input, nil
}
