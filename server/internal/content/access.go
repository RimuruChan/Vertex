package content

import (
	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/problem"
)

type Permissions struct {
	View, ViewBody, Edit, Delete, Comment, Vote bool
}

func contentManager(scope domain.Scope) bool {
	scope.Domain.Archived = false
	return scope.Allows(domain.ManageResources)
}

func ownsContent(scope domain.Scope, authorID *string) bool {
	return scope.ActiveMember() && authorID != nil && *authorID == scope.UserID
}

// EditorialPermissions always applies the parent boundary, including to the
// author. Moderation permits removal, never rewriting somebody else's work.
func EditorialPermissions(parent problem.Access, authorID *string, visibility, status string, solvedOnly, solved bool) Permissions {
	scope := parent.Scope
	if !scope.CanEnter() || !parent.Permissions.View {
		return Permissions{}
	}
	author := ownsContent(scope, authorID)
	manager := contentManager(scope)
	moderator := manager || (scope.ActiveMember() && parent.OwnerID == scope.UserID)
	visible := (status == StatusPublished && visibility == VisibilityPublic) || author || manager
	full := visible && (!solvedOnly || solved || author || moderator)
	write := visible && !scope.Domain.Archived
	participate := full && status == StatusPublished && scope.Allows(domain.CreateContent)
	return Permissions{View: visible, ViewBody: full, Edit: write && author,
		Delete: write && (author || moderator), Comment: participate, Vote: participate}
}

func PostPermissions(scope domain.Scope, parentVisible, moderator bool, authorID *string) Permissions {
	if !scope.CanEnter() || !parentVisible {
		return Permissions{}
	}
	write := !scope.Domain.Archived
	author := ownsContent(scope, authorID)
	return Permissions{View: true, ViewBody: true, Edit: write && author,
		Delete: write && (author || moderator), Comment: scope.Allows(domain.CreateContent)}
}

type Thread struct {
	Posts   []DiscussionPost
	CanPost bool
}
