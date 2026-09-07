package content

import (
	"context"
	"fmt"

	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/RimuruChan/Vertex/server/internal/domain"
)

type DiscussionStore struct{ db *database.DB }

func NewDiscussionStore(db *database.DB) *DiscussionStore { return &DiscussionStore{db: db} }

const postColumns = `d.id,d.problem_id,d.editorial_id,d.author_id,COALESCE(u.username,''),d.content_md,d.parent_id,d.created_at,d.updated_at,d.domain_id`
const postJoins = ` FROM discussion_posts d LEFT JOIN users u ON u.id=d.author_id `

func scanPost(scanner interface{ Scan(...any) error }, access threadAccess) (DiscussionPost, error) {
	var item DiscussionPost
	err := scanner.Scan(&item.ID, &item.ProblemID, &item.EditorialID, &item.AuthorID, &item.AuthorName, &item.ContentMD, &item.ParentID, &item.CreatedAt, &item.UpdatedAt, &item.DomainID)
	item.Permissions = PostPermissions(access.scope, true, access.moderator, item.AuthorID)
	item.Permissions.Comment = access.canPost
	return item, err
}

func (s *DiscussionStore) ListByProblem(ctx context.Context, id, userID string) (Thread, error) {
	return s.list(ctx, "problem_id", id, userID)
}

func (s *DiscussionStore) ListByEditorial(ctx context.Context, id, userID string) (Thread, error) {
	return s.list(ctx, "editorial_id", id, userID)
}

func (s *DiscussionStore) list(ctx context.Context, column, id, userID string) (Thread, error) {
	access, err := s.readTarget(ctx, column, id, userID)
	if err != nil {
		return Thread{}, err
	}
	rows, err := s.db.Pool.QueryxContext(ctx, "SELECT "+postColumns+postJoins+" WHERE d.domain_id=$1 AND d."+column+"=$2 ORDER BY d.created_at,d.id", domain.ID(ctx), id)
	if err != nil {
		return Thread{}, err
	}
	defer rows.Close()
	thread := Thread{Posts: []DiscussionPost{}, CanPost: access.canPost}
	for rows.Next() {
		item, err := scanPost(rows, access)
		if err != nil {
			return Thread{}, err
		}
		thread.Posts = append(thread.Posts, item)
	}
	return thread, rows.Err()
}

func readPost(ctx context.Context, q rowReader, id int64, access threadAccess, lock bool) (*DiscussionPost, error) {
	query := "SELECT " + postColumns + postJoins + " WHERE d.domain_id=$1 AND d.id=$2"
	if lock {
		query += " FOR UPDATE OF d"
	}
	item, err := scanPost(q.QueryRowxContext(ctx, query, domain.ID(ctx), id), access)
	if err != nil {
		return nil, accessError(err)
	}
	return &item, nil
}

func (s *DiscussionStore) Get(ctx context.Context, id int64, userID string) (*DiscussionPost, error) {
	column, target, err := postTarget(ctx, s.db.Pool, id)
	if err != nil {
		return nil, err
	}
	access, err := s.readTarget(ctx, column, target, userID)
	if err != nil {
		return nil, err
	}
	return readPost(ctx, s.db.Pool, id, access, false)
}

func (s *DiscussionStore) CreateProblemPost(ctx context.Context, id, userID, body string, parentID *int64) (*DiscussionPost, error) {
	return s.create(ctx, "problem_id", id, userID, body, parentID)
}

func (s *DiscussionStore) CreateEditorialPost(ctx context.Context, id, userID, body string, parentID *int64) (*DiscussionPost, error) {
	return s.create(ctx, "editorial_id", id, userID, body, parentID)
}

// Scope columns are chosen above, never supplied by a request. Parent locks
// make the same-thread check and insertion indivisible from parent deletion.
func (s *DiscussionStore) create(ctx context.Context, column, target, userID, body string, parentID *int64) (*DiscussionPost, error) {
	if err := checkPost(target, userID, body, parentID); err != nil {
		return nil, err
	}
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	access, err := lockTarget(ctx, tx, column, target, userID)
	if err != nil {
		return nil, err
	}
	if !access.canPost {
		return nil, ErrForbidden
	}
	if parentID != nil {
		var present int
		err := tx.GetContext(ctx, &present, "SELECT 1 FROM discussion_posts WHERE id=$1 AND domain_id=$2 AND "+column+"=$3 FOR SHARE", *parentID, domain.ID(ctx), target)
		if err != nil {
			if accessError(err) == ErrNotFound {
				return nil, invalid("parent post is unavailable in this discussion")
			}
			return nil, err
		}
	}
	var id int64
	err = tx.QueryRowxContext(ctx, "INSERT INTO discussion_posts(domain_id,"+column+",author_id,content_md,parent_id) VALUES($1,$2,$3,$4,$5) RETURNING id", domain.ID(ctx), target, userID, body, parentID).Scan(&id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, id, userID)
}

func (s *DiscussionStore) Update(ctx context.Context, id int64, userID, body string) (*DiscussionPost, error) {
	if err := checkPost("post", userID, body, nil); err != nil {
		return nil, err
	}
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	column, target, err := postTarget(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	access, err := lockTarget(ctx, tx, column, target, userID)
	if err != nil {
		return nil, err
	}
	item, err := readPost(ctx, tx, id, access, true)
	if err != nil {
		return nil, err
	}
	if !item.Permissions.Edit {
		return nil, ErrForbidden
	}
	if _, err := tx.ExecContext(ctx, "UPDATE discussion_posts SET content_md=$2,updated_at=now() WHERE id=$1", id, body); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, id, userID)
}

func (s *DiscussionStore) Delete(ctx context.Context, id int64, userID string) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	column, target, err := postTarget(ctx, tx, id)
	if err != nil {
		return err
	}
	access, err := lockTarget(ctx, tx, column, target, userID)
	if err != nil {
		return err
	}
	item, err := readPost(ctx, tx, id, access, true)
	if err != nil {
		return err
	}
	if !item.Permissions.Delete {
		return ErrForbidden
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM discussion_posts WHERE id=$1", id); err != nil {
		return err
	}
	if err := auditContent(ctx, tx, userID, "discussion.delete", fmt.Sprint(id)); err != nil {
		return err
	}
	return tx.Commit()
}
