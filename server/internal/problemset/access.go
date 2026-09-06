package problemset

import "github.com/RimuruChan/Vertex/server/internal/domain"

type AccessRole string

const (
	AccessReader AccessRole = "reader"
	AccessEditor AccessRole = "editor"
)

type Permissions struct {
	View, ViewAccess, Edit, EditItems, Publish, ManageAccess, Delete, Transfer bool
}

type Access struct {
	Scope                      domain.Scope
	SetID, OwnerID, Visibility string
	Role                       AccessRole
	Permissions                Permissions
}

func EffectivePermissions(scope domain.Scope, ownerID, visibility string, role AccessRole) Permissions {
	if !scope.CanEnter() {
		return Permissions{}
	}
	readScope := scope
	readScope.Domain.Archived = false
	manager := readScope.Allows(domain.ManageResources)
	owner := scope.ActiveMember() && scope.UserID == ownerID
	member := scope.ActiveMember() && (role == AccessReader || role == AccessEditor)
	manage := !scope.Domain.Archived && (owner || manager)
	edit := manage || (!scope.Domain.Archived && scope.ActiveMember() && role == AccessEditor)
	return Permissions{View: visibility == VisibilityPublic || owner || manager || member, ViewAccess: owner || manager || member,
		Edit: edit, EditItems: edit, Publish: manage, ManageAccess: manage, Delete: manage, Transfer: manage}
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
