package postgres

import (
	"context"

	contentdomain "github.com/RimuruChan/Vertex/server/internal/content/domain"
	"github.com/RimuruChan/Vertex/server/internal/content/infrastructure/postgres/internal/dbgen"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/problem/domain"
	problempg "github.com/RimuruChan/Vertex/server/internal/problem/infrastructure/postgres"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/tenancy/infrastructure/postgres"

	"github.com/jmoiron/sqlx"
)

type threadAccess struct {
	scope              tenancydomain.Scope
	moderator, canPost bool
}

func problemThread(parent problemdomain.Access) (threadAccess, error) {
	if !parent.Permissions.View {
		return threadAccess{}, contentdomain.ErrNotFound
	}
	scope := parent.Scope
	return threadAccess{scope: scope, moderator: contentdomain.ContentManager(scope) || (scope.ActiveMember() && parent.OwnerID == scope.UserID), canPost: scope.Allows(tenancydomain.CreateContent)}, nil
}

func editorialThread(scope tenancydomain.Scope, item *contentdomain.Editorial) (threadAccess, error) {
	if item.Locked {
		return threadAccess{}, contentdomain.ErrSpoilerLocked
	}
	return threadAccess{scope: scope, moderator: item.Permissions.Delete, canPost: item.Permissions.Comment}, nil
}

func (s *DiscussionRepository) readTarget(ctx context.Context, kind, id, userID string) (threadAccess, error) {
	if kind == "problem" {
		parent, err := problempg.LoadAccess(ctx, s.db.Pool, id, userID)
		if err != nil {
			return threadAccess{}, accessError(err)
		}
		return problemThread(parent)
	}
	scope, err := tenancypg.ResourceScope(ctx, s.db.Pool, userID)
	if err != nil {
		return threadAccess{}, accessError(err)
	}
	item, err := readEditorial(ctx, s.db.Pool, scope, id, false)
	if err != nil {
		return threadAccess{}, err
	}
	return editorialThread(scope, item)
}

func lockTarget(ctx context.Context, tx *sqlx.Tx, kind, id, userID string) (threadAccess, error) {
	if kind == "problem" {
		parent, err := problempg.LockAuthorization(ctx, tx, id, userID)
		if err != nil {
			return threadAccess{}, accessError(err)
		}
		return problemThread(parent)
	}
	scope, err := tenancypg.LockScope(ctx, tx, userID)
	if err != nil {
		return threadAccess{}, accessError(err)
	}
	item, err := lockEditorial(ctx, tx, id, userID)
	if err != nil {
		return threadAccess{}, err
	}
	return editorialThread(scope, item)
}

func postTarget(ctx context.Context, db dbgen.DBTX, id int64) (string, string, error) {
	row, err := dbgen.New(db).GetDiscussionTarget(ctx, dbgen.GetDiscussionTargetParams{PostID: id, DomainID: tenancydomain.ID(ctx)})
	if err != nil {
		return "", "", accessError(err)
	}
	if row.ProblemID != nil {
		return "problem", *row.ProblemID, nil
	}
	if row.EditorialID != nil {
		return "editorial", *row.EditorialID, nil
	}
	return "", "", contentdomain.ErrNotFound
}
