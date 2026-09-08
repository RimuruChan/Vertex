// Package domain defines the profile read model over users, submissions and problems.
// It owns no writes: every value here is derived from the domains that do.
package domain

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("profile not found")

// Profile 是个人主页展示的全部聚合数据。
type Profile struct {
	UserID          string
	Username        string
	Role            string
	Rating          int
	JoinedAt        time.Time
	SolvedCount     int
	AttemptedCount  int
	SubmissionCount int
	AcceptedCount   int
	ByDifficulty    []DifficultyProgress
	Activity        []ActivityDay
}

// DifficultyProgress 是「难度 d 下已通过 / 公开题目总数」。
type DifficultyProgress struct {
	Difficulty int
	Solved     int
	Total      int
}

// ActivityDay 是提交热力图的一天,Date 为 UTC 的 YYYY-MM-DD。
type ActivityDay struct {
	Date  string
	Count int
}

// ActivityWindowDays 限定热力图回溯范围,避免老账号返回超大数组。
const ActivityWindowDays = 90
