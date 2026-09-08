package domain

import (
	problemdomain "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
)

type Permissions struct {
	View, ViewBody, Edit, Delete, Comment, Vote bool
}

func ContentManager(scope tenancydomain.Scope) bool {
	scope.Domain.Archived = false
	return scope.Allows(tenancydomain.ManageResources)
}

func ownsContent(scope tenancydomain.Scope, authorID *string) bool {
	return scope.ActiveMember() && authorID != nil && *authorID == scope.UserID
}

// EditorialPermissions always applies the parent boundary, including to the
// author. Moderation permits removal, never rewriting somebody else's work.
func EditorialPermissions(parent problemdomain.Access, authorID *string, visibility, status string, solvedOnly, solved bool) Permissions {
	scope := parent.Scope
	if !scope.CanEnter() || !parent.Permissions.View {
		return Permissions{}
	}
	author := ownsContent(scope, authorID)
	manager := ContentManager(scope)
	moderator := manager || (scope.ActiveMember() && parent.OwnerID == scope.UserID)
	visible := (status == StatusPublished && visibility == VisibilityPublic) || author || manager
	full := visible && (!solvedOnly || solved || author || moderator)
	write := visible && !scope.Domain.Archived
	participate := full && status == StatusPublished && scope.Allows(tenancydomain.CreateContent)
	return Permissions{View: visible, ViewBody: full, Edit: write && author,
		Delete: write && (author || moderator), Comment: participate, Vote: participate}
}

func PostPermissions(scope tenancydomain.Scope, parentVisible, moderator bool, authorID *string) Permissions {
	if !scope.CanEnter() || !parentVisible {
		return Permissions{}
	}
	write := !scope.Domain.Archived
	author := ownsContent(scope, authorID)
	return Permissions{View: true, ViewBody: true, Edit: write && author,
		Delete: write && (author || moderator), Comment: scope.Allows(tenancydomain.CreateContent)}
}

type Thread struct {
	Posts   []DiscussionPost
	CanPost bool
}
