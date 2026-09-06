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
	Delete(ctx context.Context, id string) error
	SetItems(ctx context.Context, id, viewerID string, admin bool, items []ItemInput) error
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
	item, err := s.repository.Get(ctx, id, viewerID, admin)
	if err != nil {
		return nil, err
	}
	if !item.CanView(viewerID, admin) {
		// A private set must be indistinguishable from a missing one.
		return nil, ErrNotFound
	}
	if !admin {
		// A set may retain a reference after its target becomes private. Only
		// the problem's current owner may keep seeing that unpublished metadata.
		visible := make([]Item, 0, len(item.Items))
		for _, entry := range item.Items {
			if entry.Visibility == "public" || entry.OwnerID != nil && *entry.OwnerID == viewerID {
				visible = append(visible, entry)
			}
		}
		item.Items = visible
		item.ProblemCount = len(visible)
	}
	return item, nil
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

// Update replaces the editable fields after checking that the caller owns the
// set.
func (s *Service) Update(ctx context.Context, id, viewerID string, admin bool, input UpsertInput) (*Set, error) {
	current, err := s.authorize(ctx, id, viewerID, admin)
	if err != nil {
		return nil, err
	}
	prepared, err := prepare(input)
	if err != nil {
		return nil, err
	}
	_ = current
	return s.repository.Update(ctx, id, viewerID, admin, *prepared)
}

func (s *Service) Delete(ctx context.Context, id, viewerID string, admin bool) error {
	if _, err := s.authorize(ctx, id, viewerID, admin); err != nil {
		return err
	}
	return s.repository.Delete(ctx, id)
}

// SetItems replaces the curated list. Order is the order given.
func (s *Service) SetItems(ctx context.Context, id, viewerID string, admin bool, items []ItemInput) error {
	if _, err := s.authorize(ctx, id, viewerID, admin); err != nil {
		return err
	}
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

// authorize loads the set and rejects callers who may not modify it. A caller
// who cannot even see the set gets "not found" rather than "forbidden".
func (s *Service) authorize(ctx context.Context, id, viewerID string, admin bool) (*Set, error) {
	item, err := s.repository.Get(ctx, id, viewerID, admin)
	if err != nil {
		return nil, err
	}
	if !item.CanView(viewerID, admin) {
		return nil, ErrNotFound
	}
	if !item.CanEdit(viewerID, admin) {
		return nil, ErrForbidden
	}
	return item, nil
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
