// Package problemset owns 题单: curated, ordered lists of problems with a
// per-viewer progress read model.
//
// A set is a curation artifact, not a container: removing a problem from a set
// never touches the problem or the submissions against it, and a set can point
// at a problem that another set also uses.
package problemset

import (
	"errors"
	"time"
)

var (
	ErrNotFound     = errors.New("problem set not found")
	ErrInvalidInput = errors.New("invalid problem set input")
	ErrForbidden    = errors.New("problem set forbidden")
)

// Visibility values. A private set is readable only by its author and by
// administrators, which is what makes drafting a set in the open possible.
const (
	VisibilityPublic  = "public"
	VisibilityPrivate = "private"
)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
func (e *ValidationError) Unwrap() error { return ErrInvalidInput }

func invalid(message string) error { return &ValidationError{Message: message} }

// Set is one curated list.
type Set struct {
	ID          string
	Title       string
	Description string
	AuthorID    *string
	AuthorName  string
	Visibility  string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	// ProblemCount and SolvedCount are read-model values: the number of
	// problems in the set, and how many of them the current viewer solved.
	ProblemCount int
	SolvedCount  int
	Items        []Item
}

// Item is one problem inside a set, in curator order.
type Item struct {
	ProblemID   string
	AuthorID    *string
	SortOrder   int
	Note        string
	Title       string
	Difficulty  int
	Visibility  string
	Tags        []string
	SubmitCount int
	AcceptCount int
	// UserStatus mirrors the problem list: none / attempted / solved.
	UserStatus string
}

// Filters narrows a set listing.
type Filters struct {
	// AuthorID restricts to one curator; ViewerID and Admin decide which
	// private sets are visible at all.
	AuthorID string
	Keyword  string
	ViewerID string
	Admin    bool
	Limit    int
	Offset   int
}

// UpsertInput is the editable part of a set.
type UpsertInput struct {
	Title       string
	Description string
	Visibility  string
}

// ItemInput is one curated entry as submitted by the curator.
type ItemInput struct {
	ProblemID string
	Note      string
}

// CanEdit reports whether a caller may modify this set. Administrators may
// always edit; otherwise only the author can.
func (s *Set) CanEdit(userID string, admin bool) bool {
	if admin {
		return true
	}
	return s.AuthorID != nil && *s.AuthorID == userID && userID != ""
}

// CanView reports whether a caller may read this set.
func (s *Set) CanView(userID string, admin bool) bool {
	if s.Visibility == VisibilityPublic {
		return true
	}
	return s.CanEdit(userID, admin)
}
