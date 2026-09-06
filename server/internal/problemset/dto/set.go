// Package dto maps the problem set domain onto the HTTP wire contract.
package dto

import (
	"time"

	"github.com/RimuruChan/Vertex/server/internal/problemset"
)

type SetItemResponse struct {
	ProblemPublicID string   `json:"problemPublicId"`
	ProblemID       string   `json:"problemId"`
	SortOrder       int      `json:"sortOrder"`
	Note            string   `json:"note"`
	Title           string   `json:"title"`
	Difficulty      int      `json:"difficulty"`
	Visibility      string   `json:"visibility"`
	Tags            []string `json:"tags"`
	SubmitCount     int      `json:"submitCount"`
	AcceptCount     int      `json:"acceptCount"`
	UserStatus      string   `json:"userStatus" enums:"none,attempted,solved"`
}

type SetResponse struct {
	PublicID    string         `json:"publicId"`
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	AuthorID    *string        `json:"authorId,omitempty"`
	AuthorName  string         `json:"authorName"`
	DomainID    string         `json:"domainId"`
	OwnerID     string         `json:"ownerId"`
	OwnerName   string         `json:"ownerName"`
	Permissions SetPermissions `json:"permissions"`
	Visibility  string         `json:"visibility" enums:"public,private"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	// ProblemCount and SolvedCount drive the progress bar; SolvedCount is
	// always zero for anonymous readers.
	ProblemCount int               `json:"problemCount"`
	SolvedCount  int               `json:"solvedCount"`
	Items        []SetItemResponse `json:"items"`
	// CanEdit tells the client whether to show the editing affordances.
	CanEdit bool `json:"canEdit"`
}

type SetUpsertRequest struct {
	Title       string `json:"title" binding:"required"`
	Description string `json:"description,omitempty"`
	Visibility  string `json:"visibility,omitempty" enums:"public,private"`
}

type SetItemRequest struct {
	ProblemID string `json:"problemId" binding:"required"`
	Note      string `json:"note,omitempty"`
}

type SetItemsRequest struct {
	Items []SetItemRequest `json:"items"`
}

func (request SetUpsertRequest) Input() problemset.UpsertInput {
	return problemset.UpsertInput{
		Title: request.Title, Description: request.Description, Visibility: request.Visibility,
	}
}

func (request SetItemsRequest) Input() []problemset.ItemInput {
	items := make([]problemset.ItemInput, 0, len(request.Items))
	for _, entry := range request.Items {
		items = append(items, problemset.ItemInput{ProblemID: entry.ProblemID, Note: entry.Note})
	}
	return items
}

// FromSet projects one set. includeItems keeps list responses small: a listing
// only needs the counts, not every problem in every set.
func FromSet(value problemset.Set, includeItems bool) SetResponse {
	response := SetResponse{
		PublicID: value.PublicID,
		ID:       value.ID, Title: value.Title, Description: value.Description,
		AuthorID: value.AuthorID, AuthorName: value.AuthorName, Visibility: value.Visibility,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
		ProblemCount: value.ProblemCount, SolvedCount: value.SolvedCount,
		CanEdit: value.Permissions.Edit, Items: []SetItemResponse{},
		DomainID: value.DomainID, OwnerID: value.OwnerID, OwnerName: value.OwnerName,
		Permissions: FromPermissions(value.Permissions),
	}
	if !includeItems {
		return response
	}
	for _, entry := range value.Items {
		response.Items = append(response.Items, SetItemResponse{
			ProblemPublicID: entry.ProblemPublicID,
			ProblemID:       entry.ProblemID, SortOrder: entry.SortOrder, Note: entry.Note,
			Title: entry.Title, Difficulty: entry.Difficulty, Visibility: entry.Visibility,
			Tags: entry.Tags, SubmitCount: entry.SubmitCount, AcceptCount: entry.AcceptCount,
			UserStatus: entry.UserStatus,
		})
	}
	return response
}

func FromSets(values []problemset.Set) []SetResponse {
	result := make([]SetResponse, 0, len(values))
	for _, value := range values {
		result = append(result, FromSet(value, false))
	}
	return result
}

type SetPermissions struct {
	View         bool `json:"view"`
	ViewAccess   bool `json:"viewAccess"`
	Edit         bool `json:"edit"`
	EditItems    bool `json:"editItems"`
	Publish      bool `json:"publish"`
	ManageAccess bool `json:"manageAccess"`
	Delete       bool `json:"delete"`
	Transfer     bool `json:"transfer"`
}

func FromPermissions(p problemset.Permissions) SetPermissions {
	return SetPermissions{View: p.View, ViewAccess: p.ViewAccess, Edit: p.Edit, EditItems: p.EditItems, Publish: p.Publish, ManageAccess: p.ManageAccess, Delete: p.Delete, Transfer: p.Transfer}
}

type SetAccessRequest struct {
	Username string `json:"username,omitempty"`
	Group    string `json:"group,omitempty"`
	Role     string `json:"role" binding:"required" enums:"reader,editor"`
}
type SetOwnerRequest struct {
	Username string `json:"username" binding:"required"`
}
type SetAccessResponse struct {
	ID        int64   `json:"id"`
	UserID    *string `json:"userId,omitempty"`
	Username  *string `json:"username,omitempty"`
	GroupID   *string `json:"groupId,omitempty"`
	GroupName *string `json:"groupName,omitempty"`
	Role      string  `json:"role" enums:"reader,editor"`
}

func FromGrants(grants []problemset.AccessGrant) []SetAccessResponse {
	items := make([]SetAccessResponse, 0, len(grants))
	for _, g := range grants {
		items = append(items, SetAccessResponse{ID: g.ID, UserID: g.UserID, Username: g.Username, GroupID: g.GroupID, GroupName: g.GroupName, Role: string(g.Role)})
	}
	return items
}
