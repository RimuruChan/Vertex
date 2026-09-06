package problemset

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/problem"
)

type SetStore struct{ db *database.DB }

func NewSetStore(db *database.DB) *SetStore { return &SetStore{db: db} }

// Read parameters are the viewer, fresh domain management and membership, then domain.
// The legacy Admin caller hint is deliberately ignored.
func readArgs(scope domain.Scope) []any {
	readScope := scope
	readScope.Domain.Archived = false
	return []any{scope.UserID, readScope.Allows(domain.ManageResources), scope.ActiveMember(), scope.Domain.ID}
}

var itemAccessCondition = problem.ViewSQL("$1", "$2", "$3")
var setAccessCondition = "(s.visibility='public' OR $2 OR ($3 AND (s.owner_id=NULLIF($1::text,'')::uuid OR " + grantRankSQL() + " > 0)))"
var listColumns = `s.id,s.public_id,s.title,s.description,s.author_id,COALESCE(u.username,''),
 s.visibility,s.created_at,s.updated_at,s.domain_id,s.owner_id,owner.username,` + grantRankSQL() + `,
 NOT EXISTS(SELECT 1 FROM problem_set_problems item JOIN problems p ON p.id=item.problem_id
 WHERE item.set_id=s.id AND NOT ` + itemAccessCondition + `),
 (SELECT count(*) FROM problem_set_problems item JOIN problems p ON p.id=item.problem_id
 WHERE item.set_id=s.id AND ` + itemAccessCondition + `)::int,
 CASE WHEN $1='' THEN 0 ELSE (
 SELECT count(*) FROM problem_set_problems item JOIN problems p ON p.id=item.problem_id
 WHERE item.set_id=s.id AND ` + itemAccessCondition + ` AND EXISTS (
 SELECT 1 FROM submissions sub WHERE sub.user_id=$1::uuid AND sub.problem_id=item.problem_id
 AND sub.contest_id IS NULL AND sub.status='Accepted'))::int END`

const setJoins = ` FROM problem_sets s LEFT JOIN users u ON u.id=s.author_id JOIN users owner ON owner.id=s.owner_id `

func scanSet(scanner interface{ Scan(...any) error }, scope domain.Scope) (Set, error) {
	var item Set
	var rank int
	var allVisible bool
	err := scanner.Scan(&item.ID, &item.PublicID, &item.Title, &item.Description, &item.AuthorID, &item.AuthorName,
		&item.Visibility, &item.CreatedAt, &item.UpdatedAt, &item.DomainID, &item.OwnerID, &item.OwnerName, &rank, &allVisible,
		&item.ProblemCount, &item.SolvedCount)
	item.Permissions = EffectivePermissions(scope, item.OwnerID, item.Visibility, rankRole(rank))
	item.Permissions.EditItems = item.Permissions.EditItems && allVisible
	return item, err
}

func (s *SetStore) List(ctx context.Context, filters Filters) ([]Set, int, error) {
	scope, err := domain.ResourceScope(ctx, s.db.Pool, filters.ViewerID)
	if err != nil {
		return nil, 0, accessError(err)
	}
	args := readArgs(scope)
	clauses := []string{"s.domain_id=$4", setAccessCondition}
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, strings.ReplaceAll(clause, "?", "$"+strconv.Itoa(len(args))))
	}
	if filters.AuthorID != "" {
		add("s.author_id=?::uuid", filters.AuthorID)
	}
	if filters.Keyword != "" {
		add("(s.title ILIKE ? OR s.description ILIKE ?)", "%"+filters.Keyword+"%")
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	var total int
	if err := s.db.Pool.GetContext(ctx, &total, "SELECT count(*) FROM problem_sets s"+where, args...); err != nil {
		return nil, 0, err
	}
	if filters.Limit <= 0 || filters.Limit > 100 {
		filters.Limit = 20
	}
	if filters.Offset < 0 {
		filters.Offset = 0
	}
	args = append(args, filters.Limit, filters.Offset)
	rows, err := s.db.Pool.QueryxContext(ctx, "SELECT "+listColumns+setJoins+where+" ORDER BY s.created_at DESC,s.id DESC LIMIT $"+strconv.Itoa(len(args)-1)+" OFFSET $"+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	list := []Set{}
	for rows.Next() {
		item, err := scanSet(rows, scope)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, item)
	}
	return list, total, rows.Err()
}

func (s *SetStore) Get(ctx context.Context, id, viewerID string, _ bool) (*Set, error) {
	scope, err := domain.ResourceScope(ctx, s.db.Pool, viewerID)
	if err != nil {
		return nil, accessError(err)
	}
	args := append(readArgs(scope), id)
	item, err := scanSet(s.db.Pool.QueryRowxContext(ctx, "SELECT "+listColumns+setJoins+" WHERE s.domain_id=$4 AND s.id=$5 AND "+setAccessCondition, args...), scope)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Pool.QueryxContext(ctx, `SELECT item.problem_id,p.public_id,p.owner_id,item.sort_order,item.note,
 p.title,p.difficulty,p.visibility,p.submission_count,p.accepted_count,
 COALESCE((SELECT jsonb_agg(t.name ORDER BY t.name) FROM problem_tags pt JOIN tags t ON t.id=pt.tag_id WHERE pt.problem_id=p.id),'[]'::jsonb),
 CASE WHEN $1='' THEN 'none'
 WHEN EXISTS(SELECT 1 FROM submissions sub WHERE sub.user_id=$1::uuid AND sub.problem_id=p.id AND sub.contest_id IS NULL AND sub.status='Accepted') THEN 'solved'
 WHEN EXISTS(SELECT 1 FROM submissions sub WHERE sub.user_id=$1::uuid AND sub.problem_id=p.id AND sub.contest_id IS NULL) THEN 'attempted' ELSE 'none' END
 FROM problem_set_problems item JOIN problems p ON p.id=item.problem_id
 WHERE item.domain_id=$4 AND item.set_id=$5 AND `+itemAccessCondition+` ORDER BY item.sort_order`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	item.Items = []Item{}
	for rows.Next() {
		var entry Item
		var tagsJSON []byte
		if err := rows.Scan(&entry.ProblemID, &entry.ProblemPublicID, &entry.OwnerID, &entry.SortOrder, &entry.Note,
			&entry.Title, &entry.Difficulty, &entry.Visibility, &entry.SubmitCount, &entry.AcceptCount, &tagsJSON, &entry.UserStatus); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(tagsJSON, &entry.Tags); err != nil {
			return nil, err
		}
		item.Items = append(item.Items, entry)
	}
	return &item, rows.Err()
}
