package postgres

import (
	"context"
	"database/sql"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres/internal/dbgen"
)

// Changes captures the draft token and both immutable trees under one shared
// authorization guard. target=0 selects my draft; from=0 selects its base (or a
// commit's parent). Initial content is shown as additions until first committed.
func (repo *RevisionRepository) Changes(ctx context.Context, id string, from, target int64) (*domain.ContentComparison, error) {
	if from < 0 || target < 0 {
		return nil, domain.InvalidInput("revision numbers cannot be negative")
	}
	result := domain.ContentComparison{}
	err := repo.transaction(ctx, id, false, func(q *dbgen.Queries, actor string) error {
		var tree domain.ContentTree
		var base sql.NullInt64
		if target == 0 {
			copy, row, err := readCopy(ctx, q, id, actor)
			if err != nil {
				return err
			}
			tree = copy.Tree
			base = row.BaseRevision
			result.ETag = copy.ETag
		} else {
			commit, err := q.ReadContentCommit(ctx, dbgen.ReadContentCommitParams{ProblemID: id, Revision: target})
			if err != nil {
				return err
			}
			tree, err = loadTree(ctx, q, id, commit.TreeHash)
			if err != nil {
				return err
			}
			base = commit.ParentRevision
			result.ToRevision = &target
		}
		if from > 0 {
			base = sql.NullInt64{Int64: from, Valid: true}
		}
		before := domain.ContentTree{}
		if base.Valid {
			commit, err := q.ReadContentCommit(ctx, dbgen.ReadContentCommitParams{ProblemID: id, Revision: base.Int64})
			if err != nil {
				return err
			}
			before, err = loadTree(ctx, q, id, commit.TreeHash)
			if err != nil {
				return err
			}
			result.FromRevision = revisionPointer(base)
		}
		changes, err := domain.DiffTrees(before, tree)
		result.Changes = changes
		return err
	})
	return &result, err
}
