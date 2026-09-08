package postgres

import (
	"context"
	"database/sql"
	"errors"

	contentdomain "github.com/RimuruChan/Vertex/server/internal/content/domain"
	"github.com/RimuruChan/Vertex/server/internal/content/infrastructure/postgres/internal/dbgen"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/problem/domain"
	problempg "github.com/RimuruChan/Vertex/server/internal/problem/infrastructure/postgres"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/tenancy/infrastructure/postgres"

	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/jmoiron/sqlx"
)

type ProblemAccess struct{ db *database.DB }

func NewProblemAccess(db *database.DB) *ProblemAccess { return &ProblemAccess{db: db} }

// Access is resolved from current account, domain and problem grants.
func (s *ProblemAccess) CanViewProblem(ctx context.Context, problemID string, userID string) (bool, error) {
	access, err := problempg.LoadAccess(ctx, s.db.Pool, problemID, userID)
	if errors.Is(err, problemdomain.ErrNotFound) {
		return false, nil
	}
	return access.Permissions.View, err
}

func accessError(err error) error {
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, problemdomain.ErrNotFound) || errors.Is(err, tenancydomain.ErrNotFound) {
		return contentdomain.ErrNotFound
	}
	if errors.Is(err, tenancydomain.ErrForbidden) {
		return contentdomain.ErrForbidden
	}
	return err
}

func lockEditorial(ctx context.Context, tx *sqlx.Tx, id, userID string) (*contentdomain.Editorial, error) {
	scope, err := tenancypg.LockScope(ctx, tx, userID)
	if err != nil {
		return nil, accessError(err)
	}
	problemID, err := dbgen.New(tx).GetEditorialProblem(ctx, dbgen.GetEditorialProblemParams{EditorialID: id, DomainID: scope.Domain.ID})
	if err != nil {
		return nil, accessError(err)
	}
	// Resource authorization is stable without occupying worker-owned statistics rows.
	if _, err := problempg.LockAuthorization(ctx, tx, problemID, userID); err != nil {
		return nil, accessError(err)
	}
	return readEditorial(ctx, tx, scope, id, true)
}

func auditContent(ctx context.Context, tx *sqlx.Tx, userID, action, target string) error {
	return dbgen.New(tx).RecordContentAudit(ctx, dbgen.RecordContentAuditParams{DomainID: tenancydomain.ID(ctx), UserID: userID, Action: action, Target: target})
}
