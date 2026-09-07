package problem

import "github.com/RimuruChan/Vertex/server/internal/domain"

type AccessRole string

const (
	AccessReader AccessRole = "reader"
	AccessEditor AccessRole = "editor"
	AccessOwner  AccessRole = "owner"
)

type Permissions struct {
	View, ReadPackage, Edit, Publish, ManageAccess, Delete, Transfer, Copy bool
}

type Access struct {
	PublishedVersion               int
	Scope                          domain.Scope
	ProblemID, OwnerID, Visibility string
	Role                           AccessRole
	Permissions                    Permissions
}

// EffectivePermissions combines the domain boundary with resource grants.
// A group manager has no extra resource power beyond the group's grant.
func EffectivePermissions(scope domain.Scope, ownerID, visibility string, role AccessRole) Permissions {
	if !scope.CanEnter() {
		return Permissions{}
	}
	readScope := scope
	readScope.Domain.Archived = false
	governor := readScope.Allows(domain.ManageResources)
	owner := scope.ActiveMember() && ownerID == scope.UserID
	collaborator := scope.ActiveMember() && (role == AccessReader || role == AccessEditor)
	packageRead := governor || owner || collaborator
	manage := !scope.Domain.Archived && (governor || owner)
	edit := !scope.Domain.Archived && (manage || (scope.ActiveMember() && role == AccessEditor))
	return Permissions{
		View: visibility == "public" || packageRead, ReadPackage: packageRead,
		Edit: edit, Publish: manage, ManageAccess: manage, Delete: manage, Transfer: manage,
		Copy: packageRead,
	}
}

type AccessGrant struct {
	ID                                   int64
	UserID, Username, GroupID, GroupName *string
	Role                                 AccessRole
}

type GrantInput struct {
	Username, Group string
	Role            AccessRole
}
