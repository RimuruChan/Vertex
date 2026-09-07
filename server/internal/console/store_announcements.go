package console

import (
	"context"
	"database/sql"
	"errors"

	"github.com/RimuruChan/Vertex/server/internal/domain"
)

const announcementColumns = `a.id,a.public_id::text,a.title,a.content_md,a.pinned,a.published,COALESCE(u.username,''),a.created_at,a.updated_at`

func scanAnnouncement(scanner interface{ Scan(...any) error }) (Announcement, error) {
	var item Announcement
	err := scanner.Scan(&item.ID, &item.PublicID, &item.Title, &item.ContentMD, &item.Pinned, &item.Published, &item.AuthorName, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (s *ConsoleStore) ListAnnouncements(ctx context.Context, publishedOnly bool, limit int) ([]Announcement, error) {
	items, _, err := s.AnnouncementPage(ctx, publishedOnly, AnnouncementFilters{Limit: limit})
	return items, err
}

func (s *ConsoleStore) AnnouncementPage(ctx context.Context, publishedOnly bool, f AnnouncementFilters) ([]Announcement, int, error) {
	if _, err := domain.ResourceScope(ctx, s.db.Pool, domain.ActorID(ctx)); err != nil {
		return nil, 0, err
	}
	if !publishedOnly {
		if err := s.RequireResourceManagement(ctx, false); err != nil {
			return nil, 0, err
		}
	}
	const where = ` WHERE a.domain_id=$1 AND (NOT $2 OR a.published) AND ($3='' OR strpos(lower(a.title),lower($3))>0 OR a.public_id::text=$3)`
	var total int
	if err := s.db.Pool.QueryRowxContext(ctx, `SELECT count(*) FROM announcements a`+where, domain.ID(ctx), publishedOnly, f.Keyword).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Pool.QueryxContext(ctx, `SELECT `+announcementColumns+` FROM announcements a LEFT JOIN users u ON u.id=a.created_by`+where+` ORDER BY a.pinned DESC,a.created_at DESC,a.id DESC LIMIT $4 OFFSET $5`, domain.ID(ctx), publishedOnly, f.Keyword, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []Announcement{}
	for rows.Next() {
		item, err := scanAnnouncement(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *ConsoleStore) Announcement(ctx context.Context, id string, manage bool) (*Announcement, error) {
	if _, err := domain.ResourceScope(ctx, s.db.Pool, domain.ActorID(ctx)); err != nil {
		return nil, err
	}
	if manage {
		if err := s.RequireResourceManagement(ctx, false); err != nil {
			return nil, err
		}
	}
	return announcementFrom(ctx, s.db.Pool, id, !manage)
}

func announcementFrom(ctx context.Context, q resourceQueryer, id string, publishedOnly bool) (*Announcement, error) {
	item, err := scanAnnouncement(q.QueryRowxContext(ctx, `SELECT `+announcementColumns+` FROM announcements a LEFT JOIN users u ON u.id=a.created_by WHERE a.id=$1 AND a.domain_id=$2 AND (NOT $3 OR a.published)`, id, domain.ID(ctx), publishedOnly))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &item, err
}

func (s *ConsoleStore) CreateAnnouncement(ctx context.Context, authorID string, input AnnouncementInput) (*Announcement, error) {
	if authorID != domain.ActorID(ctx) {
		return nil, domain.ErrForbidden
	}
	tx, err := s.resourceTransaction(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var id string
	if err := tx.QueryRowxContext(ctx, `INSERT INTO announcements(domain_id,created_by,title,content_md,pinned,published) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, domain.ID(ctx), authorID, input.Title, input.ContentMD, input.Pinned, input.Published).Scan(&id); err != nil {
		return nil, err
	}
	if err := recordResourceChange(ctx, tx, "announcement.create", id); err != nil {
		return nil, err
	}
	item, err := announcementFrom(ctx, tx, id, false)
	if err != nil {
		return nil, err
	}
	return item, tx.Commit()
}

func (s *ConsoleStore) UpdateAnnouncement(ctx context.Context, id string, input AnnouncementInput) (*Announcement, error) {
	tx, err := s.resourceTransaction(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE announcements SET title=$2,content_md=$3,pinned=$4,published=$5,updated_at=now() WHERE id=$1 AND domain_id=$6`, id, input.Title, input.ContentMD, input.Pinned, input.Published, domain.ID(ctx))
	if err != nil {
		return nil, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, ErrNotFound
	}
	if err := recordResourceChange(ctx, tx, "announcement.update", id); err != nil {
		return nil, err
	}
	item, err := announcementFrom(ctx, tx, id, false)
	if err != nil {
		return nil, err
	}
	return item, tx.Commit()
}

func (s *ConsoleStore) DeleteAnnouncement(ctx context.Context, id string) error {
	tx, err := s.resourceTransaction(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `DELETE FROM announcements WHERE id=$1 AND domain_id=$2`, id, domain.ID(ctx))
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	if err := recordResourceChange(ctx, tx, "announcement.delete", id); err != nil {
		return err
	}
	return tx.Commit()
}
