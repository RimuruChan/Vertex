package content

import (
	"context"

	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/problem"
	"github.com/jmoiron/sqlx"
)

type threadAccess struct {
	scope              domain.Scope
	moderator, canPost bool
}

func problemThread(parent problem.Access) (threadAccess, error) {
	if !parent.Permissions.View {
		return threadAccess{}, ErrNotFound
	}
	scope := parent.Scope
	return threadAccess{scope: scope, moderator: contentManager(scope) || (scope.ActiveMember() && parent.OwnerID == scope.UserID), canPost: scope.Allows(domain.CreateContent)}, nil
}

func editorialThread(scope domain.Scope, item *Editorial) (threadAccess, error) {
	if item.Locked {
		return threadAccess{}, ErrSpoilerLocked
	}
	return threadAccess{scope: scope, moderator: item.Permissions.Delete, canPost: item.Permissions.Comment}, nil
}

func (s *DiscussionStore) readTarget(ctx context.Context, column, id, userID string) (threadAccess, error) {
	if column == "problem_id" {
		parent, err := problem.LoadAccess(ctx, s.db.Pool, id, userID)
		if err != nil {
			return threadAccess{}, accessError(err)
		}
		return problemThread(parent)
	}
	scope, err := domain.ResourceScope(ctx, s.db.Pool, userID)
	if err != nil {
		return threadAccess{}, accessError(err)
	}
	item, err := readEditorial(ctx, s.db.Pool, scope, id, false)
	if err != nil {
		return threadAccess{}, err
	}
	return editorialThread(scope, item)
}

func lockTarget(ctx context.Context, tx *sqlx.Tx, column, id, userID string) (threadAccess, error) {
	if column == "problem_id" {
		parent, err := problem.LockAuthorization(ctx, tx, id, userID)
		if err != nil {
			return threadAccess{}, accessError(err)
		}
		return problemThread(parent)
	}
	scope, err := domain.LockScope(ctx, tx, userID)
	if err != nil {
		return threadAccess{}, accessError(err)
	}
	item, err := lockEditorial(ctx, tx, id, userID)
	if err != nil {
		return threadAccess{}, err
	}
	return editorialThread(scope, item)
}

func postTarget(ctx context.Context, q rowReader, id int64) (string, string, error) {
	var problemID, editorialID *string
	if err := q.QueryRowxContext(ctx, "SELECT problem_id,editorial_id FROM discussion_posts WHERE id=$1 AND domain_id=$2", id, domain.ID(ctx)).Scan(&problemID, &editorialID); err != nil {
		return "", "", accessError(err)
	}
	if problemID != nil {
		return "problem_id", *problemID, nil
	}
	return "editorial_id", *editorialID, nil
}
