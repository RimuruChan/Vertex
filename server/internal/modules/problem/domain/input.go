package domain

import (
	"errors"
	"strings"
)

var (
	ErrInvalidInput = errors.New("invalid problem input")
	ErrNotFound     = errors.New("problem not found")
	ErrReferenced   = errors.New("problem is referenced by contests or submissions; hide it instead of deleting")
)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
func (e *ValidationError) Unwrap() error { return ErrInvalidInput }

type Filters struct {
	Workspace  bool
	Available  bool // published problems the viewer may reference, including private grants
	Visibility string
	Tag        string
	Difficulty int
	Keyword    string
	// ViewerID 是当前登录用户,用于个人进度标注与 Status 过滤;匿名请求留空。
	ViewerID string
	// Status 取 UserStatusSolved / UserStatusAttempted / UserStatusNone,空表示不过滤。
	Status string
	Limit  int
	Offset int
}

type CreateInput struct {
	Title         string
	StatementMD   string
	Difficulty    int
	Source        string
	TimeLimitMs   int
	MemoryLimitKb int
	Visibility    string
	Tags          []string
}

type UpdateInput struct{ CreateInput }

func IsUserStatus(value string) bool {
	switch value {
	case UserStatusSolved, UserStatusAttempted, UserStatusNone:
		return true
	default:
		return false
	}
}

func PrepareInput(input CreateInput) (CreateInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		return CreateInput{}, &ValidationError{Message: "title is required"}
	}
	if input.Difficulty == 0 {
		input.Difficulty = 1
	}
	if input.Difficulty < 1 || input.Difficulty > 10 {
		return CreateInput{}, &ValidationError{Message: "difficulty must be between 1 and 10"}
	}
	if input.TimeLimitMs == 0 {
		input.TimeLimitMs = 1000
	}
	if input.TimeLimitMs < 1 {
		return CreateInput{}, &ValidationError{Message: "time limit must be positive"}
	}
	if input.MemoryLimitKb == 0 {
		input.MemoryLimitKb = 262144
	}
	if input.MemoryLimitKb < 1 {
		return CreateInput{}, &ValidationError{Message: "memory limit must be positive"}
	}
	if input.Visibility == "" {
		input.Visibility = "draft"
	}
	switch input.Visibility {
	case "draft", "private", "public":
	default:
		return CreateInput{}, &ValidationError{Message: "unsupported visibility"}
	}
	seen := make(map[string]struct{}, len(input.Tags))
	tags := make([]string, 0, len(input.Tags))
	for _, tag := range input.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, duplicate := seen[tag]; duplicate {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	input.Tags = tags
	return input, nil
}
