package dto

import (
	"time"

	"github.com/RimuruChan/Vertex/server/internal/contest"
)

type ContestResponse struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Rule        string    `json:"rule" enums:"icpc,ioi,oi"`
	BeginAt     time.Time `json:"beginAt"`
	EndAt       time.Time `json:"endAt"`
	// Format is the normalized rule; Rule may still carry the legacy "acm".
	Format               string     `json:"format" enums:"icpc,ioi,oi"`
	FreezeAt             *time.Time `json:"freezeAt,omitempty"`
	UnfreezeAt           *time.Time `json:"unfreezeAt,omitempty"`
	PenaltyMinutes       int        `json:"penaltyMinutes"`
	PenalizeCompileError bool       `json:"penalizeCompileError"`
	Feedback             string     `json:"feedback" enums:"full,summary,none"`
	Visibility           string     `json:"visibility"`
	RankboardVisible     bool       `json:"rankboardVisible"`
	CreatedBy            *string    `json:"createdBy,omitempty"`
	CreatedAt            time.Time  `json:"createdAt"`
}

type ContestProblemResponse struct {
	ContestID  string   `json:"contestId"`
	ProblemID  string   `json:"problemId"`
	SortOrder  int      `json:"sortOrder"`
	Label      string   `json:"label"`
	Color      string   `json:"color"`
	Points     int      `json:"points"`
	Title      string   `json:"title"`
	Difficulty int      `json:"difficulty"`
	Visibility string   `json:"visibility"`
	Tags       []string `json:"tags"`
}

// ContestProblemDetailResponse is the statement reached through a contest,
// including unpublished problems that the same viewer cannot open globally.
type ContestProblemDetailResponse struct {
	ContestID     string   `json:"contestId"`
	ProblemID     string   `json:"problemId"`
	SortOrder     int      `json:"sortOrder"`
	Label         string   `json:"label"`
	Color         string   `json:"color"`
	Points        int      `json:"points"`
	Title         string   `json:"title"`
	StatementMD   string   `json:"statementMd"`
	Difficulty    int      `json:"difficulty"`
	Source        string   `json:"source"`
	TimeLimitMs   int      `json:"timeLimitMs"`
	MemoryLimitKB int      `json:"memoryLimitKb"`
	Visibility    string   `json:"visibility"`
	JudgeType     string   `json:"judgeType"`
	Tags          []string `json:"tags"`
}

type ContestDetailsResponse struct {
	Contest  ContestResponse          `json:"contest"`
	Problems []ContestProblemResponse `json:"problems"`
	// StaffRole is the caller's contest role, empty for a plain contestant.
	StaffRole string `json:"staffRole"`
}

// ContestStaffResponse is one delegated jury or observer.
type ContestStaffResponse struct {
	UserID    string    `json:"userId"`
	Username  string    `json:"username"`
	Role      string    `json:"role" enums:"jury,observer"`
	CreatedAt time.Time `json:"createdAt"`
}

type ContestStaffRequest struct {
	Username string `json:"username" binding:"required"`
	Role     string `json:"role" binding:"required" enums:"jury,observer"`
}

func FromStaff(values []contest.Staff) []ContestStaffResponse {
	result := make([]ContestStaffResponse, 0, len(values))
	for _, value := range values {
		result = append(result, ContestStaffResponse{
			UserID: value.UserID, Username: value.Username,
			Role: value.Role, CreatedAt: value.CreatedAt,
		})
	}
	return result
}

type ContestUpsertRequest struct {
	Title                string     `json:"title" binding:"required"`
	Description          string     `json:"description,omitempty"`
	Rule                 string     `json:"rule,omitempty" enums:"icpc,ioi,oi"`
	BeginAt              time.Time  `json:"beginAt" binding:"required"`
	EndAt                time.Time  `json:"endAt" binding:"required"`
	FreezeAt             *time.Time `json:"freezeAt,omitempty"`
	UnfreezeAt           *time.Time `json:"unfreezeAt,omitempty"`
	PenaltyMinutes       int        `json:"penaltyMinutes,omitempty"`
	PenalizeCompileError bool       `json:"penalizeCompileError,omitempty"`
	Feedback             string     `json:"feedback,omitempty" enums:"full,summary,none"`
	Visibility           string     `json:"visibility,omitempty"`
	Password             string     `json:"password,omitempty"`
	RankboardVisible     bool       `json:"rankboardVisible,omitempty"`
}

// ContestProblemsRequest accepts either a plain ID list or per-problem jury
// metadata. The list form keeps the original simple call working.
type ContestProblemsRequest struct {
	ProblemIDs []string                     `json:"problemIds,omitempty"`
	Problems   []ContestProblemEntryRequest `json:"problems,omitempty"`
}

type ContestProblemEntryRequest struct {
	ProblemID string `json:"problemId" binding:"required"`
	Label     string `json:"label,omitempty"`
	Color     string `json:"color,omitempty"`
	Points    int    `json:"points,omitempty"`
}

// Entries normalizes both request shapes into the domain input.
func (request ContestProblemsRequest) Entries() []contest.ProblemEntry {
	if len(request.Problems) > 0 {
		entries := make([]contest.ProblemEntry, 0, len(request.Problems))
		for _, item := range request.Problems {
			entries = append(entries, contest.ProblemEntry{
				ProblemID: item.ProblemID, Label: item.Label,
				Color: item.Color, Points: item.Points,
			})
		}
		return entries
	}
	entries := make([]contest.ProblemEntry, 0, len(request.ProblemIDs))
	for _, id := range request.ProblemIDs {
		entries = append(entries, contest.ProblemEntry{ProblemID: id})
	}
	return entries
}

func FromContest(value contest.Contest) ContestResponse {
	return ContestResponse{
		ID: value.ID, Title: value.Title, Description: value.Description, Rule: value.Rule,
		Format: value.Format(), BeginAt: value.BeginAt, EndAt: value.EndAt,
		FreezeAt: value.FreezeAt, UnfreezeAt: value.UnfreezeAt,
		PenaltyMinutes: value.PenaltyMinutes, PenalizeCompileError: value.PenalizeCompileError,
		Feedback: value.Feedback, Visibility: value.Visibility,
		RankboardVisible: value.RankboardVisible,
		CreatedBy:        value.CreatedBy, CreatedAt: value.CreatedAt,
	}
}

func FromContests(values []contest.Contest) []ContestResponse {
	result := make([]ContestResponse, 0, len(values))
	for _, value := range values {
		result = append(result, FromContest(value))
	}
	return result
}

func FromContestProblems(values []contest.Problem) []ContestProblemResponse {
	result := make([]ContestProblemResponse, 0, len(values))
	for _, value := range values {
		result = append(result, ContestProblemResponse{
			ContestID: value.ContestID, ProblemID: value.ProblemID, SortOrder: value.SortOrder,
			Label: value.Label, Color: value.Color, Points: value.Points,
			Title: value.Title, Difficulty: value.Difficulty, Visibility: value.Visibility, Tags: value.Tags,
		})
	}
	return result
}

func FromContestProblemDetail(value contest.ProblemDetail) ContestProblemDetailResponse {
	return ContestProblemDetailResponse{
		ContestID: value.ContestID, ProblemID: value.ProblemID, SortOrder: value.SortOrder,
		Label: value.Label, Color: value.Color, Points: value.Points,
		Title: value.Title, StatementMD: value.StatementMD, Difficulty: value.Difficulty,
		Source: value.Source, TimeLimitMs: value.TimeLimitMs, MemoryLimitKB: value.MemoryLimitKB,
		Visibility: value.Visibility, JudgeType: value.JudgeType, Tags: value.Tags,
	}
}

func (request ContestUpsertRequest) UpsertInput() contest.UpsertInput {
	return contest.UpsertInput{
		Title: request.Title, Description: request.Description, Rule: request.Rule,
		BeginAt: request.BeginAt, EndAt: request.EndAt,
		FreezeAt: request.FreezeAt, UnfreezeAt: request.UnfreezeAt,
		PenaltyMinutes: request.PenaltyMinutes, PenalizeCompileError: request.PenalizeCompileError,
		Feedback: request.Feedback, Visibility: request.Visibility, Password: request.Password,
		RankboardVisible: request.RankboardVisible,
	}
}
