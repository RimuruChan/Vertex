package dto

import (
	"time"

	profiledomain "github.com/RimuruChan/Vertex/server/internal/profile/domain"
)

type ProfileResponse struct {
	UserID          string                       `json:"userId"`
	Username        string                       `json:"username"`
	Role            string                       `json:"role"`
	Rating          int                          `json:"rating"`
	JoinedAt        time.Time                    `json:"joinedAt"`
	SolvedCount     int                          `json:"solvedCount"`
	AttemptedCount  int                          `json:"attemptedCount"`
	SubmissionCount int                          `json:"submissionCount"`
	AcceptedCount   int                          `json:"acceptedCount"`
	ByDifficulty    []DifficultyProgressResponse `json:"byDifficulty"`
	Activity        []ActivityDayResponse        `json:"activity"`
}

type DifficultyProgressResponse struct {
	Difficulty int `json:"difficulty"`
	Solved     int `json:"solved"`
	Total      int `json:"total"`
}

type ActivityDayResponse struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

func FromProfile(value profiledomain.Profile) ProfileResponse {
	response := ProfileResponse{
		UserID: value.UserID, Username: value.Username, Role: value.Role,
		Rating: value.Rating, JoinedAt: value.JoinedAt,
		SolvedCount: value.SolvedCount, AttemptedCount: value.AttemptedCount,
		SubmissionCount: value.SubmissionCount, AcceptedCount: value.AcceptedCount,
		ByDifficulty: make([]DifficultyProgressResponse, 0, len(value.ByDifficulty)),
		Activity:     make([]ActivityDayResponse, 0, len(value.Activity)),
	}
	for _, bucket := range value.ByDifficulty {
		response.ByDifficulty = append(response.ByDifficulty, DifficultyProgressResponse{
			Difficulty: bucket.Difficulty, Solved: bucket.Solved, Total: bucket.Total,
		})
	}
	for _, day := range value.Activity {
		response.Activity = append(response.Activity, ActivityDayResponse{Date: day.Date, Count: day.Count})
	}
	return response
}
