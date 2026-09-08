package dto

import (
	"time"

	contestdomain "github.com/RimuruChan/Vertex/server/internal/contest/domain"
)

// ClarificationResponse is one message in a contest's question channel.
type ClarificationResponse struct {
	ID          int64                   `json:"id"`
	ProblemID   *string                 `json:"problemId,omitempty"`
	ProblemName string                  `json:"problemName,omitempty"`
	ParentID    *int64                  `json:"parentId,omitempty"`
	AuthorName  string                  `json:"authorName"`
	FromJury    bool                    `json:"fromJury"`
	Announce    bool                    `json:"announce"`
	Subject     string                  `json:"subject"`
	Body        string                  `json:"body"`
	Answered    bool                    `json:"answered"`
	CreatedAt   time.Time               `json:"createdAt"`
	Replies     []ClarificationResponse `json:"replies"`
}

// ClarificationAskRequest is a contestant's question.
type ClarificationAskRequest struct {
	ProblemID *string `json:"problemId,omitempty"`
	Subject   string  `json:"subject,omitempty"`
	Body      string  `json:"body" binding:"required"`
}

// ClarificationReplyRequest is a jury answer or announcement. Leaving parentId
// and recipientId empty broadcasts to the whole contest.
type ClarificationReplyRequest struct {
	ParentID    *int64  `json:"parentId,omitempty"`
	ProblemID   *string `json:"problemId,omitempty"`
	RecipientID *string `json:"recipientId,omitempty"`
	Subject     string  `json:"subject,omitempty"`
	Body        string  `json:"body" binding:"required"`
}

func FromClarification(value contestdomain.Clarification) ClarificationResponse {
	item := ClarificationResponse{
		ID: value.ID, ProblemID: value.ProblemID, ProblemName: value.ProblemName,
		ParentID: value.ParentID, AuthorName: value.AuthorName, FromJury: value.FromJury,
		Announce: value.IsAnnouncement(), Subject: value.Subject, Body: value.Body,
		Answered: value.Answered, CreatedAt: value.CreatedAt,
		Replies: make([]ClarificationResponse, 0, len(value.Replies)),
	}
	for _, reply := range value.Replies {
		item.Replies = append(item.Replies, FromClarification(reply))
	}
	return item
}

func FromClarifications(values []contestdomain.Clarification) []ClarificationResponse {
	result := make([]ClarificationResponse, 0, len(values))
	for _, value := range values {
		result = append(result, FromClarification(value))
	}
	return result
}

func (request ClarificationAskRequest) Input(contestID, authorID string) contestdomain.ClarificationInput {
	return contestdomain.ClarificationInput{
		ContestID: contestID, ProblemID: request.ProblemID, AuthorID: authorID,
		Subject: request.Subject, Body: request.Body,
	}
}

func (request ClarificationReplyRequest) Input(contestID, authorID string) contestdomain.ClarificationInput {
	return contestdomain.ClarificationInput{
		ContestID: contestID, ProblemID: request.ProblemID, ParentID: request.ParentID,
		AuthorID: authorID, RecipientID: request.RecipientID,
		Subject: request.Subject, Body: request.Body,
	}
}
