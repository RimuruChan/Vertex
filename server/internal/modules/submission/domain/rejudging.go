package domain

import (
	"errors"
	"time"
)

// Rejudging states. A batch runs until every member has been re-judged, then
// finishes; a jury may cancel it while it is still running.
const (
	RejudgingRunning   = "running"
	RejudgingFinished  = "finished"
	RejudgingCancelled = "cancelled"
)

var (
	ErrRejudgeEmpty    = errors.New("no submissions matched the rejudge selector")
	ErrRejudgeNotFound = errors.New("rejudging not found")
	ErrRejudgeClosed   = errors.New("rejudging is no longer running")
)

// RejudgeSelector describes which submissions a batch covers. The fields are
// combined with AND; an empty selector is rejected rather than interpreted as
// "everything", because re-judging an entire installation by accident is not a
// recoverable mistake.
type RejudgeSelector struct {
	ContestID     string
	ProblemID     string
	UserID        string
	Language      string
	Status        string
	SubmissionIDs []string
	Reason        string
}

// IsEmpty reports whether the selector would match without any restriction.
func (s RejudgeSelector) IsEmpty() bool {
	return s.ContestID == "" && s.ProblemID == "" && s.UserID == "" &&
		s.Language == "" && s.Status == "" && len(s.SubmissionIDs) == 0
}

// Rejudging is one batch re-judge with its progress.
type Rejudging struct {
	ID         string
	ContestID  *string
	ProblemID  *string
	Reason     string
	State      string
	TotalCount int
	// DoneCount counts members whose current judging has finished. It is
	// derived from the submissions themselves, never incremented by a worker.
	DoneCount int
	// ChangedCount counts members whose verdict differs from the one they had
	// before the batch, which is the number a jury actually cares about.
	ChangedCount int
	CreatedBy    *string
	CreatedAt    time.Time
	FinishedAt   *time.Time
}

// Finished reports whether every member has a fresh verdict.
func (r *Rejudging) Finished() bool { return r.DoneCount >= r.TotalCount }

// RejudgingChange is one member whose verdict moved.
type RejudgingChange struct {
	SubmissionID string
	Username     string
	ProblemTitle string
	PriorStatus  string
	PriorScore   int
	Status       string
	Score        int
	Judged       bool
}

// MaxRejudgeBatch bounds one explicit rejudging operation.
const MaxRejudgeBatch = 5000
