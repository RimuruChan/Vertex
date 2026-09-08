package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	domain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/contest/infrastructure/postgres/internal/dbgen"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
)

type Repository struct {
	*Queries
	db      *database.DB
	queries *dbgen.Queries
}

func NewRepository(db *database.DB) *Repository {
	return &Repository{Queries: NewQueries(db), db: db, queries: dbgen.New(db.Pool.DB)}
}

func (r *Repository) Create(ctx context.Context, createdBy string, in *domain.PersistInput) (*domain.Contest, error) {
	tx, err := r.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	scope, err := tenancypg.LockScope(ctx, tx, createdBy)
	if err != nil {
		return nil, policyError(err)
	}
	if !scope.Allows(tenancy.CreateContest) {
		return nil, domain.ErrForbidden
	}
	if in.Admission == "" {
		in.Admission = domain.AdmissionMembers
	}
	row, err := r.queries.WithTx(tx.Tx).CreateContest(ctx, dbgen.CreateContestParams{Title: in.Title, Description: in.Description, Rule: in.Rule,
		BeginAt: in.BeginAt, EndAt: in.EndAt, FreezeAt: in.FreezeAt, UnfreezeAt: in.UnfreezeAt,
		PenaltyMinutes: in.PenaltyMinutes, PenalizeCompileError: in.PenalizeCompileError, Feedback: in.Feedback,
		Visibility: in.Visibility, PasswordHash: in.PasswordHash, RankboardVisible: in.RankboardVisible,
		CreatorID: createdBy, DomainID: scope.Domain.ID, Admission: in.Admission,
		AllowSelfRegistration: domain.RegistrationSetting(in.AllowSelfRegistration, true), AllowLateRegistration: domain.RegistrationSetting(in.AllowLateRegistration, false)})
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	item := contestFromRow(dbgen.GetContestRow(row))
	item.Permissions = domain.EffectivePermissions(scope, item.OwnerID, item.Visibility, item.Admission, domain.Grants{}, false)
	item.Permissions.Register = item.Permissions.Register && item.RegistrationOpen(time.Now())
	return &item, nil
}

// Update holds the authorization lock while replacing settings and rebuilding
// every score cell affected by those settings in the same transaction.
func (r *Repository) Update(ctx context.Context, id string, in *domain.PersistInput) (*domain.Contest, error) {
	tx, err := r.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	access, err := LockAccess(ctx, tx, id, tenancy.ActorID(ctx))
	if err != nil {
		return nil, err
	}
	if in.Admission == "" {
		in.Admission = access.Admission
	}
	if !access.Permissions.Edit {
		return nil, domain.ErrForbidden
	}
	if !access.Permissions.ManageAccess && !time.Now().Before(access.BeginAt) {
		return nil, domain.ErrForbidden
	}
	self := domain.RegistrationSetting(in.AllowSelfRegistration, access.AllowSelfRegistration)
	late := domain.RegistrationSetting(in.AllowLateRegistration, access.AllowLateRegistration)
	if !access.Permissions.ManageAccess && (in.Visibility != access.Visibility || in.Admission != access.Admission || in.PasswordHash != "" || self != access.AllowSelfRegistration || late != access.AllowLateRegistration) {
		return nil, domain.ErrForbidden
	}
	if in.Visibility == "password" && in.PasswordHash == "" && access.PasswordHash == "" {
		return nil, domain.Invalid("password required")
	}
	row, err := r.queries.WithTx(tx.Tx).UpdateContest(ctx, dbgen.UpdateContestParams{ContestID: id, DomainID: access.Scope.Domain.ID,
		Title: in.Title, Description: in.Description, Rule: in.Rule, BeginAt: in.BeginAt, EndAt: in.EndAt,
		FreezeAt: in.FreezeAt, UnfreezeAt: in.UnfreezeAt, PenaltyMinutes: in.PenaltyMinutes, PenalizeCompileError: in.PenalizeCompileError,
		Feedback: in.Feedback, Visibility: in.Visibility, PasswordHash: in.PasswordHash, RankboardVisible: in.RankboardVisible,
		Admission: in.Admission, AllowSelfRegistration: self, AllowLateRegistration: late})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := RebuildContest(ctx, tx, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	item := contestFromRow(dbgen.GetContestRow(row))
	item.Permissions = domain.EffectivePermissions(access.Scope, item.OwnerID, item.Visibility, item.Admission, access.Grants, access.Registered)
	item.Permissions.Register = item.Permissions.Register && item.RegistrationOpen(time.Now())
	return &item, nil
}
