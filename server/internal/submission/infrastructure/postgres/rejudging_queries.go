package postgres

import (
	"context"
	"database/sql"
	"errors"

	contestpg "github.com/RimuruChan/Vertex/server/internal/contest/infrastructure/postgres"
	problempg "github.com/RimuruChan/Vertex/server/internal/problem/infrastructure/postgres"
	domain "github.com/RimuruChan/Vertex/server/internal/submission/domain"
	"github.com/RimuruChan/Vertex/server/internal/submission/infrastructure/postgres/internal/dbgen"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/tenancy/infrastructure/postgres"
)

func readManager(scope tenancydomain.Scope) bool {
	scope.Domain.Archived = false
	return scope.Allows(tenancydomain.ManageResources)
}

func (s *Repository) readRejudgeTarget(ctx context.Context, contestID, problemID *string) error {
	userID := tenancydomain.ActorID(ctx)
	if userID == "" {
		return tenancydomain.ErrUnauthenticated
	}
	scope, err := tenancypg.ResourceScope(ctx, s.db.Pool, userID)
	if err != nil {
		return err
	}
	if readManager(scope) {
		return nil
	}
	if contestID != nil {
		access, err := contestpg.LoadAccess(ctx, s.db.Pool, *contestID, userID)
		if err != nil {
			return err
		}
		if access.Permissions.ViewJury {
			return nil
		}
	} else if problemID != nil {
		access, err := problempg.LoadAccess(ctx, s.db.Pool, *problemID, userID)
		if err != nil {
			return err
		}
		if access.Permissions.ReadPackage {
			return nil
		}
	}
	return tenancydomain.ErrForbidden
}

// Rejudging returns live progress after checking the actor's current access.
func (r *Repository) Rejudging(ctx context.Context, id string) (*domain.Rejudging, error) {
	row, err := r.queries.GetRejudgingProgress(ctx, dbgen.GetRejudgingProgressParams{BatchID: id, DomainID: tenancydomain.ID(ctx)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrRejudgeNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := r.readRejudgeTarget(ctx, row.ContestID, row.ProblemID); err != nil {
		return nil, err
	}
	item := rejudgingFromRow(row)
	if err := r.settleRejudging(ctx, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) ListRejudgings(ctx context.Context, contestID string, limit int) ([]domain.Rejudging, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	userID := tenancydomain.ActorID(ctx)
	if userID == "" {
		return nil, tenancydomain.ErrUnauthenticated
	}
	scope, err := tenancypg.ResourceScope(ctx, r.db.Pool, userID)
	if err != nil {
		return nil, err
	}
	rows, err := r.queries.ListRejudgings(ctx, dbgen.ListRejudgingsParams{
		ContestFilter: contestID, DomainID: tenancydomain.ID(ctx), CanManage: readManager(scope),
		ActiveMember: scope.ActiveMember(), ViewerID: userID, PageLimit: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	items := make([]domain.Rejudging, 0, len(rows))
	for _, row := range rows {
		item := rejudgingFromRow(dbgen.GetRejudgingProgressRow(row))
		if err := r.settleRejudging(ctx, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// Progress includes superseded generations; only the batch state is persisted.
func (r *Repository) settleRejudging(ctx context.Context, item *domain.Rejudging) error {
	if item.State != domain.RejudgingRunning || !item.Finished() {
		return nil
	}
	row, err := r.queries.FinishRejudging(ctx, dbgen.FinishRejudgingParams{BatchID: item.ID, DomainID: tenancydomain.ID(ctx)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	} // Another reader already settled it.
	if err != nil {
		return err
	}
	item.State, item.FinishedAt = row.State, row.FinishedAt
	return nil
}

func (r *Repository) RejudgingChanges(ctx context.Context, id string, limit int) ([]domain.RejudgingChange, error) {
	if _, err := r.Rejudging(ctx, id); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.queries.ListRejudgingChanges(ctx, dbgen.ListRejudgingChangesParams{
		BatchID: id, DomainID: tenancydomain.ID(ctx), PageLimit: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	items := make([]domain.RejudgingChange, 0, len(rows))
	for _, row := range rows {
		items = append(items, domain.RejudgingChange{SubmissionID: row.SubmissionID, Username: row.Username,
			ProblemTitle: row.ProblemTitle, PriorStatus: row.PriorStatus, PriorScore: row.PriorScore,
			Status: row.Status, Score: row.Score, Judged: row.Judged})
	}
	return items, nil
}

func rejudgingFromRow(row dbgen.GetRejudgingProgressRow) domain.Rejudging {
	return domain.Rejudging{ID: row.ID, ContestID: row.ContestID, ProblemID: row.ProblemID,
		Reason: row.Reason, State: row.State, TotalCount: row.TotalCount, DoneCount: row.DoneCount,
		ChangedCount: row.ChangedCount, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt, FinishedAt: row.FinishedAt}
}
