package domain

import "strings"

const (
	maxTitleLength       = 120
	maxDescriptionLength = 20000
	MaxNoteLength        = 500
	MaxItems             = 500
)

func Prepare(input UpsertInput) (*UpsertInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	if input.Title == "" {
		return nil, Invalid("title is required")
	}
	if len(input.Title) > maxTitleLength {
		return nil, Invalid("title must be at most 120 characters")
	}
	if len(input.Description) > maxDescriptionLength {
		return nil, Invalid("description is too long")
	}
	if input.Visibility == "" {
		input.Visibility = VisibilityPublic
	}
	if input.Visibility != VisibilityPublic && input.Visibility != VisibilityPrivate {
		return nil, Invalid("visibility must be public or private")
	}
	return &input, nil
}
