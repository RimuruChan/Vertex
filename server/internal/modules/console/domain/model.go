// Package domain defines the site administration surface: the dashboard read
// model, account moderation, and domain-scoped tag and announcement governance.
//
// It deliberately reads across domains instead of asking each of them to grow
// an admin API. Every value here is derived; the only writes are the
// moderation actions an administrator explicitly performs.
package domain

import (
	"errors"
	"time"
)

var (
	ErrNotFound     = errors.New("console resource not found")
	ErrInvalidInput = errors.New("invalid console input")
	ErrForbidden    = errors.New("console action not allowed")
)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
func (e *ValidationError) Unwrap() error { return ErrInvalidInput }

func Invalid(message string) error { return &ValidationError{Message: message} }

// Stats is the dashboard read model.
type Stats struct {
	Users            int
	UsersToday       int
	Problems         int
	PublicProblems   int
	Submissions      int
	SubmissionsToday int
	Contests         int
	RunningContests  int
	Editorials       int
	ProblemSets      int
	// Judge queue health. A growing queue with no running jobs usually means
	// no worker is connected, which is the first thing an operator checks.
	QueuedJobs    int
	RunningJobs   int
	DeadJobs      int
	OldestQueued  *time.Time
	ActiveWorkers int
	// VerdictBreakdown counts the last 24 hours by verdict.
	VerdictBreakdown []VerdictCount
}

type VerdictCount struct {
	Verdict string
	Count   int
}

// AccountSummary is one row of the user administration table.
type AccountSummary struct {
	ID              string
	Username        string
	Email           string
	Role            string
	Rating          int
	CreatedAt       time.Time
	DisabledAt      *time.Time
	DisabledReason  string
	SubmissionCount int
	SolvedCount     int
}

// Disabled reports whether the account is currently blocked from signing in.
func (a *AccountSummary) Disabled() bool { return a.DisabledAt != nil }

// AccountFilters narrows the user administration list.
type AccountFilters struct {
	Keyword string
	Role    string
	// OnlyDisabled restricts the list to blocked accounts.
	OnlyDisabled bool
	Limit        int
	Offset       int
}

// AccountUpdate is the set of moderation changes an administrator may apply.
// Nil fields are left untouched, which keeps a partial edit from resetting
// values the form did not show.
type AccountUpdate struct {
	Role     *string
	Rating   *int
	Disabled *bool
	Reason   string
}

// Tag is one catalogue entry with its usage count.
type Tag struct {
	ID           int64
	Name         string
	ProblemCount int
}

// Announcement is a domain notice; only published notices have a public view.
type Announcement struct {
	ID         string
	PublicID   string
	Title      string
	ContentMD  string
	Pinned     bool
	Published  bool
	AuthorName string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type AnnouncementFilters struct {
	Limit, Offset int
	Keyword       string
}

// AnnouncementInput is the editable part of an announcement.
type AnnouncementInput struct {
	Title     string
	ContentMD string
	Pinned    bool
	Published bool
}
