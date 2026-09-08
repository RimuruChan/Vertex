package application

import (
	"context"
	"strings"

	problemdomain "github.com/RimuruChan/Vertex/server/internal/problem/domain"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
)

type Service struct {
	reader problemdomain.Queries
	writer problemdomain.Repository
}

func NewService(reader problemdomain.Queries, writer problemdomain.Repository) *Service {
	return &Service{reader: reader, writer: writer}
}

func (s *Service) List(ctx context.Context, filters problemdomain.Filters, workspace bool) ([]problemdomain.Problem, int, error) {
	filters.Workspace = workspace
	if !workspace && !filters.Available {
		filters.Visibility = "public"
	}
	if filters.Available && filters.ViewerID == "" {
		return nil, 0, tenancydomain.ErrUnauthenticated
	}
	if !problemdomain.IsUserStatus(filters.Status) {
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

func (s *Service) Get(ctx context.Context, id string, viewerID string) (*problemdomain.Problem, error) {
	access, err := s.reader.Access(ctx, id, viewerID)
	if err != nil {
		return nil, err
	}
	if !access.Permissions.View {
		return nil, problemdomain.ErrNotFound
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
	single := []problemdomain.Problem{*item}
	if err := s.annotateStatus(ctx, viewerID, single); err != nil {
		return nil, err
	}
	return &single[0], nil
}

func (s *Service) Grants(ctx context.Context, id string) ([]problemdomain.AccessGrant, error) {
	return s.reader.Grants(ctx, id)
}

func (s *Service) GetWorkspace(ctx context.Context, id, userID string) (*problemdomain.Problem, error) {
	access, err := s.reader.Access(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	if !access.Permissions.ReadPackage {
		if !access.Permissions.View {
			return nil, problemdomain.ErrNotFound
		}
		return nil, tenancydomain.ErrForbidden
	}
	item, err := s.reader.GetWorkspace(ctx, id)
	if err != nil {
		return nil, err
	}
	item.OwnerID, item.DomainID, item.Permissions = access.OwnerID, access.Scope.Domain.ID, access.Permissions
	return item, nil
}

func (s *Service) Access(ctx context.Context, id, userID string) (problemdomain.Access, error) {
	return s.reader.Access(ctx, id, userID)
}

func (s *Service) SetGrant(ctx context.Context, id string, input problemdomain.GrantInput) error {
	input.Username, input.Group = strings.TrimSpace(input.Username), strings.TrimSpace(input.Group)
	if (input.Username == "") == (input.Group == "") {
		return &problemdomain.ValidationError{Message: "select exactly one domain member or group"}
	}
	if input.Role != problemdomain.AccessReader && input.Role != problemdomain.AccessEditor {
		return &problemdomain.ValidationError{Message: "collaborator role must be reader or editor"}
	}
	return s.writer.SetGrant(ctx, id, input)
}

func (s *Service) RemoveGrant(ctx context.Context, id string, grantID int64) error {
	if grantID <= 0 {
		return &problemdomain.ValidationError{Message: "grant ID must be positive"}
	}
	return s.writer.RemoveGrant(ctx, id, grantID)
}

func (s *Service) Transfer(ctx context.Context, id, username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return &problemdomain.ValidationError{Message: "new owner is required"}
	}
	if tenancydomain.ActorID(ctx) == "" {
		return tenancydomain.ErrUnauthenticated
	}
	return s.writer.Transfer(ctx, id, username)
}

// Tags 返回公开题库的标签目录,供题库筛选器使用。
func (s *Service) Tags(ctx context.Context) ([]problemdomain.Tag, error) {
	return s.reader.Tags(ctx)
}

// annotateStatus 就地填入每道题的查看者进度;匿名查看者一律 none。
func (s *Service) annotateStatus(ctx context.Context, viewerID string, list []problemdomain.Problem) error {
	for i := range list {
		list[i].UserStatus = problemdomain.UserStatusNone
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

func (s *Service) Create(ctx context.Context, authorID string, input problemdomain.CreateInput) (*problemdomain.Problem, error) {
	prepared, err := problemdomain.PrepareInput(input)
	if err != nil {
		return nil, err
	}
	return s.writer.Create(ctx, authorID, &prepared)
}

func (s *Service) Update(ctx context.Context, id string, input problemdomain.UpdateInput) (*problemdomain.Problem, error) {
	if strings.TrimSpace(id) == "" {
		return nil, &problemdomain.ValidationError{Message: "problem ID is required"}
	}
	prepared, err := problemdomain.PrepareInput(input.CreateInput)
	if err != nil {
		return nil, err
	}
	return s.writer.Update(ctx, id, &problemdomain.UpdateInput{CreateInput: prepared})
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.writer.Delete(ctx, id)
}

func (s *Service) SaveTestdata(ctx context.Context, problemID string, data []byte, checker string) (int, string, error) {
	if len(data) == 0 {
		return 0, "", &problemdomain.ValidationError{Message: "testdata archive is empty"}
	}
	checker = strings.TrimSpace(checker)
	if checker == "" {
		checker = "diff"
	}
	if checker != "diff" && checker != "spj" && checker != "interactive" {
		return 0, "", &problemdomain.ValidationError{Message: "unsupported checker"}
	}
	if _, err := s.reader.Get(ctx, problemID); err != nil {
		return 0, "", err
	}
	return s.writer.SaveTestdata(ctx, problemID, data, checker)
}
