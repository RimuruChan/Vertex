package postgres

import (
	"context"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres/internal/dbgen"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
)

func (repo *RevisionRepository) SetVisibility(ctx context.Context, id string, input domain.VisibilityChange) (*domain.VisibilityState, error) {
	if input.Visibility != "private" && input.Visibility != "public" {
		return nil, domain.InvalidInput("visibility must be private or public")
	}
	err := repo.transactionFor(ctx, id, manageWorkbench, func(q *dbgen.Queries, actor string) error {
		changed, err := q.ChangeAuthoringVisibility(ctx, dbgen.ChangeAuthoringVisibilityParams{ProblemID: id, Visibility: input.Visibility, ExpectedVisibility: input.ExpectedVisibility})
		if err != nil {
			return err
		}
		if changed != 1 {
			return domain.ErrWorkingCopyConflict
		}
		return q.RecordProblemCopyAudit(ctx, dbgen.RecordProblemCopyAuditParams{DomainID: tenancy.ID(ctx), ActorID: &actor, Action: "problem.visibility.changed", Target: id + ":" + input.Visibility})
	})
	if err != nil {
		return nil, err
	}
	return &domain.VisibilityState{Visibility: input.Visibility}, nil
}
