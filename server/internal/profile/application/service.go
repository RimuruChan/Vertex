package application

import (
	"context"
	"strings"

	profiledomain "github.com/RimuruChan/Vertex/server/internal/profile/domain"
)

type Service struct{ repository profiledomain.Queries }

func NewService(repository profiledomain.Queries) *Service { return &Service{repository: repository} }

// ByUsername 返回个人主页数据。用户名为空时不查库,直接当作不存在。
func (s *Service) ByUsername(ctx context.Context, username string) (*profiledomain.Profile, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, profiledomain.ErrNotFound
	}
	return s.repository.ByUsername(ctx, username)
}
