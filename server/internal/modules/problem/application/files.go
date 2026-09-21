package application

import (
	"context"
	"io"

	"github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
)

// MediaService only resolves files approved for an immutable release. HTTP
// callers must first authorize the same practice/contest statement and require
// the requested version to equal that statement's current pinned version.
type MediaService struct {
	queries domain.PublishedFileQueries
	storage domain.PublicBlobStorage
}

func NewMediaService(queries domain.PublishedFileQueries, storage domain.PublicBlobStorage) *MediaService {
	return &MediaService{queries, storage}
}
func (s *MediaService) List(ctx context.Context, id string, version int) ([]domain.PublishedFile, error) {
	if version < 1 {
		return []domain.PublishedFile{}, nil
	}
	return s.queries.PublishedFiles(ctx, id, version)
}
func (s *MediaService) Open(ctx context.Context, id string, version int, fileID string) (*domain.PublishedFile, io.ReadCloser, error) {
	if version < 1 || fileID == "" {
		return nil, nil, domain.ErrNotFound
	}
	file, err := s.queries.PublishedFile(ctx, id, version, fileID)
	if err != nil {
		return nil, nil, err
	}
	stream, err := s.storage.OpenPublished(ctx, id, file.Digest, file.Size)
	return file, stream, err
}
