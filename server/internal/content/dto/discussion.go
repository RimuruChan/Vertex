package dto

import (
	"time"

	"github.com/RimuruChan/Vertex/server/internal/content"
)

type DiscussionResponse struct {
	DomainID    string             `json:"domainId"`
	Permissions ContentPermissions `json:"permissions"`
	ID          int64              `json:"id"`
	ProblemID   *string            `json:"problemId,omitempty"`
	EditorialID *string            `json:"editorialId,omitempty"`
	AuthorID    *string            `json:"authorId,omitempty"`
	AuthorName  string             `json:"authorName,omitempty"`
	ContentMD   string             `json:"contentMd"`
	ParentID    *int64             `json:"parentId,omitempty"`
	CreatedAt   time.Time          `json:"createdAt"`
	UpdatedAt   time.Time          `json:"updatedAt"`
	// Edited is true when the post was changed after it was written.
	Edited bool `json:"edited"`
}

type ProblemDiscussionCreateRequest struct {
	ContentMD string `json:"contentMd" binding:"required"`
	ParentID  *int64 `json:"parentId,omitempty"`
}

type EditorialDiscussionCreateRequest struct {
	ParentID  *int64 `json:"parentId,omitempty"`
	ContentMD string `json:"contentMd" binding:"required"`
}

type DiscussionUpdateRequest struct {
	ContentMD string `json:"contentMd" binding:"required"`
}

func FromDiscussion(post content.DiscussionPost) DiscussionResponse {
	return DiscussionResponse{
		ID: post.ID, ProblemID: post.ProblemID, EditorialID: post.EditorialID,
		DomainID: post.DomainID, Permissions: FromPermissions(post.Permissions),
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

type DiscussionThreadResponse struct {
	Items   []DiscussionResponse `json:"items"`
	Total   int                  `json:"total"`
	CanPost bool                 `json:"canPost"`
}

func FromThread(thread content.Thread) DiscussionThreadResponse {
	return DiscussionThreadResponse{Items: FromDiscussions(thread.Posts), Total: len(thread.Posts), CanPost: thread.CanPost}
}
