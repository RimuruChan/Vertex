package problemset

import (
	"context"
	"errors"
	"sort"

	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/problem"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *SetStore) Create(ctx context.Context, authorID string, input UpsertInput) (*Set, error) {
	prepared, err := prepare(input)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	scope, err := domain.LockScope(ctx, tx, authorID)
	if err != nil {
		return nil, accessError(err)
	}
	if !scope.Allows(domain.CreateProblemSet) {
		return nil, ErrForbidden
	}
	var id string
	err = tx.QueryRowxContext(ctx, `INSERT INTO problem_sets(title,description,visibility,author_id,owner_id,domain_id)
	 VALUES($1,$2,$3,$4,$4,$5) RETURNING id`, prepared.Title, prepared.Description, prepared.Visibility, authorID, scope.Domain.ID).Scan(&id)
	if err != nil {
		return nil, err
	}
	if err := audit(ctx, tx, Access{Scope: scope, SetID: id}, "problem-set.create", ""); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, id, authorID, false)
}

func (s *SetStore) Update(ctx context.Context, id, viewerID string, _ bool, input UpsertInput) (*Set, error) {
	prepared, err := prepare(input)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	access, err := s.lockAccess(ctx, tx, id, viewerID)
	if err != nil {
		return nil, err
	}
	if !access.Permissions.Edit || (access.Visibility != prepared.Visibility && !access.Permissions.Publish) {
		return nil, ErrForbidden
	}
	if _, err := tx.ExecContext(ctx, `UPDATE problem_sets SET title=$2,description=$3,visibility=$4,updated_at=now() WHERE id=$1`,
		id, prepared.Title, prepared.Description, prepared.Visibility); err != nil {
		return nil, err
	}
	if err := audit(ctx, tx, access, "problem-set.update", ""); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, id, viewerID, false)
}

func (s *SetStore) Delete(ctx context.Context, id, viewerID string) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	access, err := s.lockAccess(ctx, tx, id, viewerID)
	if err != nil {
		return err
	}
	if !access.Permissions.Delete {
		return ErrForbidden
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM problem_sets WHERE id=$1", id); err != nil {
		return err
	}
	if err := audit(ctx, tx, access, "problem-set.delete", ""); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SetStore) SetItems(ctx context.Context, id, viewerID string, _ bool, items []ItemInput) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	access, err := s.lockAccess(ctx, tx, id, viewerID)
	if err != nil {
		return err
	}
	if !access.Permissions.Edit {
		return ErrForbidden
	}
	if len(items) > maxItems {
		return invalid("a problem set may contain at most 500 problems")
	}
	var existing []string
	if err := tx.SelectContext(ctx, &existing, "SELECT problem_id FROM problem_set_problems WHERE set_id=$1", id); err != nil {
		return err
	}
	all := make(map[string]bool, len(existing)+len(items))
	for _, id := range existing {
		all[id] = true
	}
	seen := make(map[string]bool, len(items))
	for i := range items {
		var parsed pgtype.UUID
		if err := parsed.Scan(items[i].ProblemID); err != nil || !parsed.Valid {
			return invalid("one or more problems are unavailable")
		}
		items[i].ProblemID = parsed.String()
		if seen[items[i].ProblemID] {
			return invalid("a problem may only appear once in a set")
		}
		seen[items[i].ProblemID] = true
		if len(items[i].Note) > maxNoteLength {
			return invalid("a problem note must be at most 500 characters")
		}
		if _, ok := all[items[i].ProblemID]; !ok {
			all[items[i].ProblemID] = false
		}
	}
	ordered := make([]string, 0, len(all))
	for id := range all {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	for _, problemID := range ordered {
		target, err := problem.LockAuthorization(ctx, tx, problemID, viewerID)
		if errors.Is(err, problem.ErrNotFound) {
			return invalid("one or more problems are unavailable")
		}
		if err != nil {
			return err
		}
		if !target.Permissions.View {
			if all[problemID] {
				return invalid("some existing items are inaccessible; request problem access before replacing the list")
			}
			return invalid("one or more problems are unavailable")
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM problem_set_problems WHERE set_id=$1", id); err != nil {
		return err
	}
	for order, entry := range items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO problem_set_problems(domain_id,set_id,problem_id,sort_order,note) VALUES($1,$2,$3,$4,$5)`,
			access.Scope.Domain.ID, id, entry.ProblemID, order, entry.Note); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE problem_sets SET updated_at=now() WHERE id=$1", id); err != nil {
		return err
	}
	if err := audit(ctx, tx, access, "problem-set.items.update", ""); err != nil {
		return err
	}
	return tx.Commit()
}
