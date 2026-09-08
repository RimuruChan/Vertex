package postgres

import (
	"context"
	"database/sql"
	"fmt"

	domain "github.com/RimuruChan/Vertex/server/internal/modules/content/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/content/infrastructure/postgres/internal/dbgen"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
)

type DiscussionRepository struct {
	db      *database.DB
	queries *dbgen.Queries
}

var _ domain.DiscussionRepository = (*DiscussionRepository)(nil)

func NewDiscussionRepository(db *database.DB) *DiscussionRepository {
	return &DiscussionRepository{db: db, queries: dbgen.New(db.Pool.DB)}
}
func postFromRow(row dbgen.GetPostRow, access threadAccess) domain.DiscussionPost {
	p := domain.DiscussionPost{ID: row.ID, ProblemID: row.ProblemID, EditorialID: row.EditorialID, AuthorID: row.AuthorID, AuthorName: row.AuthorName, ContentMD: row.ContentMd, ParentID: database.Int64Ptr(row.ParentID), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, DomainID: row.DomainID}
	p.Permissions = domain.PostPermissions(access.scope, true, access.moderator, p.AuthorID)
	p.Permissions.Comment = access.canPost
	return p
}
func (s *DiscussionRepository) ListByProblem(ctx context.Context, id, userID string) (domain.Thread, error) {
	return s.list(ctx, "problem", id, userID)
}
func (s *DiscussionRepository) ListByEditorial(ctx context.Context, id, userID string) (domain.Thread, error) {
	return s.list(ctx, "editorial", id, userID)
}
func (s *DiscussionRepository) list(ctx context.Context, kind, id, userID string) (domain.Thread, error) {
	access, err := s.readTarget(ctx, kind, id, userID)
	if err != nil {
		return domain.Thread{}, err
	}
	thread := domain.Thread{Posts: []domain.DiscussionPost{}, CanPost: access.canPost}
	if kind == "problem" {
		rows, err := s.queries.ListProblemPosts(ctx, dbgen.ListProblemPostsParams{DomainID: tenancy.ID(ctx), TargetID: id})
		if err != nil {
			return domain.Thread{}, err
		}
		for _, row := range rows {
			thread.Posts = append(thread.Posts, postFromRow(dbgen.GetPostRow(row), access))
		}
	} else {
		rows, err := s.queries.ListEditorialPosts(ctx, dbgen.ListEditorialPostsParams{DomainID: tenancy.ID(ctx), TargetID: id})
		if err != nil {
			return domain.Thread{}, err
		}
		for _, row := range rows {
			thread.Posts = append(thread.Posts, postFromRow(dbgen.GetPostRow(row), access))
		}
	}
	return thread, nil
}
func readPost(ctx context.Context, db dbgen.DBTX, id int64, access threadAccess, lock bool) (*domain.DiscussionPost, error) {
	q := dbgen.New(db)
	args := dbgen.GetPostParams{DomainID: tenancy.ID(ctx), PostID: id}
	var row dbgen.GetPostRow
	var err error
	if lock {
		locked, e := q.LockPost(ctx, dbgen.LockPostParams(args))
		row, err = dbgen.GetPostRow(locked), e
	} else {
		row, err = q.GetPost(ctx, args)
	}
	if err != nil {
		return nil, accessError(err)
	}
	item := postFromRow(row, access)
	return &item, nil
}

func (s *DiscussionRepository) Get(ctx context.Context, id int64, userID string) (*domain.DiscussionPost, error) {
	kind, target, err := postTarget(ctx, s.db.Pool, id)
	if err != nil {
		return nil, err
	}
	access, err := s.readTarget(ctx, kind, target, userID)
	if err != nil {
		return nil, err
	}
	return readPost(ctx, s.db.Pool, id, access, false)
}

func (s *DiscussionRepository) CreateProblemPost(ctx context.Context, id, userID, body string, parentID *int64) (*domain.DiscussionPost, error) {
	return s.create(ctx, "problem", id, userID, body, parentID)
}

func (s *DiscussionRepository) CreateEditorialPost(ctx context.Context, id, userID, body string, parentID *int64) (*domain.DiscussionPost, error) {
	return s.create(ctx, "editorial", id, userID, body, parentID)
}

// Target kinds are chosen by the entry points. Parent locks
// make the same-thread check and insertion indivisible from parent deletion.
func (s *DiscussionRepository) create(ctx context.Context, kind, target, userID, body string, parentID *int64) (*domain.DiscussionPost, error) {
	if err := domain.CheckPost(target, userID, body, parentID); err != nil {
		return nil, err
	}
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	access, err := lockTarget(ctx, tx, kind, target, userID)
	if err != nil {
		return nil, err
	}
	if !access.canPost {
		return nil, domain.ErrForbidden
	}
	queries := s.queries.WithTx(tx.Tx)
	if parentID != nil {
		if kind == "problem" {
			_, err = queries.CheckProblemParent(ctx, dbgen.CheckProblemParentParams{ParentID: *parentID, DomainID: tenancy.ID(ctx), TargetID: target})
		} else {
			_, err = queries.CheckEditorialParent(ctx, dbgen.CheckEditorialParentParams{ParentID: *parentID, DomainID: tenancy.ID(ctx), TargetID: target})
		}
		if err != nil {
			if accessError(err) == domain.ErrNotFound {
				return nil, domain.Invalid("parent post is unavailable in this discussion")
			}
			return nil, err
		}
	}
	parent := sql.NullInt64{}
	if parentID != nil {
		parent = sql.NullInt64{Int64: *parentID, Valid: true}
	}
	var id int64
	if kind == "problem" {
		id, err = queries.CreateProblemPost(ctx, dbgen.CreateProblemPostParams{DomainID: tenancy.ID(ctx), TargetID: target, UserID: userID, Body: body, ParentID: parent})
	} else {
		id, err = queries.CreateEditorialPost(ctx, dbgen.CreateEditorialPostParams{DomainID: tenancy.ID(ctx), TargetID: target, UserID: userID, Body: body, ParentID: parent})
	}
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, id, userID)
}

func (s *DiscussionRepository) Update(ctx context.Context, id int64, userID, body string) (*domain.DiscussionPost, error) {
	if err := domain.CheckPost("post", userID, body, nil); err != nil {
		return nil, err
	}
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	kind, target, err := postTarget(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	access, err := lockTarget(ctx, tx, kind, target, userID)
	if err != nil {
		return nil, err
	}
	item, err := readPost(ctx, tx, id, access, true)
	if err != nil {
		return nil, err
	}
	if !item.Permissions.Edit {
		return nil, domain.ErrForbidden
	}
	if err := s.queries.WithTx(tx.Tx).UpdateDiscussionPost(ctx, dbgen.UpdateDiscussionPostParams{PostID: id, Body: body}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, id, userID)
}

func (s *DiscussionRepository) Delete(ctx context.Context, id int64, userID string) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	kind, target, err := postTarget(ctx, tx, id)
	if err != nil {
		return err
	}
	access, err := lockTarget(ctx, tx, kind, target, userID)
	if err != nil {
		return err
	}
	item, err := readPost(ctx, tx, id, access, true)
	if err != nil {
		return err
	}
	if !item.Permissions.Delete {
		return domain.ErrForbidden
	}
	if err := s.queries.WithTx(tx.Tx).DeleteDiscussionPost(ctx, id); err != nil {
		return err
	}
	if err := auditContent(ctx, tx, userID, "discussion.delete", fmt.Sprint(id)); err != nil {
		return err
	}
	return tx.Commit()
}
