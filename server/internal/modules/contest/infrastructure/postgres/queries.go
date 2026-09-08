package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	domain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/contest/infrastructure/postgres/internal/dbgen"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
)

type Queries struct {
	db      *database.DB
	queries *dbgen.Queries
}

func NewQueries(db *database.DB) *Queries { return &Queries{db: db, queries: dbgen.New(db.Pool.DB)} }

func contestFromRow(row dbgen.GetContestRow) domain.Contest {
	return domain.Contest{ID: row.ID, PublicID: row.PublicID, Title: row.Title, Description: row.Description, Rule: row.Rule,
		BeginAt: row.BeginAt, EndAt: row.EndAt, FreezeAt: row.FreezeAt, UnfreezeAt: row.UnfreezeAt,
		PenaltyMinutes: row.PenaltyMinutes, PenalizeCompileError: row.PenalizeCompileError, Feedback: row.Feedback,
		Visibility: row.Visibility, PasswordHash: row.PasswordHash, RankboardVisible: row.RankboardVisible,
		CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt, OwnerID: row.OwnerID, OwnerName: row.OwnerName,
		DomainID: row.DomainID, Admission: row.Admission, AllowSelfRegistration: row.AllowSelfRegistration, AllowLateRegistration: row.AllowLateRegistration}
}

func (r *Queries) Get(ctx context.Context, id string) (*domain.Contest, error) {
	row, err := r.queries.GetContest(ctx, dbgen.GetContestParams{ContestID: id, DomainID: tenancy.ID(ctx)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	item := contestFromRow(row)
	return &item, nil
}

func (r *Queries) List(ctx context.Context, limit, offset int, keyword ...string) ([]domain.Contest, int, error) {
	return r.list(ctx, limit, offset, false, keyword...)
}

func (r *Queries) ListAdmin(ctx context.Context, limit, offset int, keyword ...string) ([]domain.Contest, int, error) {
	return r.list(ctx, limit, offset, true, keyword...)
}

func (r *Queries) list(ctx context.Context, limit, offset int, managedOnly bool, keyword ...string) ([]domain.Contest, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	scope, err := tenancypg.ResourceScope(ctx, r.db.Pool, tenancy.ActorID(ctx))
	if err != nil {
		return nil, 0, policyError(err)
	}
	readScope := scope
	readScope.Domain.Archived = false
	search := ""
	if len(keyword) > 0 {
		search = strings.TrimSpace(keyword[0])
	}
	manager, canSubmit := readScope.Allows(tenancy.ManageResources), readScope.Allows(tenancy.CreateSubmission)
	total, err := r.queries.CountVisibleContests(ctx, dbgen.CountVisibleContestsParams{ViewerID: scope.UserID, DomainID: scope.Domain.ID,
		IsManager: manager, ActiveMember: scope.ActiveMember(), CanSubmit: canSubmit, ManagedOnly: managedOnly, Keyword: search})
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListVisibleContests(ctx, dbgen.ListVisibleContestsParams{ViewerID: scope.UserID, DomainID: scope.Domain.ID,
		IsManager: manager, ActiveMember: scope.ActiveMember(), CanSubmit: canSubmit, ManagedOnly: managedOnly, Keyword: search, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, err
	}
	items := make([]domain.Contest, 0, len(rows))
	for _, row := range rows {
		item := contestFromRow(dbgen.GetContestRow(row))
		grants := domain.Grants{Editor: row.Editor, Jury: row.Jury, Observer: row.Observer, Participant: row.Participant}
		item.Permissions = domain.EffectivePermissions(scope, item.OwnerID, item.Visibility, item.Admission, grants, row.Registered)
		item.Permissions.Register = item.Permissions.Register && item.RegistrationOpen(time.Now())
		items = append(items, item)
	}
	return items, int(total), nil
}
