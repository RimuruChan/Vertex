package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/RimuruChan/Vertex/server/internal/database"
	domain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
	"github.com/RimuruChan/Vertex/server/internal/tenancy/infrastructure/postgres/internal/dbgen"
	"github.com/jackc/pgx/v5/pgconn"
)

// Repository implements tenancy read models and transactional governance.
type Repository struct {
	db      *sql.DB
	queries *dbgen.Queries
}

var _ domain.Repository = (*Repository)(nil)
var _ domain.Governance = (*session)(nil)

func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db.Pool.DB, queries: dbgen.New(db.Pool.DB)}
}

type subject struct {
	ID, Username string
	Admin        bool
}

func loadSubject(ctx context.Context, db dbgen.DBTX, userID string, lock bool) (subject, error) {
	if userID == "" {
		return subject{}, nil
	}
	queries := dbgen.New(db)
	if lock {
		actor, err := queries.LockGovernanceActor(ctx, userID)
		return subject{ID: actor.ID, Username: actor.Username, Admin: actor.IsAdmin}, actorError(err)
	}
	actor, err := queries.GetGovernanceActor(ctx, userID)
	return subject{ID: actor.ID, Username: actor.Username, Admin: actor.IsAdmin}, actorError(err)
}

func actorError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrUnauthenticated
	}
	return err
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

func normalizeError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505", "23503":
			return domain.ErrConflict
		case "23514":
			return domain.ErrInvalid
		}
	}
	return err
}

func scopeFromRow(row dbgen.GetDomainScopeByIDRow, actor subject) (domain.Scope, error) {
	scope := domain.Scope{
		Domain: domain.Domain{ID: row.ID, Slug: row.Slug, Name: row.Name, Description: row.Description,
			OwnerID: row.OwnerID, OwnerName: row.OwnerName, Official: row.IsOfficial,
			Visibility: row.Visibility, JoinPolicy: row.JoinPolicy, Archived: row.Archived,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt},
		UserID: actor.ID, SiteAdmin: actor.Admin, MemberRole: row.MemberRole, MemberStatus: row.MemberStatus,
	}
	if err := json.Unmarshal(row.RolePermissions, &scope.RolePermissions); err != nil {
		return domain.Scope{}, err
	}
	return scope, nil
}

func readScope(ctx context.Context, queries *dbgen.Queries, slug string, actor subject) (domain.Scope, error) {
	row, err := queries.GetDomainScopeBySlug(ctx, dbgen.GetDomainScopeBySlugParams{ViewerID: actor.ID, Slug: slug})
	if err != nil {
		return domain.Scope{}, notFound(err)
	}
	return scopeFromRow(dbgen.GetDomainScopeByIDRow(row), actor)
}

func (r *Repository) Scope(ctx context.Context, slug, userID string) (domain.Scope, error) {
	actor, err := loadSubject(ctx, r.db, userID, false)
	if err != nil {
		return domain.Scope{}, err
	}
	return readScope(ctx, r.queries, slug, actor)
}

func (r *Repository) List(ctx context.Context, userID string, filter domain.Filters) ([]domain.Scope, int, error) {
	actor, err := loadSubject(ctx, r.db, userID, false)
	if err != nil {
		return nil, 0, err
	}
	f := filter.Normalized()
	keyword := "%" + f.Keyword + "%"
	total, err := r.queries.CountAccessibleDomains(ctx, dbgen.CountAccessibleDomainsParams{ViewerID: actor.ID, IsAdmin: actor.Admin, Keyword: keyword})
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListAccessibleDomains(ctx, dbgen.ListAccessibleDomainsParams{ViewerID: actor.ID, IsAdmin: actor.Admin, Keyword: keyword, PageLimit: f.Limit, PageOffset: f.Offset})
	if err != nil {
		return nil, 0, err
	}
	scopes := make([]domain.Scope, 0, len(rows))
	for _, row := range rows {
		scope, err := scopeFromRow(dbgen.GetDomainScopeByIDRow(row), actor)
		if err != nil {
			return nil, 0, err
		}
		scopes = append(scopes, scope)
	}
	return scopes, int(total), nil
}

// WithinGovernance holds account then domain locks while the application
// applies policy through a transaction-scoped repository. Resource mutations
// take the shared version of the same domain lock.
func (r *Repository) WithinGovernance(ctx context.Context, slug, userID string, operation func(domain.Governance) error) error {
	if userID == "" {
		return domain.ErrUnauthenticated
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	queries := r.queries.WithTx(tx)
	actor, err := loadSubject(ctx, tx, userID, true)
	if err != nil {
		return err
	}
	if _, err := queries.LockDomainForGovernance(ctx, slug); err != nil {
		return notFound(err)
	}
	scope, err := readScope(ctx, queries, slug, actor)
	if err != nil {
		return err
	}
	if !scope.CanDiscover() {
		return domain.ErrNotFound
	}
	if err := operation(&session{ctx: ctx, queries: queries, scope: scope, actor: actor}); err != nil {
		return normalizeError(err)
	}
	return normalizeError(tx.Commit())
}

func seedRoles(ctx context.Context, queries *dbgen.Queries, domainID string) error {
	for _, role := range domain.BuiltinRoles() {
		permissions, err := json.Marshal(role.Permissions)
		if err != nil {
			return err
		}
		if err := queries.SeedDomainRole(ctx, dbgen.SeedDomainRoleParams{DomainID: domainID, Key: role.Key, Name: role.Name, Permissions: permissions}); err != nil {
			return err
		}
	}
	return nil
}

// EnsureOfficial is also used by isolated database fixtures after truncation.
func (r *Repository) EnsureOfficial(ctx context.Context) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	queries := r.queries.WithTx(tx)
	if err := queries.EnsureOfficialDomain(ctx, domain.OfficialID); err != nil {
		return err
	}
	if err := seedRoles(ctx, queries, domain.OfficialID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) Create(ctx context.Context, userID string, input domain.CreateInput) (domain.Scope, error) {
	if userID == "" {
		return domain.Scope{}, domain.ErrUnauthenticated
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Scope{}, err
	}
	defer tx.Rollback()
	queries := r.queries.WithTx(tx)
	actor, err := loadSubject(ctx, tx, userID, true)
	if err != nil {
		return domain.Scope{}, err
	}
	id, err := queries.CreateDomain(ctx, dbgen.CreateDomainParams{Slug: input.Slug, Name: input.Name,
		Description: input.Description, OwnerID: actor.ID, Visibility: input.Visibility, JoinPolicy: input.JoinPolicy})
	if err != nil {
		return domain.Scope{}, normalizeError(err)
	}
	if err := seedRoles(ctx, queries, id); err != nil {
		return domain.Scope{}, err
	}
	if err := queries.AddDomainOwner(ctx, dbgen.AddDomainOwnerParams{DomainID: id, UserID: actor.ID}); err != nil {
		return domain.Scope{}, err
	}
	scope, err := readScope(ctx, queries, input.Slug, actor)
	if err != nil {
		return domain.Scope{}, err
	}
	operation := session{ctx: ctx, queries: queries, scope: scope, actor: actor}
	if err := operation.Audit("domain.created", id); err != nil {
		return domain.Scope{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Scope{}, normalizeError(err)
	}
	return scope, nil
}
