package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/contest/infrastructure/postgres/internal/dbgen"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
	problempg "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
)

// Version adoption does not rewrite old generations or results. Rejudging is
// a separate, explicit operation after the jury reviews the new release.
func (s *Repository) UseProblemVersion(ctx context.Context, contestID, problemID string, version, expected int) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	access, err := LockAccess(ctx, tx, contestID, tenancydomain.ActorID(ctx))
	if err != nil {
		return err
	}
	if !access.Permissions.Rejudge {
		return contestdomain.ErrForbidden
	}
	parent, err := problempg.LockAuthorization(ctx, tx, problemID, access.Scope.UserID)
	if errors.Is(err, problemdomain.ErrNotFound) {
		return contestdomain.ErrProblemNotInContest
	}
	if err != nil {
		return err
	}
	if !parent.Permissions.View {
		return contestdomain.ErrForbidden
	}
	q := s.queries.WithTx(tx.Tx)
	current, err := q.GetContestProblemVersion(ctx, dbgen.GetContestProblemVersionParams{ContestID: contestID, ProblemID: problemID})
	if errors.Is(err, sql.ErrNoRows) {
		return contestdomain.ErrProblemNotInContest
	}
	if err != nil {
		return err
	}
	if current == version {
		return nil
	}
	if current != expected {
		return contestdomain.ErrVersionConflict
	}
	exists, err := q.GetProblemVersionLimits(ctx, dbgen.GetProblemVersionLimitsParams{ProblemID: problemID, Version: version})
	if err != nil {
		return err
	}

	if !exists {
		return contestdomain.Invalid("published version does not exist")
	}
	if err := q.SetContestProblemVersion(ctx, dbgen.SetContestProblemVersionParams{ContestID: contestID, ProblemID: problemID, Version: version}); err != nil {
		return err
	}
	if err := q.RecordContestVersionAudit(ctx, dbgen.RecordContestVersionAuditParams{DomainID: tenancydomain.ID(ctx), ActorID: access.Scope.UserID, Target: fmt.Sprintf("%s/%s:%d->%d", contestID, problemID, current, version)}); err != nil {
		return err
	}
	return tx.Commit()
}
