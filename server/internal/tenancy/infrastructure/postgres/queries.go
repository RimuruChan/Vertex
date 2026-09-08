package postgres

import (
	"context"
	"encoding/json"
	"strconv"

	domain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
	"github.com/RimuruChan/Vertex/server/internal/tenancy/infrastructure/postgres/internal/dbgen"
)

func readRole(ctx context.Context, queries *dbgen.Queries, domainID, key string) (domain.Role, error) {
	row, err := queries.GetDomainRole(ctx, dbgen.GetDomainRoleParams{DomainID: domainID, Key: key})
	if err != nil {
		return domain.Role{}, notFound(err)
	}
	return roleFromRow(row)
}

func roleFromRow(row dbgen.GetDomainRoleRow) (domain.Role, error) {
	role := domain.Role{Key: row.Key, Name: row.Name, Builtin: row.Builtin}
	if err := json.Unmarshal(row.Permissions, &role.Permissions); err != nil {
		return domain.Role{}, err
	}
	return role, nil
}

func (r *Repository) Roles(ctx context.Context, domainID string) ([]domain.Role, error) {
	rows, err := r.queries.ListDomainRoles(ctx, domainID)
	if err != nil {
		return nil, err
	}
	roles := make([]domain.Role, 0, len(rows))
	for _, row := range rows {
		role, err := roleFromRow(dbgen.GetDomainRoleRow(row))
		if err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, nil
}

func (r *Repository) Members(ctx context.Context, domainID string, filter domain.Filters) ([]domain.Member, int, error) {
	f := filter.Normalized()
	keyword := "%" + f.Keyword + "%"
	total, err := r.queries.CountDomainMembers(ctx, dbgen.CountDomainMembersParams{DomainID: domainID, Keyword: keyword})
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListDomainMembers(ctx, dbgen.ListDomainMembersParams{DomainID: domainID, Keyword: keyword, PageLimit: f.Limit, PageOffset: f.Offset})
	if err != nil {
		return nil, 0, err
	}
	members := make([]domain.Member, 0, len(rows))
	for _, row := range rows {
		members = append(members, domain.Member{UserID: row.UserID, Username: row.Username, RoleKey: row.RoleKey, Status: row.Status, JoinedAt: row.JoinedAt})
	}
	return members, int(total), nil
}

func groupFromRow(row dbgen.GetDomainGroupByIDRow) domain.Group {
	return domain.Group{ID: row.ID, PublicID: row.PublicID, DomainID: row.DomainID,
		Name: row.Name, Description: row.Description, OwnerID: row.OwnerID, OwnerName: row.OwnerName,
		CreatedAt: row.CreatedAt, MemberCount: row.MemberCount, ViewerRole: row.ViewerRole}
}

func readGroup(ctx context.Context, queries *dbgen.Queries, scope domain.Scope, ref string) (domain.Group, error) {
	if number, err := strconv.ParseInt(ref, 10, 64); err == nil {
		row, err := queries.GetDomainGroupByNumber(ctx, dbgen.GetDomainGroupByNumberParams{DomainID: scope.Domain.ID, ViewerID: scope.UserID, PublicID: number})
		if err != nil {
			return domain.Group{}, notFound(err)
		}
		return groupFromRow(dbgen.GetDomainGroupByIDRow(row)), nil
	}
	row, err := queries.GetDomainGroupByID(ctx, dbgen.GetDomainGroupByIDParams{DomainID: scope.Domain.ID, ViewerID: scope.UserID, GroupID: ref})
	if err != nil {
		return domain.Group{}, notFound(err)
	}
	return groupFromRow(row), nil
}

func (r *Repository) Group(ctx context.Context, scope domain.Scope, ref string) (domain.Group, error) {
	return readGroup(ctx, r.queries, scope, ref)
}

func (r *Repository) Groups(ctx context.Context, scope domain.Scope, filter domain.Filters) ([]domain.Group, int, error) {
	f := filter.Normalized()
	keyword := "%" + f.Keyword + "%"
	total, err := r.queries.CountDomainGroups(ctx, dbgen.CountDomainGroupsParams{DomainID: scope.Domain.ID, Keyword: keyword})
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListDomainGroups(ctx, dbgen.ListDomainGroupsParams{DomainID: scope.Domain.ID, ViewerID: scope.UserID, Keyword: keyword, PageLimit: f.Limit, PageOffset: f.Offset})
	if err != nil {
		return nil, 0, err
	}
	groups := make([]domain.Group, 0, len(rows))
	for _, row := range rows {
		groups = append(groups, groupFromRow(dbgen.GetDomainGroupByIDRow(row)))
	}
	return groups, int(total), nil
}

func (r *Repository) GroupMembers(ctx context.Context, domainID, groupID string, filter domain.Filters) ([]domain.GroupMember, int, error) {
	f := filter.Normalized()
	keyword := "%" + f.Keyword + "%"
	total, err := r.queries.CountGroupMembers(ctx, dbgen.CountGroupMembersParams{DomainID: domainID, GroupID: groupID, Keyword: keyword})
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListGroupMembers(ctx, dbgen.ListGroupMembersParams{DomainID: domainID, GroupID: groupID, Keyword: keyword, PageLimit: f.Limit, PageOffset: f.Offset})
	if err != nil {
		return nil, 0, err
	}
	members := make([]domain.GroupMember, 0, len(rows))
	for _, row := range rows {
		members = append(members, domain.GroupMember{UserID: row.UserID, Username: row.Username, Role: row.Role})
	}
	return members, int(total), nil
}
