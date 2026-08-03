package problem

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrInvalidInput = errors.New("invalid problem input")
	ErrNotFound     = errors.New("problem not found")
)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
func (e *ValidationError) Unwrap() error { return ErrInvalidInput }

type Filters struct {
	Visibility string
	Tag        string
	Difficulty int
	Keyword    string
	Limit      int
	Offset     int
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

type Reader interface {
	List(ctx context.Context, filters Filters) ([]Problem, int, error)
	Get(ctx context.Context, id string) (*Problem, error)
}

type Writer interface {
	Create(ctx context.Context, authorID string, input *CreateInput) (*Problem, error)
	Update(ctx context.Context, id string, input *UpdateInput) (*Problem, error)
	Delete(ctx context.Context, id string) error
	SaveTestdata(ctx context.Context, problemID string, zipData []byte, checker string) (int, string, error)
}

type Service struct {
	reader Reader
	writer Writer
}

func NewService(reader Reader, writer Writer) *Service {
	return &Service{reader: reader, writer: writer}
}

func (s *Service) List(ctx context.Context, filters Filters, admin bool) ([]Problem, int, error) {
	if !admin {
		filters.Visibility = "public"
	}
	return s.reader.List(ctx, filters)
}

func (s *Service) Get(ctx context.Context, id string, admin bool) (*Problem, error) {
	item, err := s.reader.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !admin && item.Visibility != "public" {
		return nil, ErrNotFound
	}
	return item, nil
}

func (s *Service) Create(ctx context.Context, authorID string, input CreateInput) (*Problem, error) {
	prepared, err := prepareInput(input)
	if err != nil {
		return nil, err
	}
	return s.writer.Create(ctx, authorID, &prepared)
}

func (s *Service) Update(ctx context.Context, id string, input UpdateInput) (*Problem, error) {
	if strings.TrimSpace(id) == "" {
		return nil, &ValidationError{Message: "problem ID is required"}
	}
	prepared, err := prepareInput(input.CreateInput)
	if err != nil {
		return nil, err
	}
	return s.writer.Update(ctx, id, &UpdateInput{CreateInput: prepared})
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.writer.Delete(ctx, id)
}

func (s *Service) SaveTestdata(ctx context.Context, problemID string, data []byte, checker string) (int, string, error) {
	if len(data) == 0 {
		return 0, "", &ValidationError{Message: "testdata archive is empty"}
	}
	checker = strings.TrimSpace(checker)
	if checker == "" {
		checker = "diff"
	}
	if checker != "diff" && checker != "spj" && checker != "interactive" {
		return 0, "", &ValidationError{Message: "unsupported checker"}
	}
	if _, err := s.reader.Get(ctx, problemID); err != nil {
		return 0, "", err
	}
	return s.writer.SaveTestdata(ctx, problemID, data, checker)
}

func prepareInput(input CreateInput) (CreateInput, error) {
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
