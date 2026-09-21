package postgres

import (
	"context"
	"encoding/json"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres/internal/dbgen"
)

// Update merges the shared head into the private copy without creating a commit.
func (repo *RevisionRepository) Update(ctx context.Context, id, etag string) (*domain.CommitOutcome, error) {
	var result domain.CommitOutcome
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
		if row.BaseRevision != head.Revision || row.TreeHash != hash {
			if err := replaceCopy(ctx, q, id, actor, etag, hash, head.Revision); err != nil {
				return err
			}
		}
		copy, _, err = readCopy(ctx, q, id, actor)
		if err == nil {
			result.Copy = *copy
		}
		return err
	})
	return &result, err
}

// Reset discards to the current shared head when revision is zero, or restores
// historical content as an uncommitted edit. Neither operation changes history.
func (repo *RevisionRepository) Reset(ctx context.Context, id, etag string, revision int64) (*domain.WorkingCopy, error) {
	if revision < 0 {
		return nil, domain.InvalidInput("revision cannot be negative")
	}
	var result *domain.WorkingCopy
	err := repo.transaction(ctx, id, true, func(q *dbgen.Queries, actor string) error {
		copy, row, err := readCopy(ctx, q, id, actor)
		if err != nil {
			return err
		}
		if copy.ETag != etag {
			return domain.ErrWorkingCopyConflict
		}
		head, err := q.ReadAuthoringHead(ctx, id)
		if err != nil {
			return err
		}
		hash, base := head.TreeHash, head.Revision
		if revision > 0 {
			commit, err := q.ReadContentCommit(ctx, dbgen.ReadContentCommitParams{ProblemID: id, Revision: revision})
			if err != nil {
				return err
			}
			hash, base = commit.TreeHash, row.BaseRevision
		}
		if copy.MergeID != "" {
			if err := q.DeleteMergeSession(ctx, dbgen.DeleteMergeSessionParams{ProblemID: id, ActorID: actor}); err != nil {
				return err
			}
		}
		if row.TreeHash != hash || row.BaseRevision != base || copy.MergeID != "" {
			if err := replaceCopy(ctx, q, id, actor, etag, hash, base); err != nil {
				return err
			}
		}
		result, _, err = readCopy(ctx, q, id, actor)
		return err
	})
	return result, err
}

func (repo *RevisionRepository) Merge(ctx context.Context, id, mergeID string) (*domain.MergeSession, error) {
	var result *domain.MergeSession
	err := repo.transaction(ctx, id, false, func(q *dbgen.Queries, actor string) error {
		row, err := q.ReadMergeSession(ctx, dbgen.ReadMergeSessionParams{ProblemID: id, ActorID: actor})
		if err != nil {
			return err
		}
		if row.ID != mergeID {
			return domain.ErrNotFound
		}
		var merged domain.ContentMerge
		if err := json.Unmarshal(row.ResultJson, &merged); err != nil {
			return err
		}
		result = mergeSession(row, merged)
		return nil
	})
	return result, err
}

// SaveMerge preserves unresolved structural conflicts as well as partial text
// decisions. Resolved keys must refer to current conflicts, never client-created
// conflict records. The final tree is validated only when completing the merge.
func (repo *RevisionRepository) SaveMerge(ctx context.Context, id, mergeID, etag string, tree domain.ContentTree, resolved []domain.ConflictKey) (*domain.MergeSession, error) {
	var result *domain.MergeSession
	err := repo.transaction(ctx, id, true, func(q *dbgen.Queries, actor string) error {
		row, err := q.ReadMergeSession(ctx, dbgen.ReadMergeSessionParams{ProblemID: id, ActorID: actor})
		if err != nil {
			return err
		}
		if row.ID != mergeID {
			return domain.ErrNotFound
		}
		if row.Etag != etag {
			return domain.ErrWorkingCopyConflict
		}
		var merged domain.ContentMerge
		if err := json.Unmarshal(row.ResultJson, &merged); err != nil {
			return err
		}
		if len(tree.Entries) > domain.MaxTreeEntries {
			return domain.InvalidInput("too many merge entries")
		}
		ids := map[string]bool{}
		for _, entry := range tree.Entries {
			if ids[entry.ID] {
				return domain.InvalidInput("duplicate merge entry ID")
			}
			ids[entry.ID] = true
			// Individual entries must be valid and accessible; cross-entry path
			// conflicts may remain while the user resolves other entries.
			if _, err := (domain.ContentTree{Entries: []domain.TreeEntry{entry}}).Canonical(); err != nil {
				return err
			}
		}
		if _, err := checkTreeBlobs(ctx, q, id, actor, tree); err != nil {
			return err
		}
		pending := map[domain.ConflictKey]bool{}
		for _, conflict := range merged.Conflicts {
			pending[domain.ConflictKey{EntryID: conflict.EntryID, Field: conflict.Field}] = true
		}
		for _, key := range resolved {
			if !pending[key] {
				return domain.InvalidInput("unknown or duplicate resolved conflict")
			}
			delete(pending, key)
		}
		remaining := []domain.ContentConflict{}
		for _, conflict := range merged.Conflicts {
			if pending[domain.ConflictKey{EntryID: conflict.EntryID, Field: conflict.Field}] {
				remaining = append(remaining, conflict)
			}
		}
		merged.Tree, merged.Conflicts = tree, remaining
		data, err := json.Marshal(merged)
		if err != nil {
			return err
		}
		affected, err := q.UpdateMergeDraft(ctx, dbgen.UpdateMergeDraftParams{ProblemID: id, ActorID: actor, ID: mergeID, Etag: etag, ResultJson: data})
		if err != nil {
			return err
		}
		if affected != 1 {
			return domain.ErrWorkingCopyConflict
		}
		row, err = q.ReadMergeSession(ctx, dbgen.ReadMergeSessionParams{ProblemID: id, ActorID: actor})
		if err != nil {
			return err
		}
		result = mergeSession(row, merged)
		return nil
	})
	return result, err
}

func (repo *RevisionRepository) CompleteMerge(ctx context.Context, id, mergeID, etag string) (*domain.WorkingCopy, error) {
	var result *domain.WorkingCopy
	err := repo.transaction(ctx, id, true, func(q *dbgen.Queries, actor string) error {
		row, err := q.ReadMergeSession(ctx, dbgen.ReadMergeSessionParams{ProblemID: id, ActorID: actor})
		if err != nil {
			return err
		}
		if row.ID != mergeID {
			return domain.ErrNotFound
		}
		if row.Etag != etag {
			return domain.ErrWorkingCopyConflict
		}
		_, copy, err := readCopy(ctx, q, id, actor)
		if err != nil {
			return err
		}
		head, err := q.ReadAuthoringHead(ctx, id)
		if err != nil {
			return err
		}
		if copy.Etag != row.CopyEtag || copy.TreeHash != row.LocalTree || copy.BaseRevision != row.BaseRevision {
			return domain.ErrMergeOutdated
		}
		var merged domain.ContentMerge
		if err := json.Unmarshal(row.ResultJson, &merged); err != nil {
			return err
		}
		if len(merged.Conflicts) != 0 {
			return domain.ErrMergeRequired
		}
		hash, err := repo.persistTree(ctx, q, id, actor, merged.Tree)
		if err != nil {
			return err
		}
		if head.Revision != row.RemoteRevision || head.TreeHash != row.RemoteTree {
			// Preserve the user's completed resolutions as their local draft,
			// based on the remote snapshot they just reconciled. Reconcile that
			// draft with any newer head rather than discarding their work.
			if err := replaceCopy(ctx, q, id, actor, copy.Etag, hash, row.RemoteRevision); err != nil {
				return err
			}
			if err := q.DeleteMergeSession(ctx, dbgen.DeleteMergeSessionParams{ProblemID: id, ActorID: actor}); err != nil {
				return err
			}
			updated, updatedRow, err := readCopy(ctx, q, id, actor)
			if err != nil {
				return err
			}
			newTree, session, err := repo.mergeHead(ctx, q, id, actor, updated, updatedRow, head)
			if err != nil {
				return err
			}
			if session != nil {
				updated.MergeID = session.ID
				result = updated
				return nil
			}
			hash, err = repo.persistTree(ctx, q, id, actor, newTree)
			if err != nil {
				return err
			}
			copy = updatedRow
		}
		if err := replaceCopy(ctx, q, id, actor, copy.Etag, hash, head.Revision); err != nil {
			return err
		}
		if err := q.DeleteMergeSession(ctx, dbgen.DeleteMergeSessionParams{ProblemID: id, ActorID: actor}); err != nil {
			return err
		}
		result, _, err = readCopy(ctx, q, id, actor)
		return err
	})
	return result, err
}
