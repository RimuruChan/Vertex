package content

import (
	"context"
	"strconv"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/problem"
)

type EditorialStore struct{ db *database.DB }

func NewEditorialStore(db *database.DB) *EditorialStore { return &EditorialStore{db: db} }

const editorialVoteColumn = `EXISTS(SELECT 1 FROM editorial_votes v WHERE v.editorial_id=e.id AND v.user_id=NULLIF($1::text,'')::uuid)`
const editorialSolvedColumn = `EXISTS(SELECT 1 FROM submissions sub WHERE sub.domain_id=e.domain_id AND sub.problem_id=e.problem_id AND sub.user_id=NULLIF($1::text,'')::uuid AND sub.contest_id IS NULL AND sub.status='Accepted')`
const editorialAuthorSQL = `($3 AND e.author_id=NULLIF($1::text,'')::uuid)`
const editorialModeratorSQL = `($2 OR ($3 AND p.owner_id=NULLIF($1::text,'')::uuid))`

var editorialVisibilitySQL = problem.ViewSQL("$1", "$2", "$3") + ` AND ((e.visibility='public' AND e.status='published') OR $2 OR ` + editorialAuthorSQL + `)`

const editorialFullSQL = `(NOT e.solved_only OR ` + editorialAuthorSQL + ` OR ` + editorialModeratorSQL + ` OR ` + editorialSolvedColumn + `)`
const editorialMetadata = `e.id,e.public_id,e.problem_id,p.public_id,p.title,e.author_id,COALESCE(u.username,''),e.title,e.visibility,e.status,e.solved_only,e.vote_count,` + editorialVoteColumn + `,e.created_at,e.updated_at,e.domain_id,p.owner_id,` + editorialSolvedColumn
const editorialJoins = ` FROM editorials e JOIN problems p ON p.id=e.problem_id LEFT JOIN users u ON u.id=e.author_id `

func permissions(scope domain.Scope, ownerID string, authorID *string, visibility, status string, solvedOnly, solved bool) Permissions {
	return EditorialPermissions(problem.Access{Scope: scope, OwnerID: ownerID, Permissions: problem.Permissions{View: true}}, authorID, visibility, status, solvedOnly, solved)
}

func scanEditorial(scanner interface{ Scan(...any) error }, scope domain.Scope) (Editorial, error) {
	var item Editorial
	var ownerID string
	var solved bool
	err := scanner.Scan(&item.ID, &item.PublicID, &item.ProblemID, &item.ProblemPublicID, &item.ProblemTitle, &item.AuthorID, &item.AuthorName,
		&item.Title, &item.Visibility, &item.Status, &item.SolvedOnly, &item.VoteCount, &item.Voted, &item.CreatedAt, &item.UpdatedAt, &item.DomainID, &ownerID, &solved, &item.ContentMD)
	item.Permissions = permissions(scope, ownerID, item.AuthorID, item.Visibility, item.Status, item.SolvedOnly, solved)
	item.Locked = item.Permissions.View && !item.Permissions.ViewBody
	return item, err
}

func scanEditorialSummary(scanner interface{ Scan(...any) error }, scope domain.Scope) (EditorialSummary, error) {
	var item EditorialSummary
	var ownerID string
	var solved bool
	err := scanner.Scan(&item.ID, &item.PublicID, &item.ProblemID, &item.ProblemPublicID, &item.ProblemTitle, &item.AuthorID, &item.AuthorName,
		&item.Title, &item.Visibility, &item.Status, &item.SolvedOnly, &item.VoteCount, &item.Voted, &item.CreatedAt, &item.UpdatedAt, &item.DomainID, &ownerID, &solved)
	item.Permissions = permissions(scope, ownerID, item.AuthorID, item.Visibility, item.Status, item.SolvedOnly, solved)
	item.Locked = item.Permissions.View && !item.Permissions.ViewBody
	return item, err
}

func (s *EditorialStore) List(ctx context.Context, filters EditorialFilters) ([]EditorialSummary, int, error) {
	scope, err := domain.ResourceScope(ctx, s.db.Pool, filters.ViewerID)
	if err != nil {
		return nil, 0, accessError(err)
	}
	args := readArgs(scope)
	clauses := []string{"e.domain_id=$4", editorialVisibilitySQL}
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, strings.ReplaceAll(clause, "?", "$"+strconv.Itoa(len(args))))
	}
	if filters.ProblemID != "" {
		add("e.problem_id=?::uuid", filters.ProblemID)
	}
	if filters.AuthorID != "" {
		add("e.author_id=?::uuid", filters.AuthorID)
	}
	if filters.Keyword != "" {
		add("(e.title ILIKE ? OR p.title ILIKE ?)", "%"+filters.Keyword+"%")
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	var total int
	if err := s.db.Pool.GetContext(ctx, &total, "SELECT count(*)"+editorialJoins+where, args...); err != nil {
		return nil, 0, err
	}
	if filters.Limit <= 0 || filters.Limit > 100 {
		filters.Limit = 20
	}
	if filters.Offset < 0 {
		filters.Offset = 0
	}
	order := "e.created_at DESC,e.id DESC"
	if filters.Sort == "votes" {
		order = "e.vote_count DESC," + order
	}
	args = append(args, filters.Limit, filters.Offset)
	// Body-free projection; spoiler state is computed in the same query.
	rows, err := s.db.Pool.QueryxContext(ctx, "SELECT "+editorialMetadata+editorialJoins+where+" ORDER BY "+order+" LIMIT $"+strconv.Itoa(len(args)-1)+" OFFSET $"+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []EditorialSummary{}
	for rows.Next() {
		item, err := scanEditorialSummary(rows, scope)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func readEditorial(ctx context.Context, q rowReader, scope domain.Scope, id string, lock bool) (*Editorial, error) {
	query := "SELECT " + editorialMetadata + ",CASE WHEN " + editorialFullSQL + " THEN e.content_md ELSE '' END" + editorialJoins + " WHERE e.domain_id=$4 AND e.id=$5 AND " + editorialVisibilitySQL
	if lock {
		query += " FOR UPDATE OF e"
	}
	args := append(readArgs(scope), id)
	item, err := scanEditorial(q.QueryRowxContext(ctx, query, args...), scope)
	if err != nil {
		return nil, accessError(err)
	}
	return &item, nil
}

func (s *EditorialStore) Get(ctx context.Context, id, viewerID string) (*Editorial, error) {
	scope, err := domain.ResourceScope(ctx, s.db.Pool, viewerID)
	if err != nil {
		return nil, accessError(err)
	}
	return readEditorial(ctx, s.db.Pool, scope, id, false)
}

func (s *EditorialStore) HasSolved(ctx context.Context, problemID, userID string) (bool, error) {
	var solved bool
	err := s.db.Pool.GetContext(ctx, &solved, `SELECT EXISTS(SELECT 1 FROM submissions WHERE domain_id=$3 AND problem_id=$2 AND user_id=NULLIF($1::text,'')::uuid AND contest_id IS NULL AND status='Accepted')`, userID, problemID, domain.ID(ctx))
	return solved, err
}
