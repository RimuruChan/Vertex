package domain

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrWorkingCopyConflict = errors.New("working copy changed since it was read")
	ErrMergeRequired       = errors.New("resolve working copy conflicts before committing")
	ErrMergeOutdated       = errors.New("working copy or shared head changed since this merge started")
)

type WorkingCopy struct {
	BaseRevision *int64      `json:"baseRevision,omitempty"`
	HeadRevision *int64      `json:"headRevision,omitempty"`
	ETag         string      `json:"etag"`
	Tree         ContentTree `json:"tree"`
	UpdatedAt    time.Time   `json:"updatedAt"`
	MergeID      string      `json:"mergeId,omitempty"`
}

type ContentCommit struct {
	Revision       int64     `json:"revision"`
	ParentRevision *int64    `json:"parentRevision,omitempty"`
	TreeHash       string    `json:"treeHash"`
	AuthorID       string    `json:"authorId"`
	Message        string    `json:"message"`
	CreatedAt      time.Time `json:"createdAt"`
}

type MergeSession struct {
	ID             string       `json:"id"`
	ETag           string       `json:"etag"`
	CopyETag       string       `json:"copyEtag"`
	BaseRevision   *int64       `json:"baseRevision,omitempty"`
	RemoteRevision *int64       `json:"remoteRevision,omitempty"`
	Result         ContentMerge `json:"result"`
}

type CommitOutcome struct {
	Commit *ContentCommit `json:"commit,omitempty"`
	Copy   WorkingCopy    `json:"copy"`
	Merge  *MergeSession  `json:"merge,omitempty"`
}

type ConflictKey struct {
	EntryID string `json:"entryId"`
	Field   string `json:"field"`
}

type ContentComparison struct {
	Review       []ReviewItem    `json:"review"`
	ETag         string          `json:"etag,omitempty"`
	FromRevision *int64          `json:"fromRevision,omitempty"`
	ToRevision   *int64          `json:"toRevision,omitempty"`
	Changes      []ContentChange `json:"changes"`
}

type CommitInput struct {
	ETag      string `json:"etag"`
	RequestID string `json:"requestId"`
	Message   string `json:"message"`
}

func (input CommitInput) Validate() error {
	if input.ETag == "" || !contentIDPattern.MatchString(input.RequestID) {
		return InvalidInput("working copy token and a valid request ID are required")
	}
	if strings.TrimSpace(input.Message) == "" || len(input.Message) > 2000 || !utf8.ValidString(input.Message) {
		return InvalidInput("commit message must contain 1-2000 UTF-8 bytes")
	}
	return nil
}

// RevisionRepository owns private drafts and shared immutable history. Every
// method derives the actor from the authenticated context, never request data.
type RevisionRepository interface {
	SetVisibility(context.Context, string, VisibilityChange) (*VisibilityState, error)
	CopyRelease(context.Context, CopyInput) (*CopyResult, error)
	Origin(context.Context, string) (*CopyOrigin, error)
	Library(context.Context, LibraryQuery) (*LibraryPage, error)
	Materials(context.Context, string, MaterialQuery) (*MaterialPage, error)
	Open(context.Context, string) (*WorkingCopy, error)
	WorkingCopy(context.Context, string) (*WorkingCopy, error)
	Save(context.Context, string, string, ContentTree) (*WorkingCopy, error)
	Commit(context.Context, string, CommitInput) (*CommitOutcome, error)
	History(context.Context, string, int64, int) ([]ContentCommit, error)
	Revision(context.Context, string, int64) (*ContentCommit, ContentTree, error)
	Changes(context.Context, string, int64, int64) (*ContentComparison, error)
	Inspect(context.Context, string, int64, string) (*MaterialInspection, error)
	StartCheck(context.Context, string, CheckSelection) (*CheckRun, error)
	Check(context.Context, string, string) (*CheckRun, error)
	CheckStatement(context.Context, string, string, string) ([]byte, error)
	CancelCheck(context.Context, string, string) (*CheckRun, error)
	Checks(context.Context, string, int) ([]CheckRun, error)
	CheckContent(context.Context, string, string, string, string) (BlobRef, io.ReadCloser, error)
	PublishCommit(context.Context, string, CommitPublication) (*CommitRelease, error)
	CommitReleases(context.Context, string) ([]CommitRelease, error)
	PreviewImport(context.Context, string, string, []byte, ImportOptions) (*ImportReceipt, error)
	Import(context.Context, string, string) (*ImportReceipt, error)
	ApplyImport(context.Context, string, string, string) (*WorkingCopy, error)
	ExportPackage(context.Context, string, int64, string) (*PackageExport, error)
	Update(context.Context, string, string) (*CommitOutcome, error)
	Reset(context.Context, string, string, int64) (*WorkingCopy, error)
	Merge(context.Context, string, string) (*MergeSession, error)
	SaveMerge(context.Context, string, string, string, ContentTree, []ConflictKey) (*MergeSession, error)
	CompleteMerge(context.Context, string, string, string) (*WorkingCopy, error)
	Upload(context.Context, string, io.Reader) (BlobRef, error)
	AuthorizeEdit(context.Context, string) error
	Blob(context.Context, string, string) (BlobRef, io.ReadCloser, error)
}

// ContentStore only addresses blobs under a problem namespace. Authorization
// belongs to the application/repository before a read or upload is permitted.
type ContentStore interface {
	Put(context.Context, string, io.Reader) (BlobRef, error)
	Open(context.Context, string, BlobRef) (io.ReadCloser, error)
}

type ArtifactReader interface {
	Read(context.Context, string, string, string, int64) ([]byte, error)
}

// CheckedArtifactCopier carries verified execution data across problem scopes,
// replacing only non-judging snapshot metadata with the selected release tree.
type CheckedArtifactCopier interface {
	CloneChecked(context.Context, string, string, PackageUpload, CheckSnapshot) (*PackageUpload, error)
}
