package domain

import (
	"context"
	"io"
)

// PublishedFile is an immutable, explicitly approved presentation file. Digest
// is storage metadata and never belongs in the public response DTO.
type PublishedFile struct {
	ID, Path, Name, MediaType, Purpose string
	Digest                             string
	Size                               int64
	SampleIndex                        int
	Preview                            string
	Truncated, Binary, Embedded        bool
}
type PublishedFileQueries interface {
	PublishedFiles(context.Context, string, int) ([]PublishedFile, error)
	PublishedFile(context.Context, string, int, string) (*PublishedFile, error)
}
type PublicBlobStorage interface {
	OpenPublished(context.Context, string, string, int64) (io.ReadCloser, error)
}
