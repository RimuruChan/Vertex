package dto

import (
	"time"

	tenancydomain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
)

type GroupResponse struct {
	ID          string    `json:"id"`
	PublicID    string    `json:"publicId"`
	DomainID    string    `json:"domainId"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	OwnerID     string    `json:"ownerId"`
	OwnerName   string    `json:"ownerName"`
	MemberCount int       `json:"memberCount"`
	ViewerRole  string    `json:"viewerRole"`
	CanManage   bool      `json:"canManage"`
	CanTransfer bool      `json:"canTransfer"`
	CanDelete   bool      `json:"canDelete"`
	CreatedAt   time.Time `json:"createdAt"`
}
type GroupRequest struct {
	Name          string `json:"name" binding:"required"`
	Description   string `json:"description,omitempty"`
	OwnerUsername string `json:"ownerUsername,omitempty"`
}
type GroupMemberRequest struct {
	Role string `json:"role" binding:"required" enums:"member,manager"`
}
type GroupMemberResponse struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

func FromGroup(g tenancydomain.Group, s tenancydomain.Scope) GroupResponse {
	govern := s.CanManageGroup(g) && (s.Allows(tenancydomain.ManageGroups) || g.OwnerID == s.UserID)
	return GroupResponse{
		ID: g.ID, PublicID: g.PublicID, DomainID: g.DomainID,
		Name: g.Name, Description: g.Description, OwnerID: g.OwnerID, OwnerName: g.OwnerName,
		MemberCount: g.MemberCount, ViewerRole: g.ViewerRole,
		CanManage: s.CanManageGroup(g), CanTransfer: govern, CanDelete: govern, CreatedAt: g.CreatedAt,
	}
}
func FromGroups(groups []tenancydomain.Group, s tenancydomain.Scope) []GroupResponse {
	result := make([]GroupResponse, 0, len(groups))
	for _, g := range groups {
		result = append(result, FromGroup(g, s))
	}
	return result
}
func FromGroupMembers(members []tenancydomain.GroupMember) []GroupMemberResponse {
	result := make([]GroupMemberResponse, 0, len(members))
	for _, m := range members {
		result = append(result, GroupMemberResponse{UserID: m.UserID, Username: m.Username, Role: m.Role})
	}
	return result
}
