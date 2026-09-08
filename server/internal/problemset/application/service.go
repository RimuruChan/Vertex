package application

import (
	"context"
	"strings"

	setdomain "github.com/RimuruChan/Vertex/server/internal/problemset/domain"
)

// Service holds the curation rules: what a valid set looks like and who may
// read or change one.
type Service struct{ repository setdomain.Repository }

func NewService(repository setdomain.Repository) *Service { return &Service{repository: repository} }

// List returns sets the viewer may see. Private sets of other users are
// filtered before pagination, so totals and pages use the same visibility rules.
func (s *Service) List(ctx context.Context, filters setdomain.Filters) ([]setdomain.Set, int, error) {
	if filters.Limit <= 0 || filters.Limit > 100 {
		filters.Limit = 20
	}
	filters.Keyword = strings.TrimSpace(filters.Keyword)
	return s.repository.List(ctx, filters)
}

// Get returns one set with its items and the viewer's progress.
func (s *Service) Get(ctx context.Context, id string, viewerID string) (*setdomain.Set, error) {
	return s.repository.Get(ctx, id, viewerID)
}

func (s *Service) Create(ctx context.Context, authorID string, input setdomain.UpsertInput) (*setdomain.Set, error) {
	prepared, err := setdomain.Prepare(input)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(authorID) == "" {
		return nil, setdomain.Invalid("an author is required")
	}
	return s.repository.Create(ctx, authorID, *prepared)
}

// Mutation authorization belongs in the repository transaction, not a
// separate read susceptible to concurrent revocation.
func (s *Service) Update(ctx context.Context, id string, viewerID string, input setdomain.UpsertInput) (*setdomain.Set, error) {
	prepared, err := setdomain.Prepare(input)
	if err != nil {
		return nil, err
	}
	return s.repository.Update(ctx, id, viewerID, *prepared)
}

func (s *Service) Delete(ctx context.Context, id string, viewerID string) error {
	return s.repository.Delete(ctx, id, viewerID)
}

// SetItems replaces the curated list. Order is the order given.
func (s *Service) SetItems(ctx context.Context, id string, viewerID string, items []setdomain.ItemInput) error {
	if len(items) > setdomain.MaxItems {
		return setdomain.Invalid("a problem set may contain at most 500 problems")
	}
	prepared := make([]setdomain.ItemInput, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, entry := range items {
		entry.ProblemID = strings.TrimSpace(entry.ProblemID)
		if entry.ProblemID == "" {
			return setdomain.Invalid("problem ID is required")
		}
		if _, duplicate := seen[entry.ProblemID]; duplicate {
			return setdomain.Invalid("a problem may only appear once in a set")
		}
		seen[entry.ProblemID] = struct{}{}
		entry.Note = strings.TrimSpace(entry.Note)
		if len(entry.Note) > setdomain.MaxNoteLength {
			return setdomain.Invalid("a problem note must be at most 500 characters")
		}
		prepared = append(prepared, entry)
	}
	return s.repository.SetItems(ctx, id, viewerID, prepared)
}

func (s *Service) Grants(ctx context.Context, id string) ([]setdomain.AccessGrant, error) {
	return s.repository.Grants(ctx, id)
}

func (s *Service) SetGrant(ctx context.Context, id string, input setdomain.GrantInput) error {
	input.Username, input.Group = strings.TrimSpace(input.Username), strings.TrimSpace(input.Group)
	if (input.Username == "") == (input.Group == "") {
		return setdomain.Invalid("select exactly one user or group")
	}
	if input.Role != setdomain.AccessReader && input.Role != setdomain.AccessEditor {
		return setdomain.Invalid("role must be reader or editor")
	}
	return s.repository.SetGrant(ctx, id, input)
}

func (s *Service) RemoveGrant(ctx context.Context, id string, grantID int64) error {
	if grantID <= 0 {
		return setdomain.Invalid("invalid grant ID")
	}
	return s.repository.RemoveGrant(ctx, id, grantID)
}

func (s *Service) Transfer(ctx context.Context, id, username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return setdomain.Invalid("target username is required")
	}
	return s.repository.Transfer(ctx, id, username)
}
