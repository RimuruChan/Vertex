package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"

	consoledomain "github.com/RimuruChan/Vertex/server/internal/console/domain"
	"github.com/RimuruChan/Vertex/server/internal/console/infrastructure/postgres/internal/dbgen"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/tenancy/infrastructure/postgres"
)

func (s *Repository) ListTags(ctx context.Context) ([]consoledomain.Tag, error) {
	if err := s.RequireResourceManagement(ctx, false); err != nil {
		return nil, err
	}
	rows, err := dbgen.New(s.db.Pool).ListTagCatalog(ctx, tenancydomain.ID(ctx))
	if err != nil {
		return nil, err
	}

	items := []consoledomain.Tag{}
	for _, record := range rows {
		var item consoledomain.Tag
		item.ID = record.ID
		item.Name = record.Name
		item.ProblemCount = record.ProblemCount
		items = append(items, item)
	}
	return items, nil
}

func (s *Repository) CreateTag(ctx context.Context, name string) (*consoledomain.Tag, error) {
	tx, err := s.resourceTransaction(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := tenancypg.ResourceGuard(ctx, tx, "tag-catalog", tenancydomain.ID(ctx), true); err != nil {
		return nil, err
	}
	id, err := s.queries.WithTx(tx.Tx).UpsertTag(ctx, dbgen.UpsertTagParams{DomainID: tenancydomain.ID(ctx), Name: name})
	if err != nil {
		return nil, err
	}

	item, err := catalogTag(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err := recordResourceChange(ctx, tx, "tag.create", strconv.FormatInt(id, 10)); err != nil {
		return nil, err
	}
	return item, tx.Commit()
}

func catalogTag(ctx context.Context, db resourceQueryer, id int64) (*consoledomain.Tag, error) {
	row, err := dbgen.New(db).GetTagCatalogEntry(ctx, dbgen.GetTagCatalogEntryParams{TagID: id, DomainID: tenancydomain.ID(ctx)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, consoledomain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &consoledomain.Tag{ID: row.ID, Name: row.Name, ProblemCount: row.ProblemCount}, nil
}

func (s *Repository) Tag(ctx context.Context, id int64) (*consoledomain.Tag, error) {
	if err := s.RequireResourceManagement(ctx, false); err != nil {
		return nil, err
	}
	return catalogTag(ctx, s.db.Pool, id)
}

type tagChange struct {
	name   string
	target int64
	remove bool
}

func (s *Repository) RenameTag(ctx context.Context, id int64, name string) (*consoledomain.Tag, error) {
	return s.changeTag(ctx, id, tagChange{name: name})
}
func (s *Repository) MergeTags(ctx context.Context, source, target int64) (*consoledomain.Tag, error) {
	return s.changeTag(ctx, source, tagChange{target: target})
}
func (s *Repository) DeleteTag(ctx context.Context, id int64) error {
	_, err := s.changeTag(ctx, id, tagChange{remove: true})
	return err
}

func (s *Repository) changeTag(ctx context.Context, id int64, change tagChange) (*consoledomain.Tag, error) {
	tx, err := s.resourceTransaction(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := tenancypg.ResourceGuard(ctx, tx, "tag-catalog", tenancydomain.ID(ctx), true); err != nil {
		return nil, err
	}
	source, err := catalogTag(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	name := change.name
	if change.target != 0 {
		target, err := catalogTag(ctx, tx, change.target)
		if err != nil {
			return nil, err
		}
		name = target.Name
	} else if !change.remove {
		change.target, err = s.queries.WithTx(tx.Tx).FindTagByName(ctx, dbgen.FindTagByNameParams{DomainID: tenancydomain.ID(ctx), Name: name})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	if change.target == id || (!change.remove && name == source.Name) {
		return source, tx.Commit()
	}
	// Snapshot tags in old releases remain immutable. Update only working labels
	// and bump their metadata revision so an already-reviewed publish is rejected.
	queries := s.queries.WithTx(tx.Tx)
	workspaces, err := queries.LockTaggedWorkspaces(ctx, dbgen.LockTaggedWorkspacesParams{DomainID: tenancydomain.ID(ctx), OldName: source.Name})
	if err != nil {
		return nil, err
	}
	for _, workspace := range workspaces {
		var tags []string
		if err := json.Unmarshal(workspace.TagsJson, &tags); err != nil {
			return nil, err
		}
		normalized, err := json.Marshal(consoledomain.TransformTags(tags, source.Name, name, change.remove))
		if err != nil {
			return nil, err
		}
		if err := queries.UpdateWorkspaceTags(ctx, dbgen.UpdateWorkspaceTagsParams{ProblemID: workspace.ProblemID, Tags: normalized}); err != nil {
			return nil, err
		}
	}

	targetID := id
	action := "tag.rename"
	if change.target != 0 {
		action = "tag.merge"
		targetID = change.target
		err = queries.MergeProblemTags(ctx, dbgen.MergeProblemTagsParams{SourceID: id, TargetID: targetID, DomainID: tenancydomain.ID(ctx)})
		if err != nil {
			return nil, err
		}
	}
	if change.remove || change.target != 0 {
		if change.remove {
			action = "tag.delete"
		}
		err = queries.DeleteTag(ctx, dbgen.DeleteTagParams{TagID: id, DomainID: tenancydomain.ID(ctx)})
	} else {
		err = queries.RenameTag(ctx, dbgen.RenameTagParams{TagID: id, Name: name, DomainID: tenancydomain.ID(ctx)})
	}
	if err != nil {
		return nil, err
	}
	if err := recordResourceChange(ctx, tx, action, strconv.FormatInt(id, 10)); err != nil {
		return nil, err
	}
	var item *consoledomain.Tag
	if !change.remove {
		item, err = catalogTag(ctx, tx, targetID)
		if err != nil {
			return nil, err
		}
	}
	return item, tx.Commit()
}
