package contest

import "time"

type Contest struct {
	ID               string
	Title            string
	Description      string
	Rule             string
	BeginAt          time.Time
	EndAt            time.Time
	FreezeAt         *time.Time
	Visibility       string
	PasswordHash     string
	RankboardVisible bool
	CreatedBy        *string
	CreatedAt        time.Time
}

type Problem struct {
	ContestID  string
	ProblemID  string
	SortOrder  int
	Title      string
	Difficulty int
	Visibility string
	Tags       []string
}
