package contest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/problem"
)

// Version adoption does not rewrite old generations or results. Rejudging is
// a separate, explicit operation after the jury reviews the new release.
func (s *ContestStore) UseProblemVersion(ctx context.Context, contestID, problemID string, version, expected int) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	access, err := LockAccess(ctx, tx, contestID, domain.ActorID(ctx))
	if err != nil {
		return err
	}
	if !access.Permissions.Rejudge {
		return ErrForbidden
	}
	parent, err := problem.LockAuthorization(ctx, tx, problemID, access.Scope.UserID)
	if errors.Is(err, problem.ErrNotFound) {
		return ErrProblemNotInContest
	}
	if err != nil {
		return err
	}
	if !parent.Permissions.View {
		return ErrForbidden
	}
	var current int
	err = tx.GetContext(ctx, &current, "SELECT problem_version FROM contest_problems WHERE contest_id=$1 AND problem_id=$2 FOR UPDATE", contestID, problemID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrProblemNotInContest
	}
	if err != nil {
		return err
	}
	if current == version {
		return nil
	}
	if current != expected {
		return ErrVersionConflict
	}
	var exists bool
	if err := tx.GetContext(ctx, &exists, "SELECT EXISTS(SELECT 1 FROM problem_versions WHERE problem_id=$1 AND version_no=$2)", problemID, version); err != nil {
		return err
	}
	if !exists {
		return invalid("published version does not exist")
	}
	if _, err := tx.ExecContext(ctx, "UPDATE contest_problems SET problem_version=$3 WHERE contest_id=$1 AND problem_id=$2", contestID, problemID, version); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO domain_audit_events(domain_id,actor_id,action,target) VALUES($1,$2,'contest.problem.version',$3)", domain.ID(ctx), access.Scope.UserID, fmt.Sprintf("%s/%s:%d->%d", contestID, problemID, current, version)); err != nil {
		return err
	}
	return tx.Commit()
}
