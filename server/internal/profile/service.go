package profile

import (
	"context"
	"strings"
)

type Repository interface {
	ByUsername(ctx context.Context, username string) (*Profile, error)
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }

// ByUsername 返回个人主页数据。用户名为空时不查库,直接当作不存在。
func (s *Service) ByUsername(ctx context.Context, username string) (*Profile, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, ErrNotFound
	}
	return s.repository.ByUsername(ctx, username)
}
