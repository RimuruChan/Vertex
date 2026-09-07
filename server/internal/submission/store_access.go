package submission

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/RimuruChan/Vertex/server/internal/contest"
	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/problem"
	"github.com/jmoiron/sqlx"
)

// Filter the observable status, not a hidden verdict. Otherwise list totals
// become an oracle even though the service later redacts every returned row.
func appendVisibleStatusFilter(args *[]any, viewer Viewer, status string) string {
	*args = append(*args, viewer.UserID, viewer.Admin, viewer.ActiveMember, status)
	u, a, m, w := len(*args)-3, len(*args)-2, len(*args)-1, len(*args)
	return fmt.Sprintf(`(CASE WHEN s.status NOT IN ('Pending','Judging') AND EXISTS (
	 SELECT 1 FROM contests feedback_contest WHERE feedback_contest.id=s.contest_id
	 AND feedback_contest.feedback='none' AND feedback_contest.end_at>=now()
	 AND NOT ($%[2]d OR ($%[3]d AND feedback_contest.owner_id=NULLIF($%[1]d::text,'')::uuid)
	 OR EXISTS(SELECT 1 FROM contest_staff staff WHERE staff.contest_id=feedback_contest.id AND staff.user_id=NULLIF($%[1]d::text,'')::uuid))
	) THEN 'Submitted' ELSE s.status END)=$%[4]d`, u, a, m, w)
}

func readManager(scope domain.Scope) bool {
	scope.Domain.Archived = false
	return scope.Allows(domain.ManageResources)
}

// resolveViewer ignores caller-supplied administrator claims.
func (s *SubmissionStore) resolveViewer(ctx context.Context, viewer Viewer) (Viewer, error) {
	scope, err := domain.ResourceScope(ctx, s.db.Pool, viewer.UserID)
	if err != nil {
		return Viewer{}, err
	}
	viewer.Admin = readManager(scope)
	viewer.ActiveMember = scope.ActiveMember()
	return viewer, nil
}

// A problem owner can rejudge practice, not contests reusing that problem.
func lockRejudgeTarget(ctx context.Context, tx *sqlx.Tx, contestID, problemID, userID string) (bool, error) {
	scope, err := domain.LockScope(ctx, tx, userID)
	if err != nil {
		return false, err
	}
	if scope.Domain.Archived {
		return false, domain.ErrForbidden
	}
	if scope.Allows(domain.ManageResources) {
		return contestID == "" && problemID != "", nil
	}
	if contestID != "" {
		access, err := contest.LockAuthorization(ctx, tx, contestID, userID)
		if err != nil {
			return false, err
		}
		if !access.Permissions.Rejudge {
			return false, domain.ErrForbidden
		}
		return false, nil
	}
	if problemID != "" {
		access, err := problem.LockAuthorization(ctx, tx, problemID, userID)
		if err != nil {
			return false, err
		}
		if access.Permissions.ManageAccess {
			return true, nil
		}
	}
	return false, domain.ErrForbidden
}

func (s *SubmissionStore) readRejudgeTarget(ctx context.Context, contestID, problemID *string) error {
	userID := domain.ActorID(ctx)
	if userID == "" {
		return domain.ErrUnauthenticated
	}
	scope, err := domain.ResourceScope(ctx, s.db.Pool, userID)
	if err != nil {
		return err
	}
	if readManager(scope) {
		return nil
	}
	if contestID != nil {
		access, err := contest.LoadAccess(ctx, s.db.Pool, *contestID, userID)
		if err != nil {
			return err
		}
		if access.Permissions.ViewJury {
			return nil
		}
	} else if problemID != nil {
		access, err := problem.LoadAccess(ctx, s.db.Pool, *problemID, userID)
		if err != nil {
			return err
		}
		if access.Permissions.ReadPackage {
			return nil
		}
	}
	return domain.ErrForbidden
}

func (s *SubmissionStore) sourceAccess(ctx context.Context, sub *Submission, viewer Viewer) (bool, error) {
	if sub.UserID == viewer.UserID || viewer.Admin {
		return true, nil
	}
	if sub.ContestID != nil {
		access, err := contest.LoadAccess(ctx, s.db.Pool, *sub.ContestID, viewer.UserID)
		return access.Permissions.ViewJury, err
	}
	access, err := problem.LoadAccess(ctx, s.db.Pool, sub.ProblemID, viewer.UserID)
	return access.Scope.ActiveMember() && access.OwnerID == viewer.UserID, err
}

func loadRejudgeParent(ctx context.Context, tx *sqlx.Tx, id string) (*string, *string, error) {
	var contestID, problemID *string
	err := tx.QueryRowxContext(ctx, "SELECT contest_id,problem_id FROM rejudgings WHERE id=$1 AND domain_id=$2", id, domain.ID(ctx)).Scan(&contestID, &problemID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrRejudgeNotFound
	}
	return contestID, problemID, err
}

func refValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// Rejudge generation changes are serialized within a domain. Workers never
// take this advisory lock; normal claims/results keep their job -> submission
// order and continue while authorization stays stable under shared guards.
func lockRejudgeGeneration(ctx context.Context, tx *sqlx.Tx) error {
	_, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "rejudge-generation:"+domain.ID(ctx))
	return err
}

func auditRejudge(ctx context.Context, tx *sqlx.Tx, userID, action, target string) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO domain_audit_events(domain_id,actor_id,action,target) VALUES($1,$2,$3,$4)", domain.ID(ctx), userID, action, target)
	return err
}
