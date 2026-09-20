package application

import (
	"context"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

// Workbench exposes the working-copy use cases. Persistence owns transaction
// ordering; content comparison and merge rules live in the authoring domain.
type Workbench struct{ revisions domain.RevisionRepository }

func NewWorkbench(revisions domain.RevisionRepository) *Workbench {
	return &Workbench{revisions: revisions}
}
func (service *Workbench) SetVisibility(ctx context.Context, id string, input domain.VisibilityChange) (*domain.VisibilityState, error) {
	return service.revisions.SetVisibility(ctx, id, input)
}
func (service *Workbench) CopyRelease(ctx context.Context, input domain.CopyInput) (*domain.CopyResult, error) {
	return service.revisions.CopyRelease(ctx, input)
}
func (service *Workbench) Origin(ctx context.Context, id string) (*domain.CopyOrigin, error) {
	return service.revisions.Origin(ctx, id)
}
func (service *Workbench) Library(ctx context.Context, input domain.LibraryQuery) (*domain.LibraryPage, error) {
	return service.revisions.Library(ctx, input)
}

func (service *Workbench) PreviewImport(ctx context.Context, id, etag string, data []byte, options domain.ImportOptions) (*domain.ImportReceipt, error) {
	return service.revisions.PreviewImport(ctx, id, etag, data, options)
}
func (service *Workbench) Import(ctx context.Context, id, importID string) (*domain.ImportReceipt, error) {
	return service.revisions.Import(ctx, id, importID)
}
func (service *Workbench) ApplyImport(ctx context.Context, id, importID, etag string) (*domain.WorkingCopy, error) {
	return service.revisions.ApplyImport(ctx, id, importID, etag)
}
func (service *Workbench) ExportPackage(ctx context.Context, id string, revision int64, format string) (*domain.PackageExport, error) {
	return service.revisions.ExportPackage(ctx, id, revision, format)
}

func (service *Workbench) Open(ctx context.Context, id string) (*domain.WorkingCopy, error) {
	return service.revisions.Open(ctx, id)
}

func (service *Workbench) PublishCommit(ctx context.Context, id string, input domain.CommitPublication) (*domain.CommitRelease, error) {
	return service.revisions.PublishCommit(ctx, id, input)
}
func (service *Workbench) CommitReleases(ctx context.Context, id string) ([]domain.CommitRelease, error) {
	return service.revisions.CommitReleases(ctx, id)
}

func (service *Workbench) StartCheck(ctx context.Context, id string, selection domain.CheckSelection) (*domain.CheckRun, error) {
	return service.revisions.StartCheck(ctx, id, selection)
}
func (service *Workbench) Check(ctx context.Context, id, checkID string) (*domain.CheckRun, error) {
	return service.revisions.Check(ctx, id, checkID)
}
func (service *Workbench) CheckStatement(ctx context.Context, id, checkID, statementID string) ([]byte, error) {
	return service.revisions.CheckStatement(ctx, id, checkID, statementID)
}
func (service *Workbench) Checks(ctx context.Context, id string, limit int) ([]domain.CheckRun, error) {
	return service.revisions.Checks(ctx, id, limit)
}
func (service *Workbench) CancelCheck(ctx context.Context, id, checkID string) (*domain.CheckRun, error) {
	return service.revisions.CancelCheck(ctx, id, checkID)
}
func (service *Workbench) CheckContent(ctx context.Context, id, worker, lease, digest string) (domain.BlobRef, io.ReadCloser, error) {
	return service.revisions.CheckContent(ctx, id, worker, lease, digest)
}
func (service *Workbench) Read(ctx context.Context, id string) (*domain.WorkingCopy, error) {
	return service.revisions.WorkingCopy(ctx, id)
}
func (service *Workbench) Save(ctx context.Context, id, etag string, tree domain.ContentTree) (*domain.WorkingCopy, error) {
	if etag == "" {
		return nil, domain.InvalidInput("working copy token is required")
	}
	canonical, err := tree.Canonical()
	if err != nil {
		return nil, err
	}
	return service.revisions.Save(ctx, id, etag, canonical)
}
func (service *Workbench) Commit(ctx context.Context, id string, input domain.CommitInput) (*domain.CommitOutcome, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	return service.revisions.Commit(ctx, id, input)
}
func (service *Workbench) History(ctx context.Context, id string, before int64, limit int) ([]domain.ContentCommit, error) {
	return service.revisions.History(ctx, id, before, limit)
}
func (service *Workbench) Revision(ctx context.Context, id string, revision int64) (*domain.ContentCommit, domain.ContentTree, error) {
	return service.revisions.Revision(ctx, id, revision)
}
func (service *Workbench) Update(ctx context.Context, id, etag string) (*domain.CommitOutcome, error) {
	return service.revisions.Update(ctx, id, etag)
}
func (service *Workbench) Reset(ctx context.Context, id, etag string, revision int64) (*domain.WorkingCopy, error) {
	return service.revisions.Reset(ctx, id, etag, revision)
}
func (service *Workbench) Merge(ctx context.Context, id, mergeID string) (*domain.MergeSession, error) {
	return service.revisions.Merge(ctx, id, mergeID)
}
func (service *Workbench) SaveMerge(ctx context.Context, id, mergeID, etag string, tree domain.ContentTree, resolved []domain.ConflictKey) (*domain.MergeSession, error) {
	return service.revisions.SaveMerge(ctx, id, mergeID, etag, tree, resolved)
}
func (service *Workbench) CompleteMerge(ctx context.Context, id, mergeID, etag string) (*domain.WorkingCopy, error) {
	return service.revisions.CompleteMerge(ctx, id, mergeID, etag)
}
func (service *Workbench) Upload(ctx context.Context, id string, body io.Reader) (domain.BlobRef, error) {
	return service.revisions.Upload(ctx, id, body)
}

// AuthorizeEdit rejects an unauthorized upload before HTTP reads/stages its body.
// Upload/import persistence repeats the check under its own transaction locks.
func (service *Workbench) AuthorizeEdit(ctx context.Context, id string) error {
	return service.revisions.AuthorizeEdit(ctx, id)
}
func (service *Workbench) Blob(ctx context.Context, id, digest string) (domain.BlobRef, io.ReadCloser, error) {
	return service.revisions.Blob(ctx, id, digest)
}

// SaveText is a single editor action: upload the immutable new text, then replace
// its entry using the original read token. A stale browser cannot silently apply
// the upload to another version of the user's working copy.
func (service *Workbench) SaveText(ctx context.Context, id, etag, entryID, text string) (*domain.WorkingCopy, error) {
	if !utf8.ValidString(text) || len(text) > 1<<20 || strings.ContainsRune(text, 0) {
		return nil, domain.InvalidInput("editor text must be valid UTF-8 and at most 1 MiB")
	}
	copy, err := service.revisions.WorkingCopy(ctx, id)
	if err != nil {
		return nil, err
	}
	if copy.ETag != etag {
		return nil, domain.ErrWorkingCopyConflict
	}
	if copy.MergeID != "" {
		return nil, domain.ErrMergeRequired
	}
	for i, entry := range copy.Tree.Entries {
		if entry.ID != entryID {
			continue
		}
		switch entry.Kind {
		case domain.EntryInput, domain.EntryAnswer, domain.EntryAsset, domain.EntryResource:
			return nil, domain.InvalidInput("use file upload to replace binary material or test data")
		}
		if entry.Attributes["format"] == "pdf" {
			return nil, domain.InvalidInput("PDF statements cannot be edited as text")
		}
		text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
		if entry.Kind == domain.EntryMetadata || entry.Kind == domain.EntryProgram || entry.Kind == domain.EntryTest || entry.Kind == domain.EntryGroup || entry.Kind == domain.EntryValidation {
			data, err := domain.NormalizeMaterial(entry.Kind, []byte(text))
			if err != nil {
				return nil, err
			}
			text = string(data)
		}
		ref, err := service.revisions.Upload(ctx, id, strings.NewReader(text))
		if err != nil {
			return nil, err
		}
		copy.Tree.Entries[i].Blob = ref
		return service.revisions.Save(ctx, id, etag, copy.Tree)
	}
	return nil, domain.ErrNotFound
}
