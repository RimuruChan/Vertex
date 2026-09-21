package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres/internal/dbgen"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
)

func (q *Queries) PublishedFiles(ctx context.Context, id string, version int) ([]domain.PublishedFile, error) {
	rows, err := q.queries.ListPublishedFiles(ctx, dbgen.ListPublishedFilesParams{DomainID: tenancy.ID(ctx), ProblemID: id, VersionNo: version})
	if err != nil {
		return nil, accessError(err)
	}
	result := make([]domain.PublishedFile, 0, len(rows))
	for _, row := range rows {
		result = append(result, publishedFile(dbgen.GetPublishedFileRow(row)))
	}
	return result, nil
}
func (q *Queries) PublishedFile(ctx context.Context, id string, version int, fileID string) (*domain.PublishedFile, error) {
	row, err := q.queries.GetPublishedFile(ctx, dbgen.GetPublishedFileParams{DomainID: tenancy.ID(ctx), ProblemID: id, VersionNo: version, FileID: fileID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, accessError(err)
	}
	result := publishedFile(row)
	return &result, nil
}
func publishedFile(row dbgen.GetPublishedFileRow) domain.PublishedFile {
	return domain.PublishedFile{ID: row.FileID, Path: row.Path, Name: row.Filename, MediaType: row.MediaType, Purpose: row.Purpose, Digest: row.BlobSha256, Size: row.ByteSize, SampleIndex: row.SampleIndex, Preview: row.Preview, Truncated: row.Truncated, Binary: row.IsBinary, Embedded: row.Embedded}
}
