package dto

import (
	"time"

	"github.com/RimuruChan/Vertex/server/internal/content"
)

type EditorialResponse struct {
	PublicID        string  `json:"publicId"`
	ProblemPublicID string  `json:"problemPublicId"`
	ID              string  `json:"id"`
	ProblemID       string  `json:"problemId"`
	ProblemTitle    string  `json:"problemTitle,omitempty"`
	AuthorID        *string `json:"authorId,omitempty"`
	AuthorName      string  `json:"authorName,omitempty"`
	Title           string  `json:"title"`
	ContentMD       string  `json:"contentMd"`
	Visibility      string  `json:"visibility" enums:"public,private"`
	Status          string  `json:"status" enums:"draft,published"`
	SolvedOnly      bool    `json:"solvedOnly"`
	VoteCount       int     `json:"voteCount"`
	Voted           bool    `json:"voted"`
	// Locked is true when the body was withheld because the reader has not
	// solved the problem yet.
	Locked    bool      `json:"locked"`
	CanEdit   bool      `json:"canEdit"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type EditorialSummaryResponse struct {
	PublicID        string    `json:"publicId"`
	ProblemPublicID string    `json:"problemPublicId"`
	ID              string    `json:"id"`
	ProblemID       string    `json:"problemId"`
	ProblemTitle    string    `json:"problemTitle,omitempty"`
	AuthorID        *string   `json:"authorId,omitempty"`
	AuthorName      string    `json:"authorName,omitempty"`
	Title           string    `json:"title"`
	Visibility      string    `json:"visibility" enums:"public,private"`
	Status          string    `json:"status" enums:"draft,published"`
	SolvedOnly      bool      `json:"solvedOnly"`
	VoteCount       int       `json:"voteCount"`
	Voted           bool      `json:"voted"`
	Locked          bool      `json:"locked"`
	CanEdit         bool      `json:"canEdit"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type EditorialCreateRequest struct {
	ProblemID  string `json:"problemId" binding:"required"`
	Title      string `json:"title" binding:"required"`
	ContentMD  string `json:"contentMd" binding:"required"`
	Visibility string `json:"visibility,omitempty" enums:"public,private"`
	Status     string `json:"status,omitempty" enums:"draft,published"`
	SolvedOnly bool   `json:"solvedOnly,omitempty"`
}

type EditorialUpdateRequest struct {
	Title      string `json:"title" binding:"required"`
	ContentMD  string `json:"contentMd" binding:"required"`
	Visibility string `json:"visibility,omitempty" enums:"public,private"`
	Status     string `json:"status,omitempty" enums:"draft,published"`
	SolvedOnly bool   `json:"solvedOnly,omitempty"`
}

type EditorialVoteRequest struct {
	// Up records an upvote; false withdraws it.
	Up bool `json:"up"`
}

type EditorialVoteResponse struct {
	VoteCount int  `json:"voteCount"`
	Voted     bool `json:"voted"`
}

func (request EditorialCreateRequest) Input() content.EditorialInput {
	return content.EditorialInput{
		ProblemID: request.ProblemID, Title: request.Title, ContentMD: request.ContentMD,
		Visibility: request.Visibility, Status: request.Status, SolvedOnly: request.SolvedOnly,
	}
}

func (request EditorialUpdateRequest) Input() content.EditorialInput {
	return content.EditorialInput{
		Title: request.Title, ContentMD: request.ContentMD,
		Visibility: request.Visibility, Status: request.Status, SolvedOnly: request.SolvedOnly,
	}
}

func FromEditorial(editorial content.Editorial, canEdit bool) EditorialResponse {
	return EditorialResponse{
		PublicID: editorial.PublicID, ProblemPublicID: editorial.ProblemPublicID,
		ID: editorial.ID, ProblemID: editorial.ProblemID, ProblemTitle: editorial.ProblemTitle,
		AuthorID: editorial.AuthorID, AuthorName: editorial.AuthorName,
		Title: editorial.Title, ContentMD: editorial.ContentMD,
		Visibility: editorial.Visibility, Status: editorial.Status,
		SolvedOnly: editorial.SolvedOnly, VoteCount: editorial.VoteCount,
		Voted: editorial.Voted, Locked: editorial.Locked, CanEdit: canEdit,
		CreatedAt: editorial.CreatedAt, UpdatedAt: editorial.UpdatedAt,
	}
}

func FromEditorialSummaries(
	editorials []content.EditorialSummary, viewerID string, admin bool,
) []EditorialSummaryResponse {
	result := make([]EditorialSummaryResponse, 0, len(editorials))
	for _, editorial := range editorials {
		result = append(result, EditorialSummaryResponse{
			PublicID: editorial.PublicID, ProblemPublicID: editorial.ProblemPublicID,
			ID: editorial.ID, ProblemID: editorial.ProblemID, ProblemTitle: editorial.ProblemTitle,
			AuthorID: editorial.AuthorID, AuthorName: editorial.AuthorName, Title: editorial.Title,
			Visibility: editorial.Visibility, Status: editorial.Status,
			SolvedOnly: editorial.SolvedOnly, VoteCount: editorial.VoteCount,
			Voted: editorial.Voted, Locked: editorial.Locked,
			CanEdit:   editorial.CanEdit(viewerID, admin),
			CreatedAt: editorial.CreatedAt, UpdatedAt: editorial.UpdatedAt,
		})
	}
	return result
}
