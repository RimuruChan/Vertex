package dto

import (
	"time"

	"github.com/RimuruChan/Vertex/server/internal/problem"
)

type ProblemResponse struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	StatementMD     string    `json:"statementMd"`
	Difficulty      int       `json:"difficulty"`
	Source          string    `json:"source"`
	TimeLimitMs     int       `json:"timeLimitMs"`
	MemoryLimitKB   int       `json:"memoryLimitKb"`
	Visibility      string    `json:"visibility"`
	AuthorID        *string   `json:"authorId,omitempty"`
	SubmissionCount int       `json:"submissionCount"`
	AcceptedCount   int       `json:"acceptedCount"`
	SolvedUserCount int       `json:"solvedUserCount"`
	JudgeType       string    `json:"judgeType"`
	Tags            []string  `json:"tags"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type ProblemUpsertRequest struct {
	Title         string   `json:"title" binding:"required"`
	StatementMD   string   `json:"statementMd,omitempty"`
	Difficulty    int      `json:"difficulty,omitempty"`
	Source        string   `json:"source,omitempty"`
	TimeLimitMs   int      `json:"timeLimitMs,omitempty"`
	MemoryLimitKB int      `json:"memoryLimitKb,omitempty"`
	Visibility    string   `json:"visibility,omitempty"`
	Tags          []string `json:"tags,omitempty"`
}

func FromProblem(value problem.Problem, includeStatement bool) ProblemResponse {
	statement := ""
	if includeStatement {
		statement = value.StatementMD
	}
	return ProblemResponse{
		ID: value.ID, Title: value.Title, StatementMD: statement, Difficulty: value.Difficulty,
		Source: value.Source, TimeLimitMs: value.TimeLimitMs, MemoryLimitKB: value.MemoryLimitKb,
		Visibility: value.Visibility, AuthorID: value.AuthorID, SubmissionCount: value.SubmissionCount,
		AcceptedCount: value.AcceptedCount, SolvedUserCount: value.SolvedUserCount,
		JudgeType: value.JudgeType, Tags: value.Tags, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func FromProblems(values []problem.Problem, includeStatement bool) []ProblemResponse {
	result := make([]ProblemResponse, 0, len(values))
	for _, value := range values {
		result = append(result, FromProblem(value, includeStatement))
	}
	return result
}

func (request ProblemUpsertRequest) CreateInput() problem.CreateInput {
	return problem.CreateInput{
		Title: request.Title, StatementMD: request.StatementMD, Difficulty: request.Difficulty,
		Source: request.Source, TimeLimitMs: request.TimeLimitMs, MemoryLimitKb: request.MemoryLimitKB,
		Visibility: request.Visibility, Tags: request.Tags,
	}
}
