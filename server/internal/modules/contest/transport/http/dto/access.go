package dto

import contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"

type ContestPermissions struct {
	View            bool `json:"view"`
	Edit            bool `json:"edit"`
	ManageAccess    bool `json:"manageAccess"`
	Delete          bool `json:"delete"`
	Transfer        bool `json:"transfer"`
	PreviewProblems bool `json:"previewProblems"`
	ViewJury        bool `json:"viewJury"`
	Rejudge         bool `json:"rejudge"`
	Reply           bool `json:"reply"`
	Eligible        bool `json:"eligible"`
	Register        bool `json:"register"`
	Submit          bool `json:"submit"`
}

func PermissionsFromDomain(p contestdomain.Permissions) ContestPermissions {
	return ContestPermissions{View: p.View, Edit: p.Edit, ManageAccess: p.ManageAccess, Delete: p.Delete, Transfer: p.Transfer, PreviewProblems: p.PreviewProblems, ViewJury: p.ViewJury, Rejudge: p.Rejudge, Reply: p.Reply, Eligible: p.Eligible, Register: p.Register, Submit: p.Submit}
}

type ContestGrantRequest struct {
	Username string `json:"username,omitempty"`
	Group    string `json:"group,omitempty"`
	Role     string `json:"role" binding:"required" enums:"editor,jury,observer,participant"`
}

type ContestGrantResponse struct {
	ID        int64   `json:"id"`
	UserID    *string `json:"userId,omitempty"`
	Username  *string `json:"username,omitempty"`
	GroupID   *string `json:"groupId,omitempty"`
	GroupName *string `json:"groupName,omitempty"`
	Role      string  `json:"role" enums:"editor,jury,observer,participant"`
}

type ContestOwnerRequest struct {
	Username string `json:"username" binding:"required"`
}

func GrantsFromDomain(grants []contestdomain.AccessGrant) []ContestGrantResponse {
	result := make([]ContestGrantResponse, 0, len(grants))
	for _, g := range grants {
		result = append(result, ContestGrantResponse{ID: g.ID, UserID: g.UserID, Username: g.Username, GroupID: g.GroupID, GroupName: g.GroupName, Role: g.Role})
	}
	return result
}
