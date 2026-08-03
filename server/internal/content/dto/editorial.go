package dto

import (
	"time"

	"github.com/RimuruChan/Vertex/server/internal/content"
)

type EditorialResponse struct {
	ID         string    `json:"id"`
	ProblemID  string    `json:"problemId"`
	AuthorID   *string   `json:"authorId,omitempty"`
	AuthorName string    `json:"authorName,omitempty"`
	Title      string    `json:"title"`
	ContentMD  string    `json:"contentMd"`
	Visibility string    `json:"visibility"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type EditorialCreateRequest struct {
	ProblemID string `json:"problemId" binding:"required"`
	Title     string `json:"title" binding:"required"`
	ContentMD string `json:"contentMd" binding:"required"`
}

func FromEditorial(editorial content.Editorial) EditorialResponse {
	return EditorialResponse{
		ID: editorial.ID, ProblemID: editorial.ProblemID, AuthorID: editorial.AuthorID,
		AuthorName: editorial.AuthorName, Title: editorial.Title, ContentMD: editorial.ContentMD,
		Visibility: editorial.Visibility, Status: editorial.Status,
		CreatedAt: editorial.CreatedAt, UpdatedAt: editorial.UpdatedAt,
	}
}

func FromEditorials(editorials []content.Editorial) []EditorialResponse {
	result := make([]EditorialResponse, 0, len(editorials))
	for _, editorial := range editorials {
		result = append(result, FromEditorial(editorial))
	}
	return result
}
