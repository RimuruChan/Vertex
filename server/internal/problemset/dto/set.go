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
	PublicID    string    `json:"publicId"`
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	AuthorID    *string   `json:"authorId,omitempty"`
	AuthorName  string    `json:"authorName"`
	Visibility  string    `json:"visibility" enums:"public,private"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
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
func FromSet(value problemset.Set, canEdit, includeItems bool) SetResponse {
	response := SetResponse{
		PublicID: value.PublicID,
		ID:       value.ID, Title: value.Title, Description: value.Description,
		AuthorID: value.AuthorID, AuthorName: value.AuthorName, Visibility: value.Visibility,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
		ProblemCount: value.ProblemCount, SolvedCount: value.SolvedCount,
		CanEdit: canEdit, Items: []SetItemResponse{},
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

func FromSets(values []problemset.Set, viewerID string, admin bool) []SetResponse {
	result := make([]SetResponse, 0, len(values))
	for _, value := range values {
		result = append(result, FromSet(value, value.CanEdit(viewerID, admin), false))
	}
	return result
}
