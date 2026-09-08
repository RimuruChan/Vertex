package dto

import problemdomain "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"

type ProblemPermissions struct {
	View         bool `json:"view"`
	ReadPackage  bool `json:"readPackage"`
	Edit         bool `json:"edit"`
	Publish      bool `json:"publish"`
	ManageAccess bool `json:"manageAccess"`
	Delete       bool `json:"delete"`
	Transfer     bool `json:"transfer"`
	Copy         bool `json:"copy"`
}

func PermissionsFromDomain(p problemdomain.Permissions) ProblemPermissions {
	return ProblemPermissions{View: p.View, ReadPackage: p.ReadPackage, Edit: p.Edit, Publish: p.Publish, ManageAccess: p.ManageAccess, Delete: p.Delete, Transfer: p.Transfer, Copy: p.Copy}
}

type ProblemGrantRequest struct {
	Username string `json:"username,omitempty"`
	Group    string `json:"group,omitempty"`
	Role     string `json:"role" binding:"required" enums:"reader,editor"`
}

type ProblemGrantResponse struct {
	ID        int64   `json:"id"`
	UserID    *string `json:"userId,omitempty"`
	Username  *string `json:"username,omitempty"`
	GroupID   *string `json:"groupId,omitempty"`
	GroupName *string `json:"groupName,omitempty"`
	Role      string  `json:"role" enums:"reader,editor"`
}

type ProblemOwnerRequest struct {
	Username string `json:"username" binding:"required"`
}

func GrantsFromDomain(grants []problemdomain.AccessGrant) []ProblemGrantResponse {
	result := make([]ProblemGrantResponse, 0, len(grants))
	for _, grant := range grants {
		result = append(result, ProblemGrantResponse{ID: grant.ID, UserID: grant.UserID, Username: grant.Username, GroupID: grant.GroupID, GroupName: grant.GroupName, Role: string(grant.Role)})
	}
	return result
}
