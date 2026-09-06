package domain

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
)

type Store struct{ db *database.DB }

func NewStore(db *database.DB) *Store { return &Store{db: db} }

type queryer interface {
	QueryRowxContext(context.Context, string, ...any) *sqlx.Row
	QueryxContext(context.Context, string, ...any) (*sqlx.Rows, error)
}
type scanner interface{ Scan(...any) error }
type subject struct {
	ID, Username string
	Admin        bool
}

func loadSubject(ctx context.Context, q queryer, userID string, lock bool) (subject, error) {
	if userID == "" {
		return subject{}, nil
	}
	query := "SELECT id, username, role = 'admin' FROM users WHERE id = $1 AND disabled_at IS NULL"
	if lock {
		query += " FOR SHARE"
	}
	var value subject
	err := q.QueryRowxContext(ctx, query, userID).Scan(&value.ID, &value.Username, &value.Admin)
	if errors.Is(err, sql.ErrNoRows) {
		return subject{}, ErrUnauthenticated
	}
	return value, err
}

const scopeColumns = `d.id,d.slug,d.name,d.description,d.owner_id,COALESCE(owner.username,''),
    d.is_official,d.visibility,d.join_policy,d.archived,d.created_at,d.updated_at,
    COALESCE(m.role_key,''),COALESCE(m.status,''),COALESCE(r.permissions,'[]'::jsonb)`
const scopeJoins = `FROM domains d LEFT JOIN users owner ON owner.id=d.owner_id
    LEFT JOIN domain_members m ON m.domain_id=d.id AND m.user_id=NULLIF($1,'')::uuid
    LEFT JOIN domain_roles r ON r.domain_id=m.domain_id AND r.key=m.role_key`

func scanScope(row scanner, actor subject) (Scope, error) {
	var value Scope
	var permissions []byte
	d := &value.Domain
	err := row.Scan(&d.ID, &d.Slug, &d.Name, &d.Description, &d.OwnerID, &d.OwnerName, &d.Official,
		&d.Visibility, &d.JoinPolicy, &d.Archived, &d.CreatedAt, &d.UpdatedAt,
		&value.MemberRole, &value.MemberStatus, &permissions)
	if errors.Is(err, sql.ErrNoRows) {
		return Scope{}, ErrNotFound
	}
	if err != nil {
		return Scope{}, err
	}
	if err = json.Unmarshal(permissions, &value.RolePermissions); err != nil {
		return Scope{}, err
	}
	value.UserID, value.SiteAdmin = actor.ID, actor.Admin
	return value, nil
}

func readScope(ctx context.Context, q queryer, slug string, actor subject) (Scope, error) {
	return scanScope(q.QueryRowxContext(ctx, "SELECT "+scopeColumns+" "+scopeJoins+" WHERE d.slug=$2", actor.ID, slug), actor)
}

func (s *Store) Scope(ctx context.Context, slug, userID string) (Scope, error) {
	actor, err := loadSubject(ctx, s.db.Pool, userID, false)
	if err != nil {
		return Scope{}, err
	}
	return readScope(ctx, s.db.Pool, slug, actor)
}

func (s *Store) List(ctx context.Context, userID string, filter Filters) ([]Scope, int, error) {
	actor, err := loadSubject(ctx, s.db.Pool, userID, false)
	if err != nil {
		return nil, 0, err
	}
	f := filter.normalized()
	condition := `WHERE ($2 OR d.visibility='public' OR m.status IN ('active','invited','pending'))
        AND (NOT d.archived OR $2 OR m.status='active') AND (d.name ILIKE $3 OR d.slug ILIKE $3)`
	var total int
	err = s.db.Pool.QueryRowxContext(ctx, "SELECT count(*) "+scopeJoins+" "+condition, actor.ID, actor.Admin, "%"+f.Keyword+"%").Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Pool.QueryxContext(ctx, "SELECT "+scopeColumns+" "+scopeJoins+" "+condition+" ORDER BY d.is_official DESC,d.created_at,d.id LIMIT $4 OFFSET $5", actor.ID, actor.Admin, "%"+f.Keyword+"%", f.Limit, f.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := []Scope{}
	for rows.Next() {
		value, err := scanScope(rows, actor)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, value)
	}
	return result, total, rows.Err()
}

// session serializes domain administration while reloading the actor and role
// under the same transaction. Resource writes will use a shared domain lock.
type session struct {
	tx    *sqlx.Tx
	ctx   context.Context
	scope Scope
	actor subject
}

func (s *Store) write(ctx context.Context, slug, userID string, fn func(*session) error) error {
	if userID == "" {
		return ErrUnauthenticated
	}
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	actor, err := loadSubject(ctx, tx, userID, true)
	if err != nil {
		return err
	}
	var domainID string
	err = tx.QueryRowxContext(ctx, "SELECT id FROM domains WHERE slug=$1 FOR UPDATE", slug).Scan(&domainID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	scope, err := readScope(ctx, tx, slug, actor)
	if err != nil {
		return err
	}
	if !scope.CanDiscover() {
		return ErrNotFound
	}
	if err = fn(&session{tx: tx, ctx: ctx, scope: scope, actor: actor}); err != nil {
		return normalizeError(err)
	}
	return normalizeError(tx.Commit())
}

func (tx *session) audit(action, target string) error {
	_, err := tx.tx.ExecContext(tx.ctx, "INSERT INTO domain_audit_events(domain_id,actor_id,action,target) VALUES($1,$2,$3,$4)", tx.scope.Domain.ID, tx.actor.ID, action, target)
	return err
}

func normalizeError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505", "23503":
			return ErrConflict
		case "23514":
			return ErrInvalid
		}
	}
	return err
}

func seedRoles(ctx context.Context, tx *sqlx.Tx, domainID string) error {
	for _, role := range BuiltinRoles() {
		permissions, err := json.Marshal(role.Permissions)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO domain_roles(domain_id,key,name,permissions,builtin) VALUES($1,$2,$3,$4,TRUE) ON CONFLICT(domain_id,key) DO NOTHING", domainID, role.Key, role.Name, permissions)
		if err != nil {
			return err
		}
	}
	return nil
}

// EnsureOfficial is also used by isolated integration fixtures after truncation.
func (s *Store) EnsureOfficial(ctx context.Context) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, "INSERT INTO domains(id,slug,name,is_official,visibility,join_policy) VALUES($1,'official','官方',TRUE,'public','open') ON CONFLICT(id) DO NOTHING", OfficialID)
	if err != nil {
		return err
	}
	if err = seedRoles(ctx, tx, OfficialID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) create(ctx context.Context, userID string, input CreateInput) (Scope, error) {
	if userID == "" {
		return Scope{}, ErrUnauthenticated
	}
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return Scope{}, err
	}
	defer tx.Rollback()
	actor, err := loadSubject(ctx, tx, userID, true)
	if err != nil {
		return Scope{}, err
	}
	var id string
	err = tx.QueryRowxContext(ctx, `INSERT INTO domains(slug,name,description,owner_id,visibility,join_policy)
        VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, input.Slug, input.Name, input.Description, actor.ID, input.Visibility, input.JoinPolicy).Scan(&id)
	if err != nil {
		return Scope{}, normalizeError(err)
	}
	if err = seedRoles(ctx, tx, id); err != nil {
		return Scope{}, err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO domain_members(domain_id,user_id,role_key,status) VALUES($1,$2,'admin','active')", id, actor.ID)
	if err != nil {
		return Scope{}, err
	}
	scope, err := readScope(ctx, tx, input.Slug, actor)
	if err != nil {
		return Scope{}, err
	}
	operation := session{tx: tx, ctx: ctx, scope: scope, actor: actor}
	if err = operation.audit("domain.created", id); err != nil {
		return Scope{}, err
	}
	if err = tx.Commit(); err != nil {
		return Scope{}, normalizeError(err)
	}
	return scope, nil
}

func readRole(ctx context.Context, q queryer, domainID, key string) (Role, error) {
	var value Role
	var permissions []byte
	err := q.QueryRowxContext(ctx, "SELECT key,name,permissions,builtin FROM domain_roles WHERE domain_id=$1 AND key=$2", domainID, key).Scan(&value.Key, &value.Name, &permissions, &value.Builtin)
	if errors.Is(err, sql.ErrNoRows) {
		return Role{}, ErrNotFound
	}
	if err != nil {
		return Role{}, err
	}
	err = json.Unmarshal(permissions, &value.Permissions)
	return value, err
}

func (s *Store) roles(ctx context.Context, domainID string) ([]Role, error) {
	rows, err := s.db.Pool.QueryxContext(ctx, "SELECT key,name,permissions,builtin FROM domain_roles WHERE domain_id=$1 ORDER BY builtin DESC,key", domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Role{}
	for rows.Next() {
		var r Role
		var p []byte
		if err := rows.Scan(&r.Key, &r.Name, &p, &r.Builtin); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(p, &r.Permissions); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (tx *session) member(username string) (Member, error) {
	return tx.lookupMember(username, true)
}

func (tx *session) lookupMember(username string, activeAccountOnly bool) (Member, error) {
	var value Member
	err := tx.tx.QueryRowxContext(tx.ctx, `SELECT u.id,u.username,COALESCE(m.role_key,''),COALESCE(m.status,''),COALESCE(m.joined_at,u.created_at)
        FROM users u LEFT JOIN domain_members m ON m.user_id=u.id AND m.domain_id=$1
        WHERE u.username=$2 AND (NOT $3 OR u.disabled_at IS NULL) FOR SHARE OF u`, tx.scope.Domain.ID, username, activeAccountOnly).Scan(&value.UserID, &value.Username, &value.RoleKey, &value.Status, &value.JoinedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Member{}, ErrNotFound
	}
	return value, err
}

func (tx *session) saveMember(member Member) error {
	_, err := tx.tx.ExecContext(tx.ctx, `INSERT INTO domain_members(domain_id,user_id,role_key,status) VALUES($1,$2,$3,$4)
        ON CONFLICT(domain_id,user_id) DO UPDATE SET role_key=EXCLUDED.role_key,status=EXCLUDED.status,updated_at=now()`, tx.scope.Domain.ID, member.UserID, member.RoleKey, member.Status)
	return err
}

func (s *Store) members(ctx context.Context, domainID string, filter Filters) ([]Member, int, error) {
	f := filter.normalized()
	pattern := "%" + f.Keyword + "%"
	var total int
	err := s.db.Pool.QueryRowxContext(ctx, "SELECT count(*) FROM domain_members m JOIN users u ON u.id=m.user_id WHERE m.domain_id=$1 AND u.username ILIKE $2", domainID, pattern).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Pool.QueryxContext(ctx, `SELECT m.user_id,u.username,m.role_key,m.status,m.joined_at FROM domain_members m JOIN users u ON u.id=m.user_id
        WHERE m.domain_id=$1 AND u.username ILIKE $2 ORDER BY m.joined_at,m.user_id LIMIT $3 OFFSET $4`, domainID, pattern, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := []Member{}
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.Username, &m.RoleKey, &m.Status, &m.JoinedAt); err != nil {
			return nil, 0, err
		}
		result = append(result, m)
	}
	return result, total, rows.Err()
}

const groupColumns = `g.id,g.public_id,g.domain_id,g.name,g.description,g.owner_id,u.username,g.created_at,
    (SELECT count(*) FROM domain_group_members allgm JOIN domain_members dm ON dm.domain_id=allgm.domain_id AND dm.user_id=allgm.user_id WHERE allgm.group_id=g.id AND dm.status='active'),COALESCE(gm.role,'')`
const groupJoins = `FROM domain_groups g JOIN users u ON u.id=g.owner_id
    LEFT JOIN domain_group_members gm ON gm.group_id=g.id AND gm.user_id=NULLIF($2,'')::uuid`

func scanGroup(row scanner) (Group, error) {
	var g Group
	err := row.Scan(&g.ID, &g.PublicID, &g.DomainID, &g.Name, &g.Description, &g.OwnerID, &g.OwnerName, &g.CreatedAt, &g.MemberCount, &g.ViewerRole)
	if errors.Is(err, sql.ErrNoRows) {
		return Group{}, ErrNotFound
	}
	return g, err
}

func readGroup(ctx context.Context, q queryer, scope Scope, ref string) (Group, error) {
	field := "g.id"
	var arg any = ref
	if value, err := strconv.ParseInt(ref, 10, 64); err == nil {
		field = "g.public_id"
		arg = value
	}
	return scanGroup(q.QueryRowxContext(ctx, "SELECT "+groupColumns+" "+groupJoins+" WHERE g.domain_id=$1 AND "+field+"=$3", scope.Domain.ID, scope.UserID, arg))
}

func (s *Store) groups(ctx context.Context, scope Scope, filter Filters) ([]Group, int, error) {
	f := filter.normalized()
	pattern := "%" + f.Keyword + "%"
	var total int
	err := s.db.Pool.QueryRowxContext(ctx, "SELECT count(*) FROM domain_groups WHERE domain_id=$1 AND name ILIKE $2", scope.Domain.ID, pattern).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Pool.QueryxContext(ctx, "SELECT "+groupColumns+" "+groupJoins+" WHERE g.domain_id=$1 AND g.name ILIKE $3 ORDER BY g.name,g.id LIMIT $4 OFFSET $5", scope.Domain.ID, scope.UserID, pattern, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := []Group{}
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, g)
	}
	return result, total, rows.Err()
}

func (s *Store) groupMembers(ctx context.Context, domainID, groupID string, filter Filters) ([]GroupMember, int, error) {
	f := filter.normalized()
	var total int
	joins := `FROM domain_group_members gm JOIN domain_members dm ON dm.domain_id=gm.domain_id AND dm.user_id=gm.user_id
        JOIN users u ON u.id=gm.user_id WHERE gm.domain_id=$1 AND gm.group_id=$2 AND dm.status='active' AND u.username ILIKE $3`
	if err := s.db.Pool.QueryRowxContext(ctx, "SELECT count(*) "+joins, domainID, groupID, "%"+f.Keyword+"%").Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Pool.QueryxContext(ctx, `SELECT gm.user_id,u.username,gm.role FROM domain_group_members gm
        JOIN domain_members dm ON dm.domain_id=gm.domain_id AND dm.user_id=gm.user_id
        JOIN users u ON u.id=gm.user_id WHERE gm.domain_id=$1 AND gm.group_id=$2 AND dm.status='active' AND u.username ILIKE $3 ORDER BY u.username LIMIT $4 OFFSET $5`, domainID, groupID, "%"+f.Keyword+"%", f.Limit, f.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := []GroupMember{}
	for rows.Next() {
		var m GroupMember
		if err := rows.Scan(&m.UserID, &m.Username, &m.Role); err != nil {
			return nil, 0, err
		}
		result = append(result, m)
	}
	return result, total, rows.Err()
}
