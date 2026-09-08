package domain

import (
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

func Invalid(message string) error { return &ValidationError{Message: message} }

const (
	MaxEditorialTitleBytes = 200
	MaxEditorialBodyBytes  = 200_000
	MaxPostBodyBytes       = 20_000
)

func PrepareEditorial(input EditorialInput) (*EditorialInput, error) {
	input.ProblemID = strings.TrimSpace(input.ProblemID)
	input.Title = strings.TrimSpace(input.Title)
	input.ContentMD = strings.TrimSpace(input.ContentMD)
	if input.Title == "" {
		return nil, Invalid("a title is required")
	}
	if len(input.Title) > MaxEditorialTitleBytes {
		return nil, Invalid("title must be at most 200 characters")
	}
	if input.ContentMD == "" {
		return nil, Invalid("content is required")
	}
	if len(input.ContentMD) > MaxEditorialBodyBytes {
		return nil, Invalid("content is too long")
	}
	if input.Visibility == "" {
		input.Visibility = VisibilityPublic
	}
	if input.Visibility != VisibilityPublic && input.Visibility != VisibilityPrivate {
		return nil, Invalid("visibility must be public or private")
	}
	if input.Status == "" {
		input.Status = StatusPublished
	}
	if input.Status != StatusDraft && input.Status != StatusPublished {
		return nil, Invalid("status must be draft or published")
	}
	return &input, nil
}

func CheckPost(scopeID, authorID, contentMD string, parentID *int64) error {
	if strings.TrimSpace(scopeID) == "" || strings.TrimSpace(authorID) == "" {
		return Invalid("scope and author are required")
	}
	contentMD = strings.TrimSpace(contentMD)
	if contentMD == "" {
		return Invalid("content is required")
	}
	if len(contentMD) > MaxPostBodyBytes {
		return Invalid("content is too long")
	}
	if parentID != nil && *parentID <= 0 {
		return Invalid("parent ID must be positive")
	}
	return nil
}
