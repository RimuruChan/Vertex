package problem

import (
	"context"
	"errors"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/domain"
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

type Reader interface {
	Access(ctx context.Context, id, userID string) (Access, error)
	Grants(ctx context.Context, id string) ([]AccessGrant, error)
	List(ctx context.Context, filters Filters) ([]Problem, int, error)
	UserStatuses(ctx context.Context, viewerID string, problemIDs []string) (map[string]string, error)
	Tags(ctx context.Context) ([]Tag, error)
	Get(ctx context.Context, id string) (*Problem, error)
	GetWorkspace(ctx context.Context, id string) (*Problem, error)
}

type Writer interface {
	SetGrant(ctx context.Context, id string, input GrantInput) error
	RemoveGrant(ctx context.Context, id string, grantID int64) error
	Transfer(ctx context.Context, id, username string) error
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

func (s *Service) List(ctx context.Context, filters Filters, workspace bool) ([]Problem, int, error) {
	filters.Workspace = workspace
	if !workspace {
		filters.Visibility = "public"
	}
	if !isUserStatus(filters.Status) {
		filters.Status = ""
	}
	list, total, err := s.reader.List(ctx, filters)
	if err != nil {
		return nil, 0, err
	}
	if err := s.annotateStatus(ctx, filters.ViewerID, list); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (s *Service) Get(ctx context.Context, id string, viewerID string, _ bool) (*Problem, error) {
	access, err := s.reader.Access(ctx, id, viewerID)
	if err != nil {
		return nil, err
	}
	if !access.Permissions.View {
		return nil, ErrNotFound
	}
	item, err := s.reader.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if item.PublishedVersion == 0 && access.Permissions.ReadPackage {
		item, err = s.reader.GetWorkspace(ctx, id)
		if err != nil {
			return nil, err
		}
	}
	item.OwnerID, item.DomainID, item.Permissions = access.OwnerID, access.Scope.Domain.ID, access.Permissions
	single := []Problem{*item}
	if err := s.annotateStatus(ctx, viewerID, single); err != nil {
		return nil, err
	}
	return &single[0], nil
}

func (s *Service) Grants(ctx context.Context, id string) ([]AccessGrant, error) {
	return s.reader.Grants(ctx, id)
}

func (s *Service) GetWorkspace(ctx context.Context, id, userID string) (*Problem, error) {
	access, err := s.reader.Access(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	if !access.Permissions.ReadPackage {
		if !access.Permissions.View {
			return nil, ErrNotFound
		}
		return nil, domain.ErrForbidden
	}
	item, err := s.reader.GetWorkspace(ctx, id)
	if err != nil {
		return nil, err
	}
	item.OwnerID, item.DomainID, item.Permissions = access.OwnerID, access.Scope.Domain.ID, access.Permissions
	return item, nil
}

func (s *Service) Access(ctx context.Context, id, userID string) (Access, error) {
	return s.reader.Access(ctx, id, userID)
}

func (s *Service) SetGrant(ctx context.Context, id string, input GrantInput) error {
	input.Username, input.Group = strings.TrimSpace(input.Username), strings.TrimSpace(input.Group)
	if (input.Username == "") == (input.Group == "") {
		return &ValidationError{Message: "select exactly one domain member or group"}
	}
	if input.Role != AccessReader && input.Role != AccessEditor {
		return &ValidationError{Message: "collaborator role must be reader or editor"}
	}
	return s.writer.SetGrant(ctx, id, input)
}

func (s *Service) RemoveGrant(ctx context.Context, id string, grantID int64) error {
	if grantID <= 0 {
		return &ValidationError{Message: "grant ID must be positive"}
	}
	return s.writer.RemoveGrant(ctx, id, grantID)
}

func (s *Service) Transfer(ctx context.Context, id, username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return &ValidationError{Message: "new owner is required"}
	}
	if domain.ActorID(ctx) == "" {
		return domain.ErrUnauthenticated
	}
	return s.writer.Transfer(ctx, id, username)
}

// Tags 返回公开题库的标签目录,供题库筛选器使用。
func (s *Service) Tags(ctx context.Context) ([]Tag, error) {
	return s.reader.Tags(ctx)
}

// annotateStatus 就地填入每道题的查看者进度;匿名查看者一律 none。
func (s *Service) annotateStatus(ctx context.Context, viewerID string, list []Problem) error {
	for i := range list {
		list[i].UserStatus = UserStatusNone
	}
	if viewerID == "" || len(list) == 0 {
		return nil
	}
	ids := make([]string, 0, len(list))
	for _, item := range list {
		ids = append(ids, item.ID)
	}
	statuses, err := s.reader.UserStatuses(ctx, viewerID, ids)
	if err != nil {
		return err
	}
	for i := range list {
		if status, ok := statuses[list[i].ID]; ok {
			list[i].UserStatus = status
		}
	}
	return nil
}

func isUserStatus(value string) bool {
	switch value {
	case UserStatusSolved, UserStatusAttempted, UserStatusNone:
		return true
	default:
		return false
	}
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
