package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/contest/infrastructure/postgres/internal/dbgen"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"

	"github.com/jmoiron/sqlx"
)

func policyError(err error) error {
	if errors.Is(err, tenancydomain.ErrNotFound) {
		return contestdomain.ErrNotFound
	}
	if errors.Is(err, tenancydomain.ErrForbidden) {
		return contestdomain.ErrForbidden
	}
	return err
}

func readAccess(ctx context.Context, db dbgen.DBTX, scope tenancydomain.Scope, id string, lock bool) (contestdomain.Access, error) {
	q := dbgen.New(db)
	args := dbgen.GetContestAccessParams{ContestID: id, DomainID: scope.Domain.ID}
	var row dbgen.GetContestAccessRow
	var err error
	if lock {
		locked, e := q.LockContestAccess(ctx, dbgen.LockContestAccessParams(args))
		row, err = dbgen.GetContestAccessRow(locked), e
	} else {
		row, err = q.GetContestAccess(ctx, args)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return contestdomain.Access{}, contestdomain.ErrNotFound
	}
	if err != nil {
		return contestdomain.Access{}, err
	}
	value := contestdomain.Access{Scope: scope, ContestID: row.ID, OwnerID: row.OwnerID, Visibility: row.Visibility, Admission: row.Admission, BeginAt: row.BeginAt, EndAt: row.EndAt, PasswordHash: row.PasswordHash, AllowSelfRegistration: row.AllowSelfRegistration, AllowLateRegistration: row.AllowLateRegistration}
	if scope.ActiveMember() {
		grants, err := q.GetContestGrantFlags(ctx, dbgen.GetContestGrantFlagsParams{ContestID: id, DomainID: scope.Domain.ID, UserID: scope.UserID})
		if err != nil {
			return contestdomain.Access{}, err
		}
		value.Grants = contestdomain.Grants{Editor: grants.Editor, Jury: grants.Jury, Observer: grants.Observer, Participant: grants.Participant}
	}
	if scope.UserID != "" {
		value.Registered, err = q.HasRegistration(ctx, dbgen.HasRegistrationParams{ContestID: id, UserID: scope.UserID})
		if err != nil {
			return contestdomain.Access{}, err
		}
	}
	value.Permissions = contestdomain.EffectivePermissions(scope, value.OwnerID, value.Visibility, value.Admission, value.Grants, value.Registered)
	value.Permissions.Register = value.Permissions.Register && contestdomain.RegistrationError(value.AllowSelfRegistration, value.AllowLateRegistration, value.BeginAt, value.EndAt, time.Now()) == nil
	return value, nil
}

func LoadAccess(ctx context.Context, db *sqlx.DB, id, userID string) (contestdomain.Access, error) {
	scope, err := tenancypg.ResourceScope(ctx, db, userID)
	if err != nil {
		return contestdomain.Access{}, policyError(err)
	}
	return readAccess(ctx, db, scope, id, false)
}

func LockAccess(ctx context.Context, tx *sqlx.Tx, id, userID string) (contestdomain.Access, error) {
	scope, err := tenancypg.LockScope(ctx, tx, userID)
	if err != nil {
		return contestdomain.Access{}, policyError(err)
	}
	if err := tenancypg.ResourceGuard(ctx, tx, "contest", id, true); err != nil {
		return contestdomain.Access{}, err
	}
	return readAccess(ctx, tx, scope, id, true)
}

func LockAuthorization(ctx context.Context, tx *sqlx.Tx, id, userID string) (contestdomain.Access, error) {
	scope, err := tenancypg.LockScope(ctx, tx, userID)
	if err != nil {
		return contestdomain.Access{}, policyError(err)
	}
	if err := tenancypg.ResourceGuard(ctx, tx, "contest", id, false); err != nil {
		return contestdomain.Access{}, err
	}
	return readAccess(ctx, tx, scope, id, false)
}

// persistGrant runs inside the caller's authorized contest transaction. The
// caller obtains LockAccess and checks ManageAccess before resolving a subject.
func persistGrant(ctx context.Context, tx *sqlx.Tx, access contestdomain.Access, subjectID, role string, group bool) error {
	q := dbgen.New(tx)
	if group {
		return q.GrantContestGroup(ctx, dbgen.GrantContestGroupParams{DomainID: access.Scope.Domain.ID, ContestID: access.ContestID, SubjectID: subjectID, Role: role, ActorID: access.Scope.UserID})
	}
	return q.GrantContestUser(ctx, dbgen.GrantContestUserParams{DomainID: access.Scope.Domain.ID, ContestID: access.ContestID, SubjectID: subjectID, Role: role, ActorID: access.Scope.UserID})
}

func (r *Repository) Access(ctx context.Context, id, userID string) (contestdomain.Access, error) {
	return LoadAccess(ctx, r.db.Pool, id, userID)
}
