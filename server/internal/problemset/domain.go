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

// Visibility controls catalogue reads; collaboration never grants access to
// the problems referenced by a set.
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
	PublicID    string
	ID          string
	Title       string
	Description string
	AuthorID    *string
	AuthorName  string
	DomainID    string
	OwnerID     string
	OwnerName   string
	Permissions Permissions
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
	ProblemPublicID string
	ProblemID       string
	OwnerID         *string
	SortOrder       int
	Note            string
	Title           string
	Difficulty      int
	Visibility      string
	Tags            []string
	SubmitCount     int
	AcceptCount     int
	// UserStatus mirrors the problem list: none / attempted / solved.
	UserStatus string
}

// Filters narrows a set listing.
type Filters struct {
	// AuthorID is immutable creation attribution, not current ownership.
	AuthorID string
	Keyword  string
	ViewerID string
	Admin    bool // Legacy caller hint; the store ignores it and reloads domain rights.
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
