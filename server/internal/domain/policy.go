package domain

import "slices"

type Permission string

const (
	ManageSettings   Permission = "domain.settings.manage"
	ManageMembers    Permission = "domain.members.manage"
	ManageRoles      Permission = "domain.roles.manage"
	ManageGroups     Permission = "domain.groups.manage"
	ManageResources  Permission = "domain.resources.manage"
	CreateProblem    Permission = "problem.create"
	CreateContest    Permission = "contest.create"
	CreateProblemSet Permission = "problem_set.create"
	CreateSubmission Permission = "submission.create"
	CreateContent    Permission = "content.create"
)

type PermissionInfo struct {
	Key   Permission
	Label string
}

var catalog = []PermissionInfo{
	{ManageSettings, "管理域设置"}, {ManageMembers, "管理成员"}, {ManageRoles, "管理域角色"},
	{ManageGroups, "管理群组"}, {ManageResources, "管理全域资源"}, {CreateProblem, "创建题目"},
	{CreateContest, "创建比赛"}, {CreateProblemSet, "创建题单"}, {CreateSubmission, "提交程序"}, {CreateContent, "发布题解和讨论"},
}

func PermissionCatalog() []PermissionInfo { return slices.Clone(catalog) }
func knownPermission(permission Permission) bool {
	return slices.ContainsFunc(catalog, func(item PermissionInfo) bool { return item.Key == permission })
}
func allPermissions() []Permission {
	result := make([]Permission, 0, len(catalog))
	for _, item := range catalog {
		result = append(result, item.Key)
	}
	return result
}

func BuiltinRoles() []Role {
	return []Role{
		{Key: "admin", Name: "域管理员", Permissions: allPermissions(), Builtin: true},
		{Key: "author", Name: "出题人", Permissions: []Permission{CreateProblem, CreateContest, CreateProblemSet, CreateSubmission, CreateContent}, Builtin: true},
		{Key: "member", Name: "成员", Permissions: []Permission{CreateProblemSet, CreateSubmission, CreateContent}, Builtin: true},
		{Key: "viewer", Name: "只读成员", Permissions: []Permission{}, Builtin: true},
	}
}

func (s Scope) IsOwner() bool {
	return s.UserID != "" && s.Domain.OwnerID != nil && *s.Domain.OwnerID == s.UserID && s.ActiveMember()
}
func (s Scope) ActiveMember() bool { return s.UserID != "" && s.MemberStatus == "active" }
func (s Scope) CanDiscover() bool {
	return s.SiteAdmin || s.Domain.Visibility == "public" || s.ActiveMember() || s.MemberStatus == "invited" || s.MemberStatus == "pending"
}
func (s Scope) CanEnter() bool {
	if s.SiteAdmin {
		return true
	}
	if s.MemberStatus == "suspended" {
		return false
	}
	return s.Domain.Visibility == "public" || s.ActiveMember()
}
func (s Scope) CanGovernOwnership() bool { return !s.Domain.Official && (s.SiteAdmin || s.IsOwner()) }
func (s Scope) Allows(permission Permission) bool {
	if !knownPermission(permission) || s.Domain.Archived {
		return false
	}
	if s.SiteAdmin {
		return true
	}
	if !s.ActiveMember() {
		return false
	}
	return s.IsOwner() || slices.Contains(s.RolePermissions, permission)
}
func (s Scope) Permissions() []Permission {
	result := []Permission{}
	for _, item := range catalog {
		if s.Allows(item.Key) {
			result = append(result, item.Key)
		}
	}
	return result
}
func (s Scope) CanManageGroup(group Group) bool {
	if group.DomainID != s.Domain.ID || s.Domain.Archived {
		return false
	}
	return s.Allows(ManageGroups) || (s.ActiveMember() && (group.OwnerID == s.UserID || group.ViewerRole == "manager"))
}

func validateDelegation(scope Scope, permissions []Permission) error {
	for _, permission := range permissions {
		if !knownPermission(permission) {
			return invalid("未知或站点级权限不能写入域角色")
		}
		if !scope.Allows(permission) {
			return ErrForbidden
		}
	}
	return nil
}
