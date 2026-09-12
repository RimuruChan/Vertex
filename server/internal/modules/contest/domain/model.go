package domain

import "time"

// Contest is one scheduled competition. Rule/Format selects the scoring model;
// the remaining settings are the knobs a jury tunes per contest rather than
// per deployment.
type Contest struct {
	OwnerID                    string
	OwnerName                  string
	DomainID                   string
	Admission                  string
	AllowSelfRegistration      bool
	AllowLateRegistration      bool
	Permissions                Permissions
	PublicID                   string
	ID                         string
	Title                      string
	Description                string
	Rule                       string
	BeginAt                    time.Time
	EndAt                      time.Time
	FreezeAt                   *time.Time
	UnfreezeAt                 *time.Time
	PenaltyMinutes             int
	PenalizeCompileError       bool
	Feedback                   string
	Visibility                 string
	PasswordHash               string
	RankboardVisible           bool
	ShowProblemMetadata        bool
	SubmissionVisibility       string
	SourceCodeVisibility       string
	FrozenSubmissionVisibility string
	CreatedBy                  *string
	CreatedAt                  time.Time
}

// Format returns the scoring format, defaulting an omitted rule to ICPC.
func (c *Contest) Format() string { return NormalizeFormat(c.Rule) }

// Frozen reports whether the public scoreboard hides post-freeze results at
// the given moment. An explicit unfreeze time reopens it automatically.
func (c *Contest) Frozen(now time.Time) bool {
	if c.Format() == FormatOI {
		return false
	}
	if c.FreezeAt == nil || !now.After(*c.FreezeAt) {
		return false
	}
	if c.UnfreezeAt != nil && !now.Before(*c.UnfreezeAt) {
		return false
	}
	return true
}

// Running reports whether submissions are currently accepted.
func (c *Contest) Running(now time.Time) bool {
	return !now.Before(c.BeginAt) && !now.After(c.EndAt)
}

// Ended reports whether the contest is over, which is when full feedback is
// restored regardless of the configured level.
func (c *Contest) Ended(now time.Time) bool { return now.After(c.EndAt) }

// FeedbackFor returns the feedback level that applies to a contestant right
// now. Once a contest ends everyone sees everything.
func (c *Contest) FeedbackFor(now time.Time) string {
	if c.Ended(now) {
		return FeedbackFull
	}
	if NormalizeFormat(c.Rule) == FormatOI {
		return FeedbackNone
	}
	switch c.Feedback {
	case FeedbackNone, FeedbackSummary, FeedbackFirstError:
		return c.Feedback
	default:
		return FeedbackFull
	}
}

type ProblemProgress struct {
	UserStatus       string
	LastSubmissionID string
}

// Problem is one problem as it appears inside a contest.
type Problem struct {
	UserStatus       string
	LastSubmissionID string
	Version          int
	ProblemPublicID  string
	ContestPublicID  string
	ContestID        string
	ProblemID        string
	SortOrder        int
	Label            string
	Color            string
	Points           int
	Title            string
	Difficulty       int
	Visibility       string
	Tags             []string
}

// ProblemDetail is the contest-scoped statement view. It deliberately lives
// outside the public problem read path: an unpublished problem may be opened
// by registered contestants after the round starts and by staff while they
// prepare it, without making the problem globally visible.
type ProblemDetail struct {
	Problem
	StatementMD   string
	Source        string
	TimeLimitMs   int
	MemoryLimitKB int
	JudgeType     string
}

// ProblemEntry is the jury-supplied definition of a contest problem slot.
type ProblemEntry struct {
	ProblemID string
	Label     string
	Color     string
	Points    int
}

// Staff is a delegated contest role.
type Staff struct {
	ContestID string
	UserID    string
	Username  string
	Role      string
	CreatedAt time.Time
}

// Viewer carries everything the service needs to decide what one caller may
// see: their identity, their global role, and their contest-scoped role.
type Viewer struct {
	Access *Access
	UserID string
	Role   string
	Staff  string
}

// IsAdmin reports global administrator rights.
func (v Viewer) IsAdmin() bool {
	if v.Access != nil {
		return v.Access.Scope.SiteAdmin
	}
	return v.Role == "admin"
}

// IsJury reports whether the viewer may act on the contest: rejudge, answer
// clarifications and read the unfrozen scoreboard.
func (v Viewer) IsJury() bool {
	if v.Access != nil {
		return v.Access.Permissions.Rejudge
	}
	return v.IsAdmin() || v.Staff == StaffJury
}

// IsStaff reports read access to jury views without the right to act.
func (v Viewer) IsStaff() bool {
	if v.Access != nil {
		return v.Access.Permissions.ViewJury
	}
	return v.IsJury() || v.Staff == StaffObserver
}

func (v Viewer) CanPreview() bool {
	if v.Access != nil {
		return v.Access.Permissions.PreviewProblems
	}
	return v.IsStaff()
}

// RankRow is one contestant's line on the scoreboard.
type RankRow struct {
	Rank           int
	Username       string
	UserID         string
	Solved         int
	Score          int
	Penalty        int
	LastAcceptedAt *time.Time
	Cells          []Cell
	// HasPending marks a row whose displayed state is incomplete because the
	// scoreboard is frozen.
	HasPending bool
}

// Rankboard is the whole scoreboard for one view (public or jury).
type Rankboard struct {
	Format       string
	ProblemCount int
	ProblemIDs   []string
	Problems     []Problem
	Rows         []RankRow
	Frozen       bool
	FrozenAt     *time.Time
	UnfreezeAt   *time.Time
	// FullResults selects the current cell projection, also used after public unfreeze.
	FullResults bool
	// JuryView identifies a privileged unfrozen view, not public unfreezing.
	JuryView bool
	// FirstSolvers maps a problem ID to the user ID that solved it first,
	// which the board renders as a first-blood highlight.
	FirstSolvers map[string]string
}
