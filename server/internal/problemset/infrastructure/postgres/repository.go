package postgres

import (
	"context"
	"errors"
	"sort"

	problemdomain "github.com/RimuruChan/Vertex/server/internal/problem/domain"
	problempg "github.com/RimuruChan/Vertex/server/internal/problem/infrastructure/postgres"
	domain "github.com/RimuruChan/Vertex/server/internal/problemset/domain"
	"github.com/RimuruChan/Vertex/server/internal/problemset/infrastructure/postgres/internal/dbgen"
	tenancy "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/tenancy/infrastructure/postgres"
	"github.com/jackc/pgx/v5/pgtype"
)

func (r *Repository) Create(ctx context.Context, authorID string, input domain.UpsertInput) (*domain.Set, error) {
	prepared, err := domain.Prepare(input)
	if err != nil {
		return nil, err
	}
	tx, err := r.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	scope, err := tenancypg.LockScope(ctx, tx, authorID)
	if err != nil {
		return nil, accessError(err)
	}
	if !scope.Allows(tenancy.CreateProblemSet) {
		return nil, domain.ErrForbidden
	}
	queries := r.queries.WithTx(tx.Tx)
	id, err := queries.CreateProblemSet(ctx, dbgen.CreateProblemSetParams{Title: prepared.Title, Description: prepared.Description, Visibility: prepared.Visibility, AuthorID: authorID, DomainID: scope.Domain.ID})
	if err != nil {
		return nil, err
	}
	if err := audit(ctx, queries, domain.Access{Scope: scope, SetID: id}, "problem-set.create", ""); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.Get(ctx, id, authorID)
}

func (r *Repository) Update(ctx context.Context, id, viewerID string, input domain.UpsertInput) (*domain.Set, error) {
	prepared, err := domain.Prepare(input)
	if err != nil {
		return nil, err
	}
	tx, err := r.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	access, err := r.lockAccess(ctx, tx, id, viewerID)
	if err != nil {
		return nil, err
	}
	if !access.Permissions.Edit || (access.Visibility != prepared.Visibility && !access.Permissions.Publish) {
		return nil, domain.ErrForbidden
	}
	queries := r.queries.WithTx(tx.Tx)
	if err := queries.UpdateProblemSet(ctx, dbgen.UpdateProblemSetParams{SetID: id, Title: prepared.Title, Description: prepared.Description, Visibility: prepared.Visibility}); err != nil {
		return nil, err
	}
	if err := audit(ctx, queries, access, "problem-set.update", ""); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.Get(ctx, id, viewerID)
}

func (r *Repository) Delete(ctx context.Context, id, viewerID string) error {
	tx, err := r.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	access, err := r.lockAccess(ctx, tx, id, viewerID)
	if err != nil {
		return err
	}
	if !access.Permissions.Delete {
		return domain.ErrForbidden
	}
	queries := r.queries.WithTx(tx.Tx)
	if err := queries.DeleteProblemSet(ctx, id); err != nil {
		return err
	}
	if err := audit(ctx, queries, access, "problem-set.delete", ""); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) SetItems(ctx context.Context, id, viewerID string, items []domain.ItemInput) error {
	tx, err := r.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	access, err := r.lockAccess(ctx, tx, id, viewerID)
	if err != nil {
		return err
	}
	if !access.Permissions.Edit {
		return domain.ErrForbidden
	}
	if len(items) > domain.MaxItems {
		return domain.Invalid("a problem set may contain at most 500 problems")
	}
	queries := r.queries.WithTx(tx.Tx)
	existing, err := queries.ListProblemSetProblemIDs(ctx, id)
	if err != nil {
		return err
	}
	// Lock the union of old and new problem references in deterministic order.
	// Hidden existing entries cannot be silently discarded by replacing a set.
	all := make(map[string]bool, len(existing)+len(items))
	for _, id := range existing {
		all[id] = true
	}
	seen := make(map[string]bool, len(items))
	for i := range items {
		var parsed pgtype.UUID
		if err := parsed.Scan(items[i].ProblemID); err != nil || !parsed.Valid {
			return domain.Invalid("one or more problems are unavailable")
		}
		items[i].ProblemID = parsed.String()
		if seen[items[i].ProblemID] {
			return domain.Invalid("a problem may only appear once in a set")
		}
		seen[items[i].ProblemID] = true
		if len(items[i].Note) > domain.MaxNoteLength {
			return domain.Invalid("a problem note must be at most 500 characters")
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
		target, err := problempg.LockAuthorization(ctx, tx, problemID, viewerID)
		if errors.Is(err, problemdomain.ErrNotFound) {
			return domain.Invalid("one or more problems are unavailable")
		}
		if err != nil {
			return err
		}
		if !target.Permissions.View {
			if all[problemID] {
				return domain.Invalid("some existing items are inaccessible; request problem access before replacing the list")
			}
			return domain.Invalid("one or more problems are unavailable")
		}
	}

	if err := queries.DeleteProblemSetItems(ctx, id); err != nil {
		return err
	}
	for order, entry := range items {
		if err := queries.InsertProblemSetItem(ctx, dbgen.InsertProblemSetItemParams{DomainID: access.Scope.Domain.ID, SetID: id, ProblemID: entry.ProblemID, SortOrder: order, Note: entry.Note}); err != nil {
			return err
		}
	}
	if err := queries.TouchProblemSet(ctx, id); err != nil {
		return err
	}
	if err := audit(ctx, queries, access, "problem-set.items.update", ""); err != nil {
		return err
	}
	return tx.Commit()
}
