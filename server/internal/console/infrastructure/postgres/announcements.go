package postgres

import (
	"context"
	"database/sql"
	"errors"

	domain "github.com/RimuruChan/Vertex/server/internal/console/domain"
	"github.com/RimuruChan/Vertex/server/internal/console/infrastructure/postgres/internal/dbgen"
	tenancy "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/tenancy/infrastructure/postgres"
)

func announcementFromRow(row dbgen.GetAnnouncementRow) domain.Announcement {
	return domain.Announcement{ID: row.ID, PublicID: row.PublicID, Title: row.Title, ContentMD: row.ContentMd, Pinned: row.Pinned, Published: row.Published, AuthorName: row.AuthorName, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}
func announcementFrom(ctx context.Context, db dbgen.DBTX, id string, publishedOnly bool) (*domain.Announcement, error) {
	row, err := dbgen.New(db).GetAnnouncement(ctx, dbgen.GetAnnouncementParams{AnnouncementID: id, DomainID: tenancy.ID(ctx), PublishedOnly: publishedOnly})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	item := announcementFromRow(row)
	return &item, nil
}
func (s *Repository) ListAnnouncements(ctx context.Context, publishedOnly bool, limit int) ([]domain.Announcement, error) {
	items, _, err := s.AnnouncementPage(ctx, publishedOnly, domain.AnnouncementFilters{Limit: limit})
	return items, err
}
func (s *Repository) AnnouncementPage(ctx context.Context, publishedOnly bool, f domain.AnnouncementFilters) ([]domain.Announcement, int, error) {
	if _, err := tenancypg.ResourceScope(ctx, s.db.Pool, tenancy.ActorID(ctx)); err != nil {
		return nil, 0, err
	}
	if !publishedOnly {
		if err := s.RequireResourceManagement(ctx, false); err != nil {
			return nil, 0, err
		}
	}
	total, err := s.queries.CountAnnouncements(ctx, dbgen.CountAnnouncementsParams{DomainID: tenancy.ID(ctx), PublishedOnly: publishedOnly, Keyword: f.Keyword})
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.queries.ListAnnouncements(ctx, dbgen.ListAnnouncementsParams{DomainID: tenancy.ID(ctx), PublishedOnly: publishedOnly, Keyword: f.Keyword, PageLimit: f.Limit, PageOffset: f.Offset})
	if err != nil {
		return nil, 0, err
	}
	items := make([]domain.Announcement, 0, len(rows))
	for _, row := range rows {
		items = append(items, announcementFromRow(dbgen.GetAnnouncementRow(row)))
	}
	return items, int(total), nil
}
func (s *Repository) Announcement(ctx context.Context, id string, manage bool) (*domain.Announcement, error) {
	if _, err := tenancypg.ResourceScope(ctx, s.db.Pool, tenancy.ActorID(ctx)); err != nil {
		return nil, err
	}
	if manage {
		if err := s.RequireResourceManagement(ctx, false); err != nil {
			return nil, err
		}
	}
	return announcementFrom(ctx, s.db.Pool, id, !manage)
}

func (s *Repository) CreateAnnouncement(ctx context.Context, authorID string, input domain.AnnouncementInput) (*domain.Announcement, error) {
	if authorID != tenancy.ActorID(ctx) {
		return nil, tenancy.ErrForbidden
	}
	tx, err := s.resourceTransaction(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	id, err := s.queries.WithTx(tx.Tx).CreateAnnouncement(ctx, dbgen.CreateAnnouncementParams{DomainID: tenancy.ID(ctx), UserID: authorID, Title: input.Title, Body: input.ContentMD, Pinned: input.Pinned, Published: input.Published})
	if err != nil {
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

func (s *Repository) UpdateAnnouncement(ctx context.Context, id string, input domain.AnnouncementInput) (*domain.Announcement, error) {
	tx, err := s.resourceTransaction(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	count, err := s.queries.WithTx(tx.Tx).UpdateAnnouncement(ctx, dbgen.UpdateAnnouncementParams{AnnouncementID: id, Title: input.Title, Body: input.ContentMD, Pinned: input.Pinned, Published: input.Published, DomainID: tenancy.ID(ctx)})
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, domain.ErrNotFound
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

func (s *Repository) DeleteAnnouncement(ctx context.Context, id string) error {
	tx, err := s.resourceTransaction(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	count, err := s.queries.WithTx(tx.Tx).DeleteAnnouncement(ctx, dbgen.DeleteAnnouncementParams{AnnouncementID: id, DomainID: tenancy.ID(ctx)})
	if err != nil {
		return err
	}
	if count == 0 {
		return domain.ErrNotFound
	}
	if err := recordResourceChange(ctx, tx, "announcement.delete", id); err != nil {
		return err
	}
	return tx.Commit()
}
