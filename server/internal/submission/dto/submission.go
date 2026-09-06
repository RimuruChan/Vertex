package dto

import (
	"time"

	"github.com/RimuruChan/Vertex/server/internal/submission"
)

type SubmissionResponse struct {
	PublicID        string               `json:"publicId"`
	ProblemPublicID string               `json:"problemPublicId"`
	ContestPublicID *string              `json:"contestPublicId,omitempty"`
	ID              string               `json:"id"`
	UserID          string               `json:"userId"`
	ProblemID       string               `json:"problemId"`
	Language        string               `json:"language"`
	SourceCode      string               `json:"sourceCode,omitempty"`
	Status          string               `json:"status"`
	Score           int                  `json:"score"`
	TotalTimeMs     int                  `json:"totalTimeMs"`
	PeakMemoryKB    int                  `json:"peakMemoryKb"`
	CompileResult   string               `json:"compileResult,omitempty"`
	CaseResults     []CaseResultResponse `json:"caseResults,omitempty"`
	JudgedCases     int                  `json:"judgedCases"`
	TotalCases      int                  `json:"totalCases"`
	ContestID       *string              `json:"contestId,omitempty"`
	SubmittedAt     time.Time            `json:"submittedAt"`
	JudgedAt        *time.Time           `json:"judgedAt,omitempty"`
	Username        string               `json:"username,omitempty"`
	ProblemTitle    string               `json:"problemTitle,omitempty"`
}

type SubmissionCreateRequest struct {
	ProblemID  string  `json:"problemId" binding:"required"`
	Language   string  `json:"language" binding:"required"`
	SourceCode string  `json:"sourceCode" binding:"required"`
	ContestID  *string `json:"contestId,omitempty"`
}

func (request SubmissionCreateRequest) CreateInput() submission.CreateInput {
	return submission.CreateInput{
		ProblemID: request.ProblemID, Language: request.Language,
		SourceCode: request.SourceCode, ContestID: request.ContestID,
	}
}

func FromSubmission(value submission.Submission, includeSource bool) SubmissionResponse {
	response := SubmissionResponse{
		PublicID: value.PublicID, ProblemPublicID: value.ProblemPublicID, ContestPublicID: value.ContestPublicID,
		ID: value.ID, UserID: value.UserID, ProblemID: value.ProblemID,
		Language: value.Language, Status: value.Status, Score: value.Score,
		TotalTimeMs: value.TotalTimeMs, PeakMemoryKB: value.PeakMemoryKb,
		CompileResult: value.CompileResult,
		JudgedCases:   value.JudgedCases, TotalCases: value.TotalCases,
		ContestID:   value.ContestID,
		SubmittedAt: value.SubmittedAt, JudgedAt: value.JudgedAt,
		Username: value.Username, ProblemTitle: value.ProblemTitle,
		CaseResults: make([]CaseResultResponse, 0, len(value.CaseResults)),
	}
	if includeSource {
		response.SourceCode = value.SourceCode
	}
	for _, item := range value.CaseResults {
		response.CaseResults = append(response.CaseResults, FromCaseResult(item))
	}
	return response
}

func FromSubmissions(values []submission.Submission) []SubmissionResponse {
	result := make([]SubmissionResponse, 0, len(values))
	for _, value := range values {
		result = append(result, FromSubmission(value, false))
	}
	return result
}
