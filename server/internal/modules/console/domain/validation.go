package domain

import "strings"

func PrepareAnnouncement(input AnnouncementInput) (*AnnouncementInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.ContentMD = strings.TrimSpace(input.ContentMD)
	if input.Title == "" {
		return nil, Invalid("a title is required")
	}
	if len(input.Title) > 200 {
		return nil, Invalid("title must be at most 200 characters")
	}
	if len(input.ContentMD) > 100_000 {
		return nil, Invalid("content is too long")
	}
	return &input, nil
}
