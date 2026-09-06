package dto

import (
	"time"

	"github.com/RimuruChan/Vertex/server/internal/content"
)

type DiscussionResponse struct {
	ID          int64     `json:"id"`
	ProblemID   *string   `json:"problemId,omitempty"`
	EditorialID *string   `json:"editorialId,omitempty"`
	ContestID   *string   `json:"contestId,omitempty"`
	AuthorID    *string   `json:"authorId,omitempty"`
	AuthorName  string    `json:"authorName,omitempty"`
	ContentMD   string    `json:"contentMd"`
	ParentID    *int64    `json:"parentId,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	// Edited is true when the post was changed after it was written.
	Edited bool `json:"edited"`
}

type ProblemDiscussionCreateRequest struct {
	ContentMD string `json:"contentMd" binding:"required"`
	ParentID  *int64 `json:"parentId,omitempty"`
}

type EditorialDiscussionCreateRequest struct {
	ContentMD string `json:"contentMd" binding:"required"`
}

func FromDiscussion(post content.DiscussionPost) DiscussionResponse {
	return DiscussionResponse{
		ID: post.ID, ProblemID: post.ProblemID, EditorialID: post.EditorialID, ContestID: post.ContestID,
		AuthorID: post.AuthorID, AuthorName: post.AuthorName, ContentMD: post.ContentMD,
		ParentID: post.ParentID, CreatedAt: post.CreatedAt, UpdatedAt: post.UpdatedAt,
		Edited: post.Edited(),
	}
}

func FromDiscussions(posts []content.DiscussionPost) []DiscussionResponse {
	result := make([]DiscussionResponse, 0, len(posts))
	for _, post := range posts {
		result = append(result, FromDiscussion(post))
	}
	return result
}
