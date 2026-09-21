package dto

import "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"

type WorkingCopySaveRequest struct {
	ETag string             `json:"etag" binding:"required"`
	Tree domain.ContentTree `json:"tree"`
}
type WorkingCopyTokenRequest struct {
	ETag string `json:"etag" binding:"required"`
}
type WorkingCopyRestoreRequest struct {
	ETag     string `json:"etag" binding:"required"`
	Revision int64  `json:"revision" binding:"min=1"`
}
type WorkingCopyTextRequest struct {
	ETag string `json:"etag" binding:"required"`
	Text string `json:"text"`
}
type CommitHistoryResponse struct {
	Items []domain.ContentCommit `json:"items"`
}
type CommitDetailResponse struct {
	Commit domain.ContentCommit `json:"commit"`
	Tree   domain.ContentTree   `json:"tree"`
}
type MergeSaveRequest struct {
	ETag     string               `json:"etag" binding:"required"`
	Tree     domain.ContentTree   `json:"tree"`
	Resolved []domain.ConflictKey `json:"resolved"`
}

type MaterialEntryRequest struct {
	ETag  string        `json:"etag" binding:"required"`
	Entry MaterialEntry `json:"entry"`
	Text  *string       `json:"text,omitempty"`
}

// MaterialEntry accepts either inline text or an uploaded blob reference;
// callers creating text should not invent a fake hash just to satisfy a schema.
type MaterialEntry struct {
	ID         string            `json:"id"`
	Path       string            `json:"path"`
	Kind       string            `json:"kind"`
	Attributes map[string]string `json:"attributes"`
	Blob       *domain.BlobRef   `json:"blob,omitempty"`
}

func (entry MaterialEntry) Domain() domain.TreeEntry {
	value := domain.TreeEntry{ID: entry.ID, Path: entry.Path, Kind: entry.Kind, Attributes: entry.Attributes}
	if entry.Blob != nil {
		value.Blob = *entry.Blob
	}
	return value
}
