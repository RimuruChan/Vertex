package dto

import (
	"time"

	problemdomain "github.com/RimuruChan/Vertex/server/internal/problem/domain"
)

type ProblemResponse struct {
	PublishedVersion int                `json:"publishedVersion"`
	OwnerID          string             `json:"ownerId"`
	OwnerName        string             `json:"ownerName"`
	DomainID         string             `json:"domainId"`
	Permissions      ProblemPermissions `json:"permissions"`
	// PublicID is the stable numeric reference used in URLs. ID remains the internal UUID.
	PublicID        string    `json:"publicId"`
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
	UserStatus      string    `json:"userStatus" enums:"none,attempted,solved"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type TagResponse struct {
	Name         string `json:"name"`
	ProblemCount int    `json:"problemCount"`
}

func FromTags(values []problemdomain.Tag) []TagResponse {
	result := make([]TagResponse, 0, len(values))
	for _, value := range values {
		result = append(result, TagResponse{Name: value.Name, ProblemCount: value.ProblemCount})
	}
	return result
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

func FromProblem(value problemdomain.Problem, includeStatement bool) ProblemResponse {
	tags := value.Tags
	if tags == nil {
		tags = []string{}
	}
	statement := ""
	if includeStatement {
		statement = value.StatementMD
	}
	return ProblemResponse{
		PublishedVersion: value.PublishedVersion,
		OwnerID:          value.OwnerID, OwnerName: value.OwnerName, DomainID: value.DomainID, Permissions: PermissionsFromDomain(value.Permissions),
		PublicID: value.PublicID,
		ID:       value.ID, Title: value.Title, StatementMD: statement, Difficulty: value.Difficulty,
		Source: value.Source, TimeLimitMs: value.TimeLimitMs, MemoryLimitKB: value.MemoryLimitKb,
		Visibility: value.Visibility, AuthorID: value.AuthorID, SubmissionCount: value.SubmissionCount,
		AcceptedCount: value.AcceptedCount, SolvedUserCount: value.SolvedUserCount,
		JudgeType: value.JudgeType, Tags: tags, UserStatus: userStatus(value.UserStatus),
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

// userStatus keeps the wire contract closed even when a caller builds a
// Problem without going through the service annotation step.
func userStatus(value string) string {
	switch value {
	case problemdomain.UserStatusSolved, problemdomain.UserStatusAttempted:
		return value
	default:
		return problemdomain.UserStatusNone
	}
}

func FromProblems(values []problemdomain.Problem, includeStatement bool) []ProblemResponse {
	result := make([]ProblemResponse, 0, len(values))
	for _, value := range values {
		result = append(result, FromProblem(value, includeStatement))
	}
	return result
}

func (request ProblemUpsertRequest) CreateInput() problemdomain.CreateInput {
	return problemdomain.CreateInput{
		Title: request.Title, StatementMD: request.StatementMD, Difficulty: request.Difficulty,
		Source: request.Source, TimeLimitMs: request.TimeLimitMs, MemoryLimitKb: request.MemoryLimitKB,
		Visibility: request.Visibility, Tags: request.Tags,
	}
}
