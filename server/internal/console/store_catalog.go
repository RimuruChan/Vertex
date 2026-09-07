package console

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/domain"
)

func (s *ConsoleStore) ListTags(ctx context.Context) ([]Tag, error) {
	if err := s.RequireResourceManagement(ctx, false); err != nil {
		return nil, err
	}
	rows, err := s.db.Pool.QueryContext(ctx, `SELECT t.id,t.name,(SELECT count(*) FROM problem_tags pt WHERE pt.tag_id=t.id AND pt.domain_id=t.domain_id)::int FROM tags t WHERE t.domain_id=$1 ORDER BY t.name`, domain.ID(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Tag{}
	for rows.Next() {
		var item Tag
		if err := rows.Scan(&item.ID, &item.Name, &item.ProblemCount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *ConsoleStore) CreateTag(ctx context.Context, name string) (*Tag, error) {
	tx, err := s.resourceTransaction(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := domain.ResourceGuard(ctx, tx, "tag-catalog", domain.ID(ctx), true); err != nil {
		return nil, err
	}
	var id int64
	if err := tx.QueryRowxContext(ctx, `INSERT INTO tags(domain_id,name) VALUES($1,$2) ON CONFLICT(domain_id,name) DO UPDATE SET name=excluded.name RETURNING id`, domain.ID(ctx), name).Scan(&id); err != nil {
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

func catalogTag(ctx context.Context, tx resourceQueryer, id int64) (*Tag, error) {
	var item Tag
	err := tx.QueryRowxContext(ctx, `SELECT t.id,t.name,(SELECT count(*) FROM problem_tags pt WHERE pt.tag_id=t.id AND pt.domain_id=t.domain_id)::int FROM tags t WHERE t.id=$1 AND t.domain_id=$2`, id, domain.ID(ctx)).Scan(&item.ID, &item.Name, &item.ProblemCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &item, err
}

func (s *ConsoleStore) Tag(ctx context.Context, id int64) (*Tag, error) {
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

func (s *ConsoleStore) RenameTag(ctx context.Context, id int64, name string) (*Tag, error) {
	return s.changeTag(ctx, id, tagChange{name: name})
}
func (s *ConsoleStore) MergeTags(ctx context.Context, source, target int64) (*Tag, error) {
	return s.changeTag(ctx, source, tagChange{target: target})
}
func (s *ConsoleStore) DeleteTag(ctx context.Context, id int64) error {
	_, err := s.changeTag(ctx, id, tagChange{remove: true})
	return err
}

func (s *ConsoleStore) changeTag(ctx context.Context, id int64, change tagChange) (*Tag, error) {
	tx, err := s.resourceTransaction(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := domain.ResourceGuard(ctx, tx, "tag-catalog", domain.ID(ctx), true); err != nil {
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
		err = tx.QueryRowxContext(ctx, `SELECT id FROM tags WHERE domain_id=$1 AND name=$2`, domain.ID(ctx), name).Scan(&change.target)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	if change.target == id || (!change.remove && name == source.Name) {
		return source, tx.Commit()
	}
	// Snapshot tags in old releases remain immutable. Update only working labels
	// and bump their metadata revision so an already-reviewed publish is rejected.
	_, err = tx.ExecContext(ctx, `WITH changed AS (
 UPDATE problem_workspaces w SET tags_json=(
  SELECT COALESCE(jsonb_agg(value ORDER BY position),'[]'::jsonb) FROM (
   SELECT CASE WHEN value=$2 THEN $3 ELSE value END AS value,min(ordinality) AS position
   FROM jsonb_array_elements_text(w.tags_json) WITH ORDINALITY
   WHERE NOT($4 AND value=$2) GROUP BY CASE WHEN value=$2 THEN $3 ELSE value END
  ) normalized), updated_at=now()
 FROM problems p WHERE p.id=w.problem_id AND p.domain_id=$1 AND w.tags_json ? $2 RETURNING w.problem_id)
 UPDATE problems SET package_revision=package_revision+1,updated_at=now() WHERE id IN(SELECT problem_id FROM changed)`, domain.ID(ctx), source.Name, name, change.remove)
	if err != nil {
		return nil, err
	}
	targetID := id
	action := "tag.rename"
	if change.target != 0 {
		action = "tag.merge"
		targetID = change.target
		_, err = tx.ExecContext(ctx, `INSERT INTO problem_tags(domain_id,problem_id,tag_id) SELECT domain_id,problem_id,$2 FROM problem_tags WHERE tag_id=$1 AND domain_id=$3 ON CONFLICT(problem_id,tag_id) DO NOTHING`, id, targetID, domain.ID(ctx))
		if err != nil {
			return nil, err
		}
	}
	if change.remove || change.target != 0 {
		if change.remove {
			action = "tag.delete"
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM tags WHERE id=$1 AND domain_id=$2`, id, domain.ID(ctx))
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE tags SET name=$2 WHERE id=$1 AND domain_id=$3`, id, name, domain.ID(ctx))
	}
	if err != nil {
		return nil, err
	}
	if err := recordResourceChange(ctx, tx, action, strconv.FormatInt(id, 10)); err != nil {
		return nil, err
	}
	var item *Tag
	if !change.remove {
		item, err = catalogTag(ctx, tx, targetID)
		if err != nil {
			return nil, err
		}
	}
	return item, tx.Commit()
}
