package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres/internal/dbgen"
	problempg "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
)

type RevisionRepository struct {
	db        *database.DB
	blobs     domain.ContentStore
	artifacts domain.ArtifactReader
}

func NewRevisionRepository(db *database.DB, blobs domain.ContentStore, artifacts ...domain.ArtifactReader) *RevisionRepository {
	repo := &RevisionRepository{db: db, blobs: blobs}
	if len(artifacts) > 0 {
		repo.artifacts = artifacts[0]
	}
	return repo
}

var _ domain.RevisionRepository = (*RevisionRepository)(nil)

func (repo *RevisionRepository) AuthorizeEdit(ctx context.Context, id string) error {
	return repo.transactionFor(ctx, id, editWorkbench, func(*dbgen.Queries, string) error { return nil })
}

// All reads hold the existing shared authorization guard; writes take the
// exclusive problem guard before touching head/copy state. Permission revocation
// and commits therefore have an unambiguous transaction ordering.
func (repo *RevisionRepository) transaction(ctx context.Context, problemID string, edit bool, fn func(*dbgen.Queries, string) error) error {
	permission := readWorkbench
	if edit {
		permission = editWorkbench
	}
	return repo.transactionFor(ctx, problemID, permission, fn)
}

type workbenchPermission int

const (
	readWorkbench workbenchPermission = iota
	editWorkbench
	publishWorkbench
	manageWorkbench
)

func (repo *RevisionRepository) transactionFor(ctx context.Context, problemID string, permission workbenchPermission, fn func(*dbgen.Queries, string) error) error {
	tx, err := repo.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := tenancypg.LockContentStorage(ctx, tx, problemID); err != nil {
		return err
	}
	actor := tenancy.ActorID(ctx)
	if actor == "" {
		return tenancy.ErrForbidden
	}
	lock := problempg.LockAuthorization
	if permission != readWorkbench {
		lock = problempg.LockAccess
	}
	access, err := lock(ctx, tx, problemID, actor)
	if err != nil {
		return packageReadError(err)
	}
	if !access.Permissions.ReadPackage || permission == editWorkbench && !access.Permissions.Edit || permission == publishWorkbench && !access.Permissions.Publish || permission == manageWorkbench && !access.Permissions.ManageAccess {
		return tenancy.ErrForbidden
	}
	if err := fn(dbgen.New(tx), actor); err != nil {
		return packageReadError(err)
	}
	return tx.Commit()
}

// Multipart parsing has already staged the upload. One guarded transaction
// covers installation and its fresh temporary grant, including deduplicated files.
func (repo *RevisionRepository) Upload(ctx context.Context, id string, body io.Reader) (domain.BlobRef, error) {
	var ref domain.BlobRef
	err := repo.transaction(ctx, id, true, func(q *dbgen.Queries, actor string) error {
		var err error
		ref, err = repo.blobs.Put(ctx, id, body)
		if err != nil {
			return err
		}
		return registerBlob(ctx, q, id, actor, ref)
	})
	return ref, err
}

func registerBlob(ctx context.Context, q *dbgen.Queries, id, actor string, ref domain.BlobRef) error {
	if err := q.RegisterAuthoringBlob(ctx, dbgen.RegisterAuthoringBlobParams{ProblemID: id, Sha256: ref.SHA256, ByteSize: ref.Bytes}); err != nil {
		return err
	}
	return q.GrantAuthoringBlobUpload(ctx, dbgen.GrantAuthoringBlobUploadParams{ProblemID: id, Sha256: ref.SHA256, ActorID: actor})
}

func (repo *RevisionRepository) Blob(ctx context.Context, id, digest string) (domain.BlobRef, io.ReadCloser, error) {
	var ref domain.BlobRef
	var file io.ReadCloser
	err := repo.transaction(ctx, id, false, func(q *dbgen.Queries, actor string) error {
		row, err := q.ReadAuthoringBlob(ctx, dbgen.ReadAuthoringBlobParams{ProblemID: id, Sha256: digest, ActorID: actor})
		if err == nil {
			ref = domain.BlobRef{SHA256: row.Sha256, Bytes: row.ByteSize}
			file, err = repo.blobs.Open(ctx, id, ref)
		}
		return err
	})
	if err != nil {
		if file != nil {
			file.Close()
		}
		return ref, nil, err
	}
	return ref, file, nil
}

func (repo *RevisionRepository) Open(ctx context.Context, id string) (*domain.WorkingCopy, error) {
	var result *domain.WorkingCopy
	err := repo.transaction(ctx, id, true, func(q *dbgen.Queries, actor string) error {
		_, err := q.ReadAuthoringHead(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			seed, err := q.InitialProblemMaterial(ctx, id)
			if err != nil {
				return err
			}
			materials, err := domain.InitialMaterials(seed.Title, seed.StatementMd, seed.StatementLanguage, seed.Source, seed.JudgeType, seed.TimeLimitMs, seed.MemoryLimitKb, seed.Difficulty)
			if err != nil {
				return err
			}
			tree := domain.ContentTree{Entries: []domain.TreeEntry{}}
			for _, material := range materials {
				ref, err := repo.blobs.Put(ctx, id, strings.NewReader(string(material.Data)))
				if err != nil {
					return err
				}
				if err := registerBlob(ctx, q, id, actor, ref); err != nil {
					return err
				}
				tree.Entries = append(tree.Entries, material.Entry)
			}
			hash, err := repo.persistTree(ctx, q, id, actor, tree)
			if err != nil {
				return err
			}
			if err := q.InitializeAuthoringHead(ctx, dbgen.InitializeAuthoringHeadParams{ProblemID: id, TreeHash: hash}); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if err := q.StartWorkingCopy(ctx, dbgen.StartWorkingCopyParams{ProblemID: id, ActorID: actor}); err != nil {
			return err
		}
		result, _, err = readCopy(ctx, q, id, actor)
		return err
	})
	return result, err
}

func (repo *RevisionRepository) WorkingCopy(ctx context.Context, id string) (*domain.WorkingCopy, error) {
	var result *domain.WorkingCopy
	err := repo.transaction(ctx, id, false, func(q *dbgen.Queries, actor string) error {
		var err error
		result, _, err = readCopy(ctx, q, id, actor)
		return err
	})
	return result, err
}

func (repo *RevisionRepository) Save(ctx context.Context, id, etag string, tree domain.ContentTree) (*domain.WorkingCopy, error) {
	var result *domain.WorkingCopy
	err := repo.transaction(ctx, id, true, func(q *dbgen.Queries, actor string) error {
		copy, row, err := readCopy(ctx, q, id, actor)
		if err != nil {
			return err
		}
		if copy.ETag != etag {
			return domain.ErrWorkingCopyConflict
		}
		if copy.MergeID != "" {
			return domain.ErrMergeRequired
		}
		hash, err := repo.persistTree(ctx, q, id, actor, tree)
		if err != nil {
			return err
		}
		if hash == row.TreeHash {
			result = copy
			return nil
		}
		if err := replaceCopy(ctx, q, id, actor, etag, hash, row.BaseRevision); err != nil {
			return err
		}
		result, _, err = readCopy(ctx, q, id, actor)
		return err
	})
	return result, err
}

func (repo *RevisionRepository) Commit(ctx context.Context, id string, input domain.CommitInput) (*domain.CommitOutcome, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	input.Message = strings.TrimSpace(input.Message)
	var result domain.CommitOutcome
	err := repo.transaction(ctx, id, true, func(q *dbgen.Queries, actor string) error {
		copy, row, err := readCopy(ctx, q, id, actor)
		if err != nil {
			return err
		}
		prior, err := q.FindContentCommitRequest(ctx, dbgen.FindContentCommitRequestParams{ProblemID: id, ActorID: &actor, RequestID: input.RequestID})
		if err == nil {
			if prior.RequestEtag != input.ETag || prior.Message != input.Message {
				return domain.InvalidInput("request ID was already used with different commit input")
			}
			commit := contentCommit(prior)
			result = domain.CommitOutcome{Commit: &commit, Copy: *copy}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if copy.ETag != input.ETag {
			return domain.ErrWorkingCopyConflict
		}
		if copy.MergeID != "" {
			return domain.ErrMergeRequired
		}
		head, err := q.ReadAuthoringHead(ctx, id)
		if err != nil {
			return err
		}
		merged, session, err := repo.mergeHead(ctx, q, id, actor, copy, row, head)
		if err != nil {
			return err
		}
		if session != nil {
			copy.MergeID = session.ID
			result = domain.CommitOutcome{Copy: *copy, Merge: session}
			return nil
		}
		hash, err := repo.persistTree(ctx, q, id, actor, merged)
		if err != nil {
			return err
		}
		if hash == head.TreeHash && head.Revision.Valid {
			if row.BaseRevision != head.Revision || hash != row.TreeHash {
				if err := replaceCopy(ctx, q, id, actor, row.Etag, hash, head.Revision); err != nil {
					return err
				}
			}
			updated, _, err := readCopy(ctx, q, id, actor)
			if err == nil {
				result.Copy = *updated
			}
			return err
		}
		revision := head.Revision.Int64 + 1
		created, err := q.InsertContentCommit(ctx, dbgen.InsertContentCommitParams{
			ProblemID: id, Revision: revision, ParentRevision: head.Revision, TreeHash: hash, AuthorID: &actor,
			Message: input.Message, RequestID: input.RequestID, RequestEtag: input.ETag,
		})
		if err != nil {
			return err
		}
		nextRevision := sql.NullInt64{Int64: revision, Valid: true}
		if err := q.AdvanceAuthoringHead(ctx, dbgen.AdvanceAuthoringHeadParams{ProblemID: id, Revision: nextRevision, TreeHash: hash}); err != nil {
			return err
		}
		if err := replaceCopy(ctx, q, id, actor, row.Etag, hash, nextRevision); err != nil {
			return err
		}
		updated, _, err := readCopy(ctx, q, id, actor)
		if err != nil {
			return err
		}
		commit := contentCommit(created)
		result = domain.CommitOutcome{Commit: &commit, Copy: *updated}
		return nil
	})
	return &result, err
}

func (repo *RevisionRepository) History(ctx context.Context, id string, before int64, limit int) ([]domain.ContentCommit, error) {
	if before < 0 || limit < 1 || limit > 100 {
		return nil, domain.InvalidInput("invalid history pagination")
	}
	result := []domain.ContentCommit{}
	err := repo.transaction(ctx, id, false, func(q *dbgen.Queries, _ string) error {
		rows, err := q.ListContentCommits(ctx, dbgen.ListContentCommitsParams{ProblemID: id, BeforeRevision: before, PageLimit: limit})
		for _, row := range rows {
			result = append(result, contentCommit(row))
		}
		return err
	})
	return result, err
}

func (repo *RevisionRepository) Revision(ctx context.Context, id string, revision int64) (*domain.ContentCommit, domain.ContentTree, error) {
	if revision < 1 {
		return nil, domain.ContentTree{}, domain.InvalidInput("revision must be positive")
	}
	var commit domain.ContentCommit
	var tree domain.ContentTree
	err := repo.transaction(ctx, id, false, func(q *dbgen.Queries, _ string) error {
		row, err := q.ReadContentCommit(ctx, dbgen.ReadContentCommitParams{ProblemID: id, Revision: revision})
		if err != nil {
			return err
		}
		commit = contentCommit(row)
		tree, err = loadTree(ctx, q, id, row.TreeHash)
		return err
	})
	return &commit, tree, err
}

func (repo *RevisionRepository) mergeHead(ctx context.Context, q *dbgen.Queries, id, actor string, copy *domain.WorkingCopy, row dbgen.ReadWorkingCopyRow, head dbgen.ReadAuthoringHeadRow) (domain.ContentTree, *domain.MergeSession, error) {
	if row.BaseRevision == head.Revision {
		return copy.Tree, nil, nil
	}
	initialHash, err := q.ReadInitialAuthoringTree(ctx, id)
	if err != nil {
		return domain.ContentTree{}, nil, err
	}
	base, err := loadTree(ctx, q, id, initialHash)
	if err != nil {
		return domain.ContentTree{}, nil, err
	}
	if row.BaseRevision.Valid {
		commit, err := q.ReadContentCommit(ctx, dbgen.ReadContentCommitParams{ProblemID: id, Revision: row.BaseRevision.Int64})
		if err != nil {
			return domain.ContentTree{}, nil, err
		}
		base, err = loadTree(ctx, q, id, commit.TreeHash)
		if err != nil {
			return domain.ContentTree{}, nil, err
		}
	}
	remote, err := loadTree(ctx, q, id, head.TreeHash)
	if err != nil {
		return domain.ContentTree{}, nil, err
	}
	merged, err := domain.MergeTrees(base, copy.Tree, remote, repo.blobMerger(ctx, q, id, actor))
	if err != nil {
		return domain.ContentTree{}, nil, err
	}
	if len(merged.Conflicts) == 0 {
		return merged.Tree, nil, nil
	}
	data, err := json.Marshal(merged)
	if err != nil {
		return domain.ContentTree{}, nil, err
	}
	session, err := q.SaveMergeSession(ctx, dbgen.SaveMergeSessionParams{ProblemID: id, ActorID: actor, CopyEtag: row.Etag,
		BaseRevision: row.BaseRevision, RemoteRevision: head.Revision, LocalTree: row.TreeHash, RemoteTree: head.TreeHash, ResultJson: data})
	if err != nil {
		return domain.ContentTree{}, nil, err
	}
	return merged.Tree, mergeSession(session, merged), nil
}

func (repo *RevisionRepository) blobMerger(ctx context.Context, q *dbgen.Queries, id, actor string) domain.BlobMerger {
	return func(kind string, base, local, remote domain.BlobRef) (domain.BlobRef, bool, error) {
		var bodies [][]byte
		for _, ref := range []domain.BlobRef{base, local, remote} {
			if ref.Bytes > 1<<20 {
				return local, false, nil
			}
			file, err := repo.blobs.Open(ctx, id, ref)
			if err != nil {
				return local, false, err
			}
			body, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
			_ = file.Close()
			if err != nil {
				return local, false, err
			}
			if err := domain.ValidateBlob(ref, body); err != nil {
				return local, false, err
			}
			bodies = append(bodies, body)
		}
		merge := domain.MergeText
		if kind == domain.EntryMetadata || kind == domain.EntryProgram || kind == domain.EntryTest || kind == domain.EntryGroup || kind == domain.EntryValidation || kind == domain.EntryGeneration {
			merge = domain.MergeJSON
		}
		body, ok := merge(bodies[0], bodies[1], bodies[2])
		if !ok {
			return local, false, nil
		}
		ref, err := repo.blobs.Put(ctx, id, strings.NewReader(string(body)))
		if err != nil {
			return local, false, err
		}
		if err := registerBlob(ctx, q, id, actor, ref); err != nil {
			return local, false, err
		}
		return ref, true, nil
	}
}

func (repo *RevisionRepository) persistTree(ctx context.Context, q *dbgen.Queries, id, actor string, tree domain.ContentTree) (string, error) {
	data, err := tree.Encode()
	if err != nil {
		return "", err
	}
	digests, err := checkTreeBlobs(ctx, q, id, actor, tree)
	if err != nil {
		return "", err
	}
	hash := domain.Digest(data)
	// A derived, immutable list projection. The content blob remains the source
	// of truth; invalid draft documents remain editable and have no projection.
	summary := map[string]any{}
	for _, entry := range tree.Entries {
		if entry.ID != "problem" || entry.Kind != domain.EntryMetadata || entry.Blob.Bytes > 1<<20 {
			continue
		}
		reader, err := repo.blobs.Open(ctx, id, entry.Blob)
		if err != nil {
			return "", err
		}
		body, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
		reader.Close()
		if err != nil {
			return "", err
		}
		if err := domain.ValidateBlob(entry.Blob, body); err != nil {
			return "", err
		}
		view, err := domain.DecodeMaterial(entry, body)
		if err == nil {
			summary = map[string]any{"title": view.Metadata.Title, "source": view.Metadata.Source, "difficulty": view.Metadata.Difficulty}
		}
	}
	projection, err := json.Marshal(summary)
	if err != nil {
		return "", err
	}
	if err := q.InsertAuthoringTree(ctx, dbgen.InsertAuthoringTreeParams{ProblemID: id, TreeHash: hash, Manifest: data, Summary: projection}); err != nil {
		return "", err
	}
	if err := q.InsertAuthoringTreeBlobs(ctx, dbgen.InsertAuthoringTreeBlobsParams{ProblemID: id, TreeHash: hash, Digests: digests}); err != nil {
		return "", err
	}
	return hash, nil
}

func checkTreeBlobs(ctx context.Context, q *dbgen.Queries, id, actor string, tree domain.ContentTree) ([]string, error) {
	digests := make([]string, 0, len(tree.Entries))
	for _, entry := range tree.Entries {
		digests = append(digests, entry.Blob.SHA256)
	}
	rows, err := q.AvailableAuthoringBlobs(ctx, dbgen.AvailableAuthoringBlobsParams{ProblemID: id, ActorID: actor, Digests: digests})
	if err != nil {
		return nil, err
	}
	available := map[string]int64{}
	for _, row := range rows {
		available[row.Sha256] = row.ByteSize
	}
	for _, entry := range tree.Entries {
		size, exists := available[entry.Blob.SHA256]
		if !exists || size != entry.Blob.Bytes {
			return nil, domain.InvalidInput("entry content is missing or inaccessible: " + entry.Path)
		}
	}
	return digests, nil
}

func loadTree(ctx context.Context, q *dbgen.Queries, id, hash string) (domain.ContentTree, error) {
	data, err := q.ReadAuthoringTree(ctx, dbgen.ReadAuthoringTreeParams{ProblemID: id, TreeHash: hash})
	var tree domain.ContentTree
	if err != nil {
		return tree, err
	}
	if err := json.Unmarshal(data, &tree); err != nil {
		return tree, err
	}
	canonical, err := tree.Canonical()
	if err != nil {
		return tree, err
	}
	actual, err := canonical.Hash()
	if err != nil || actual != hash {
		return tree, errors.New("stored content tree does not match its digest")
	}
	return canonical, nil
}

func readCopy(ctx context.Context, q *dbgen.Queries, id, actor string) (*domain.WorkingCopy, dbgen.ReadWorkingCopyRow, error) {
	row, err := q.ReadWorkingCopy(ctx, dbgen.ReadWorkingCopyParams{ProblemID: id, ActorID: actor})
	if err != nil {
		return nil, row, err
	}
	tree, err := loadTree(ctx, q, id, row.TreeHash)
	if err != nil {
		return nil, row, err
	}
	return &domain.WorkingCopy{BaseRevision: revisionPointer(row.BaseRevision), HeadRevision: revisionPointer(row.HeadRevision),
		ETag: row.Etag, Tree: tree, UpdatedAt: row.UpdatedAt, MergeID: row.MergeID}, row, nil
}

func replaceCopy(ctx context.Context, q *dbgen.Queries, id, actor, etag, hash string, base sql.NullInt64) error {
	affected, err := q.ReplaceWorkingCopy(ctx, dbgen.ReplaceWorkingCopyParams{ProblemID: id, ActorID: actor, Etag: etag, TreeHash: hash, BaseRevision: base})
	if err != nil {
		return err
	}
	if affected != 1 {
		return domain.ErrWorkingCopyConflict
	}
	return nil
}

func revisionPointer(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}

func contentCommit(row dbgen.ProblemCommit) domain.ContentCommit {
	actor := ""
	if row.AuthorID != nil {
		actor = *row.AuthorID
	}
	return domain.ContentCommit{Revision: row.Revision, ParentRevision: revisionPointer(row.ParentRevision), TreeHash: row.TreeHash, AuthorID: actor, Message: row.Message, CreatedAt: row.CreatedAt}
}

func mergeSession(row dbgen.ProblemMergeSession, merged domain.ContentMerge) *domain.MergeSession {
	return &domain.MergeSession{ID: row.ID, ETag: row.Etag, CopyETag: row.CopyEtag, BaseRevision: revisionPointer(row.BaseRevision), RemoteRevision: revisionPointer(row.RemoteRevision), Result: merged}
}
