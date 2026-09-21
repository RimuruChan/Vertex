package postgres

import (
	"context"
	"io"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres/internal/dbgen"
)

func (repo *RevisionRepository) Inspect(ctx context.Context, id string, revision int64, etag string) (*domain.MaterialInspection, error) {
	if revision < 0 {
		return nil, domain.InvalidInput("revision cannot be negative")
	}
	var report domain.MaterialInspection
	err := repo.transaction(ctx, id, false, func(q *dbgen.Queries, actor string) error {
		tree, token, err := selectCheckTree(ctx, q, id, actor, revision, etag)
		if err != nil {
			return err
		}
		report, _, err = repo.inspectTree(ctx, id, tree)
		if revision > 0 {
			report.Revision = &revision
		} else {
			report.ETag = token
		}
		return err
	})
	return &report, err
}

func selectCheckTree(ctx context.Context, q *dbgen.Queries, id, actor string, revision int64, etag string) (domain.ContentTree, string, error) {
	if revision > 0 {
		row, err := q.ReadContentCommit(ctx, dbgen.ReadContentCommitParams{ProblemID: id, Revision: revision})
		if err != nil {
			return domain.ContentTree{}, "", err
		}
		tree, err := loadTree(ctx, q, id, row.TreeHash)
		return tree, "", err
	}
	copy, _, err := readCopy(ctx, q, id, actor)
	if err != nil {
		return domain.ContentTree{}, "", err
	}
	if etag != "" && copy.ETag != etag {
		return domain.ContentTree{}, "", domain.ErrWorkingCopyConflict
	}
	if copy.MergeID != "" {
		return domain.ContentTree{}, "", domain.ErrMergeRequired
	}
	return copy.Tree, copy.ETag, nil
}

// This helper only runs after selecting a readable tree inside authorization's
// transaction. Calling the public Blob method here would acquire a second set
// of database locks and would not describe one coherent inspection.
func (repo *RevisionRepository) inspectTree(ctx context.Context, id string, tree domain.ContentTree) (domain.MaterialInspection, *domain.CheckSnapshot, error) {
	return domain.InspectMaterials(tree, func(ref domain.BlobRef) ([]byte, error) {
		file, err := repo.blobs.Open(ctx, id, ref)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		return io.ReadAll(io.LimitReader(file, (1<<20)+1))
	})
}
