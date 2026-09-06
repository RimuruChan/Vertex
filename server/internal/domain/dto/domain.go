package dto

import (
	"time"

	"github.com/RimuruChan/Vertex/server/internal/domain"
)

type DomainResponse struct {
	ID           string              `json:"id"`
	Slug         string              `json:"slug"`
	Name         string              `json:"name"`
	Description  string              `json:"description"`
	OwnerID      *string             `json:"ownerId,omitempty"`
	OwnerName    string              `json:"ownerName"`
	Official     bool                `json:"official"`
	Visibility   string              `json:"visibility" enums:"public,private"`
	JoinPolicy   string              `json:"joinPolicy" enums:"open,approval,invite"`
	Archived     bool                `json:"archived"`
	MemberRole   string              `json:"memberRole"`
	MemberStatus string              `json:"memberStatus"`
	Permissions  []domain.Permission `json:"permissions"`
	CanEnter     bool                `json:"canEnter"`
	CanTransfer  bool                `json:"canTransfer"`
	CreatedAt    time.Time           `json:"createdAt"`
}
type CreateDomainRequest struct {
	Slug        string `json:"slug" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description,omitempty"`
	Visibility  string `json:"visibility,omitempty" enums:"public,private"`
	JoinPolicy  string `json:"joinPolicy,omitempty" enums:"open,approval,invite"`
}
type UpdateDomainRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Visibility  string `json:"visibility" binding:"required" enums:"public,private"`
	JoinPolicy  string `json:"joinPolicy" binding:"required" enums:"open,approval,invite"`
}
type ArchiveRequest struct {
	Archived bool `json:"archived"`
}
type OwnerRequest struct {
	Username string `json:"username" binding:"required"`
}
type MemberRequest struct {
	RoleKey string `json:"roleKey" binding:"required"`
	Status  string `json:"status" binding:"required" enums:"active,pending,invited,suspended"`
}
type MemberResponse struct {
	UserID   string    `json:"userId"`
	Username string    `json:"username"`
	RoleKey  string    `json:"roleKey"`
	Status   string    `json:"status"`
	JoinedAt time.Time `json:"joinedAt"`
}
type RoleResponse struct {
	Key         string              `json:"key"`
	Name        string              `json:"name"`
	Permissions []domain.Permission `json:"permissions"`
	Builtin     bool                `json:"builtin"`
}
type RoleRequest struct {
	Name        string              `json:"name" binding:"required"`
	Permissions []domain.Permission `json:"permissions"`
}
type PermissionResponse struct {
	Key   domain.Permission `json:"key"`
	Label string            `json:"label"`
}

func FromDomain(scope domain.Scope) DomainResponse {
	d := scope.Domain
	return DomainResponse{
		ID: d.ID, Slug: d.Slug, Name: d.Name, Description: d.Description,
		OwnerID: d.OwnerID, OwnerName: d.OwnerName, Official: d.Official,
		Visibility: d.Visibility, JoinPolicy: d.JoinPolicy, Archived: d.Archived,
		MemberRole: scope.MemberRole, MemberStatus: scope.MemberStatus,
		Permissions: scope.Permissions(), CanEnter: scope.CanEnter(),
		CanTransfer: scope.CanGovernOwnership() && !d.Archived, CreatedAt: d.CreatedAt,
	}
}
func FromDomains(scopes []domain.Scope) []DomainResponse {
	result := make([]DomainResponse, 0, len(scopes))
	for _, s := range scopes {
		result = append(result, FromDomain(s))
	}
	return result
}
func FromMembers(members []domain.Member) []MemberResponse {
	result := make([]MemberResponse, 0, len(members))
	for _, m := range members {
		result = append(result, MemberResponse{UserID: m.UserID, Username: m.Username, RoleKey: m.RoleKey, Status: m.Status, JoinedAt: m.JoinedAt})
	}
	return result
}
func FromRoles(roles []domain.Role) []RoleResponse {
	result := make([]RoleResponse, 0, len(roles))
	for _, r := range roles {
		result = append(result, RoleResponse{Key: r.Key, Name: r.Name, Permissions: r.Permissions, Builtin: r.Builtin})
	}
	return result
}
