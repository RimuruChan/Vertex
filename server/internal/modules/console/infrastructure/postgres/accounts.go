package postgres

import (
	"context"
	"database/sql"
	"errors"

	domain "github.com/RimuruChan/Vertex/server/internal/modules/console/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/console/infrastructure/postgres/internal/dbgen"
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
)

// Repository keeps site-wide accounts distinct from domain-scoped resources.
type Repository struct {
	db      *database.DB
	queries *dbgen.Queries
}

var _ domain.Repository = (*Repository)(nil)

func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db, queries: dbgen.New(db.Pool.DB)}
}
func accountFromRow(row dbgen.GetAccountSummaryRow) domain.AccountSummary {
	return domain.AccountSummary{ID: row.ID, Username: row.Username, Email: row.Email, Role: row.Role, Rating: row.Rating, CreatedAt: row.CreatedAt, DisabledAt: row.DisabledAt, DisabledReason: row.DisabledReason, SubmissionCount: row.SubmissionCount, SolvedCount: row.SolvedCount}
}
func (r *Repository) account(ctx context.Context, id string) (*domain.AccountSummary, error) {
	row, err := r.queries.GetAccountSummary(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	item := accountFromRow(row)
	return &item, nil
}
func (r *Repository) ListAccounts(ctx context.Context, f domain.AccountFilters) ([]domain.AccountSummary, int, error) {
	total, err := r.queries.CountAccounts(ctx, dbgen.CountAccountsParams{Keyword: f.Keyword, Role: f.Role, OnlyDisabled: f.OnlyDisabled})
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListAccounts(ctx, dbgen.ListAccountsParams{Keyword: f.Keyword, Role: f.Role, OnlyDisabled: f.OnlyDisabled, PageLimit: f.Limit, PageOffset: f.Offset})
	if err != nil {
		return nil, 0, err
	}
	items := make([]domain.AccountSummary, 0, len(rows))
	for _, row := range rows {
		items = append(items, accountFromRow(dbgen.GetAccountSummaryRow(row)))
	}
	return items, total, nil
}
func (r *Repository) UpdateAccount(ctx context.Context, userID string, update domain.AccountUpdate) (*domain.AccountSummary, error) {
	if update.Role == nil && update.Rating == nil && update.Disabled == nil {
		return r.account(ctx, userID)
	}
	args := dbgen.UpdateAccountParams{UserID: userID, SetRole: update.Role != nil, SetRating: update.Rating != nil, SetDisabled: update.Disabled != nil, Reason: update.Reason}
	if update.Role != nil {
		args.Role = *update.Role
	}
	if update.Rating != nil {
		args.Rating = *update.Rating
	}
	if update.Disabled != nil {
		args.Disabled = *update.Disabled
	}
	tx, err := r.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	q := r.queries.WithTx(tx.Tx)
	affected, err := q.UpdateAccount(ctx, args)
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, domain.ErrNotFound
	}
	if args.SetDisabled && args.Disabled {
		if err := q.RevokeAccountSessions(ctx, userID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.account(ctx, userID)
}
