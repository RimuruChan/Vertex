package dto

import (
	"time"

	submissiondomain "github.com/RimuruChan/Vertex/server/internal/submission/domain"
)

// RejudgingResponse is one batch re-judge with derived progress.
type RejudgingResponse struct {
	ID        string  `json:"id"`
	ContestID *string `json:"contestId,omitempty"`
	ProblemID *string `json:"problemId,omitempty"`
	Reason    string  `json:"reason"`
	State     string  `json:"state" enums:"running,finished,cancelled"`
	// Total is the number of submissions the batch covers; Done counts those
	// already re-judged and Changed those whose verdict actually moved.
	Total      int        `json:"total"`
	Done       int        `json:"done"`
	Changed    int        `json:"changed"`
	CreatedAt  time.Time  `json:"createdAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

// RejudgingChangeResponse is one submission whose verdict moved.
type RejudgingChangeResponse struct {
	SubmissionID string `json:"submissionId"`
	Username     string `json:"username"`
	ProblemTitle string `json:"problemTitle"`
	PriorStatus  string `json:"priorStatus"`
	PriorScore   int    `json:"priorScore"`
	Status       string `json:"status"`
	Score        int    `json:"score"`
	Judged       bool   `json:"judged"`
}

// RejudgingCreateRequest selects the submissions to re-judge. The fields are
// combined with AND, and at least one must be present.
type RejudgingCreateRequest struct {
	ContestID     string   `json:"contestId,omitempty"`
	ProblemID     string   `json:"problemId,omitempty"`
	UserID        string   `json:"userId,omitempty"`
	Language      string   `json:"language,omitempty"`
	Status        string   `json:"status,omitempty"`
	SubmissionIDs []string `json:"submissionIds,omitempty"`
	Reason        string   `json:"reason,omitempty"`
}

func (request RejudgingCreateRequest) Selector() submissiondomain.RejudgeSelector {
	return submissiondomain.RejudgeSelector{
		ContestID: request.ContestID, ProblemID: request.ProblemID, UserID: request.UserID,
		Language: request.Language, Status: request.Status,
		SubmissionIDs: request.SubmissionIDs, Reason: request.Reason,
	}
}

func FromRejudging(value submissiondomain.Rejudging) RejudgingResponse {
	return RejudgingResponse{
		ID: value.ID, ContestID: value.ContestID, ProblemID: value.ProblemID,
		Reason: value.Reason, State: value.State, Total: value.TotalCount,
		Done: value.DoneCount, Changed: value.ChangedCount,
		CreatedAt: value.CreatedAt, FinishedAt: value.FinishedAt,
	}
}

func FromRejudgings(values []submissiondomain.Rejudging) []RejudgingResponse {
	result := make([]RejudgingResponse, 0, len(values))
	for _, value := range values {
		result = append(result, FromRejudging(value))
	}
	return result
}

func FromRejudgingChanges(values []submissiondomain.RejudgingChange) []RejudgingChangeResponse {
	result := make([]RejudgingChangeResponse, 0, len(values))
	for _, value := range values {
		result = append(result, RejudgingChangeResponse{
			SubmissionID: value.SubmissionID, Username: value.Username,
			ProblemTitle: value.ProblemTitle, PriorStatus: value.PriorStatus,
			PriorScore: value.PriorScore, Status: value.Status, Score: value.Score,
			Judged: value.Judged,
		})
	}
	return result
}
