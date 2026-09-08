package application

import (
	"context"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/authoring/domain"
)

func (s *Service) Copy(ctx context.Context, input authoringdomain.CopyInput) (*authoringdomain.CopyResult, error) {
	normalized, err := authoringdomain.NormalizeCopy(input)
	if err != nil {
		return nil, err
	}
	return s.packages.Copy(ctx, normalized, s.publisher)
}

func (s *Service) Origin(ctx context.Context, problemID string) (*authoringdomain.CopyOrigin, error) {
	return s.packages.Origin(ctx, problemID)
}
