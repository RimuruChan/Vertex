package dto

import (
	"time"

	"github.com/RimuruChan/Vertex/server/internal/contest"
)

type ContestResponse struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	Description      string     `json:"description"`
	Rule             string     `json:"rule"`
	BeginAt          time.Time  `json:"beginAt"`
	EndAt            time.Time  `json:"endAt"`
	FreezeAt         *time.Time `json:"freezeAt,omitempty"`
	Visibility       string     `json:"visibility"`
	RankboardVisible bool       `json:"rankboardVisible"`
	CreatedBy        *string    `json:"createdBy,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
}

type ContestProblemResponse struct {
	ContestID  string   `json:"contestId"`
	ProblemID  string   `json:"problemId"`
	SortOrder  int      `json:"sortOrder"`
	Title      string   `json:"title"`
	Difficulty int      `json:"difficulty"`
	Visibility string   `json:"visibility"`
	Tags       []string `json:"tags"`
}

type ContestDetailsResponse struct {
	Contest  ContestResponse          `json:"contest"`
	Problems []ContestProblemResponse `json:"problems"`
}

type ContestUpsertRequest struct {
	Title            string     `json:"title" binding:"required"`
	Description      string     `json:"description,omitempty"`
	Rule             string     `json:"rule,omitempty"`
	BeginAt          time.Time  `json:"beginAt" binding:"required"`
	EndAt            time.Time  `json:"endAt" binding:"required"`
	FreezeAt         *time.Time `json:"freezeAt,omitempty"`
	Visibility       string     `json:"visibility,omitempty"`
	Password         string     `json:"password,omitempty"`
	RankboardVisible bool       `json:"rankboardVisible,omitempty"`
}

type ContestProblemsRequest struct {
	ProblemIDs []string `json:"problemIds" binding:"required"`
}

func FromContest(value contest.Contest) ContestResponse {
	return ContestResponse{
		ID: value.ID, Title: value.Title, Description: value.Description, Rule: value.Rule,
		BeginAt: value.BeginAt, EndAt: value.EndAt, FreezeAt: value.FreezeAt,
		Visibility: value.Visibility, RankboardVisible: value.RankboardVisible,
		CreatedBy: value.CreatedBy, CreatedAt: value.CreatedAt,
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
			Title: value.Title, Difficulty: value.Difficulty, Visibility: value.Visibility, Tags: value.Tags,
		})
	}
	return result
}

func (request ContestUpsertRequest) UpsertInput() contest.UpsertInput {
	return contest.UpsertInput{
		Title: request.Title, Description: request.Description, Rule: request.Rule,
		BeginAt: request.BeginAt, EndAt: request.EndAt, FreezeAt: request.FreezeAt,
		Visibility: request.Visibility, Password: request.Password,
		RankboardVisible: request.RankboardVisible,
	}
}
