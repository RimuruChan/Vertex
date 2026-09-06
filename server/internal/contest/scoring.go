package contest

import (
	"sort"
	"time"
)

// Contest formats. `acm` is accepted on the wire and in existing rows as a
// synonym for `icpc`; every read path normalizes through NormalizeFormat.
const (
	FormatICPC = "icpc"
	FormatIOI  = "ioi"
	FormatOI   = "oi"
)

// Feedback levels control how much of a verdict a contestant sees while the
// contest is still running.
const (
	FeedbackFull    = "full"
	FeedbackSummary = "summary"
	FeedbackNone    = "none"
)

// Staff roles inside one contest. A global administrator implicitly has jury
// rights everywhere; this table only delegates them to specific users.
const (
	StaffJury     = "jury"
	StaffObserver = "observer"
)

// NormalizeFormat maps legacy and empty values onto the supported set.
func NormalizeFormat(rule string) string {
	switch rule {
	case FormatIOI:
		return FormatIOI
	case FormatOI:
		return FormatOI
	default:
		// "acm" is the historical name for what is now icpc.
		return FormatICPC
	}
}

// Verdicts that scoring treats specially. Everything else is a plain rejection.
const (
	VerdictAccepted     = "Accepted"
	VerdictCompileError = "Compile Error"
	VerdictPending      = "Pending"
	VerdictJudging      = "Judging"
	VerdictSystemError  = "System Error"
)

// ScoringRules is the part of a contest's configuration that scoring depends on.
type ScoringRules struct {
	Format               string
	BeginAt              time.Time
	EndAt                time.Time
	FreezeAt             *time.Time
	PenaltyMinutes       int
	PenalizeCompileError bool
	// MaxPoints is the problem's full score, used by IOI and OI to clamp a
	// judge score that was reported on a 0-100 scale.
	MaxPoints int
}

// ScoredSubmission is one judged submission of a single (user, problem) pair.
// Score is the judge's 0-100 result; ICPC ignores it entirely.
type ScoredSubmission struct {
	SubmittedAt time.Time
	Status      string
	Score       int
}

// Cell is one scoreboard square. The bare fields are the jury truth; the
// Public* fields are what a frozen scoreboard is allowed to reveal. Computing
// both together is what lets the scoreboard read be a single query.
type Cell struct {
	Attempts     int
	PenaltySec   int
	Score        int
	SolvedAt     *time.Time
	LastSubmitAt *time.Time

	PublicAttempts   int
	PublicPenaltySec int
	PublicScore      int
	PublicSolvedAt   *time.Time
	// PendingCount is how many submissions landed after the freeze. A frozen
	// scoreboard renders this as "?" instead of a verdict.
	PendingCount int
}

// Solved reports whether the jury view considers the problem solved. For IOI
// and OI that means a full score, which is what "solved" means on those boards.
func (c Cell) Solved() bool { return c.SolvedAt != nil }

// ScoreCell computes one scoreboard square from a user's submissions to one
// problem. It is deliberately pure: contest scoring is the part of an OJ that
// silently corrupts standings when it is wrong, so it must be testable without
// a database.
//
// Submissions outside [BeginAt, EndAt] and submissions that are still queued or
// failed with a system error never count: neither says anything about what the
// contestant achieved.
func ScoreCell(rules ScoringRules, submissions []ScoredSubmission) Cell {
	eligible := eligibleSubmissions(rules, submissions)
	if len(eligible) == 0 {
		return Cell{}
	}

	full := scoreWindow(rules, eligible)
	cell := Cell{
		Attempts: full.attempts, PenaltySec: full.penaltySec, Score: full.score,
		SolvedAt: full.solvedAt, LastSubmitAt: full.lastSubmitAt,
	}

	if rules.FreezeAt == nil {
		// Without a freeze the two views are the same board.
		cell.PublicAttempts = full.attempts
		cell.PublicPenaltySec = full.penaltySec
		cell.PublicScore = full.score
		cell.PublicSolvedAt = full.solvedAt
		return cell
	}

	freeze := *rules.FreezeAt
	beforeFreeze := make([]ScoredSubmission, 0, len(eligible))
	pending := 0
	for _, item := range eligible {
		if item.SubmittedAt.Before(freeze) {
			beforeFreeze = append(beforeFreeze, item)
			continue
		}
		pending++
	}
	public := scoreWindow(rules, beforeFreeze)
	cell.PublicAttempts = public.attempts
	cell.PublicPenaltySec = public.penaltySec
	cell.PublicScore = public.score
	cell.PublicSolvedAt = public.solvedAt
	cell.PendingCount = pending
	return cell
}

// eligibleSubmissions filters and orders the submissions scoring may consider.
func eligibleSubmissions(rules ScoringRules, submissions []ScoredSubmission) []ScoredSubmission {
	eligible := make([]ScoredSubmission, 0, len(submissions))
	for _, item := range submissions {
		if item.SubmittedAt.Before(rules.BeginAt) || item.SubmittedAt.After(rules.EndAt) {
			continue
		}
		switch item.Status {
		case VerdictPending, VerdictJudging, VerdictSystemError, "":
			// Not a contestant-attributable outcome.
			continue
		case VerdictCompileError:
			// A contest may decide a compile error is a free retry.
			if !rules.PenalizeCompileError {
				continue
			}
		}
		eligible = append(eligible, item)
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		return eligible[i].SubmittedAt.Before(eligible[j].SubmittedAt)
	})
	return eligible
}

type windowScore struct {
	attempts     int
	penaltySec   int
	score        int
	solvedAt     *time.Time
	lastSubmitAt *time.Time
}

func scoreWindow(rules ScoringRules, eligible []ScoredSubmission) windowScore {
	if len(eligible) == 0 {
		return windowScore{}
	}
	last := eligible[len(eligible)-1].SubmittedAt
	result := windowScore{lastSubmitAt: &last}

	switch NormalizeFormat(rules.Format) {
	case FormatIOI:
		scoreIOI(rules, eligible, &result)
	case FormatOI:
		scoreOI(rules, eligible, &result)
	default:
		scoreICPC(rules, eligible, &result)
	}
	return result
}

// scoreICPC implements the ICPC rule: only the run that first solves the
// problem matters, and every rejected run *before* it costs a fixed penalty.
// Runs submitted after the first accepted one are ignored entirely.
func scoreICPC(rules ScoringRules, eligible []ScoredSubmission, result *windowScore) {
	penaltyPerAttempt := rules.PenaltyMinutes * 60
	if penaltyPerAttempt < 0 {
		penaltyPerAttempt = 0
	}

	rejected := 0
	for _, item := range eligible {
		result.attempts++
		if item.Status != VerdictAccepted {
			rejected++
			continue
		}
		solved := item.SubmittedAt
		result.solvedAt = &solved
		result.score = rules.MaxPoints
		elapsed := int(item.SubmittedAt.Sub(rules.BeginAt).Seconds())
		if elapsed < 0 {
			elapsed = 0
		}
		result.penaltySec = elapsed + rejected*penaltyPerAttempt
		return
	}
	// Never solved: attempts are recorded, but an unsolved problem contributes
	// no penalty to the standings.
	result.score = 0
}

// scoreIOI keeps the best score ever achieved, which is the IOI rule: a later
// worse submission never damages what a contestant already earned.
func scoreIOI(rules ScoringRules, eligible []ScoredSubmission, result *windowScore) {
	best := 0
	var bestAt *time.Time
	for _, item := range eligible {
		result.attempts++
		points := scaleScore(item.Score, rules.MaxPoints)
		if points > best {
			best = points
			at := item.SubmittedAt
			bestAt = &at
		}
	}
	result.score = best
	// The board treats a full score as "solved" so ICPC-style solve counts and
	// first-solver highlighting stay meaningful under IOI too.
	if best > 0 && best >= rules.MaxPoints {
		result.solvedAt = bestAt
	}
}

// scoreOI implements the OI rule: exactly one submission counts, the last one.
// Contestants get no feedback during the contest, so an earlier better attempt
// is not something they could have known to keep.
func scoreOI(rules ScoringRules, eligible []ScoredSubmission, result *windowScore) {
	result.attempts = len(eligible)
	final := eligible[len(eligible)-1]
	result.score = scaleScore(final.Score, rules.MaxPoints)
	if result.score > 0 && result.score >= rules.MaxPoints {
		at := final.SubmittedAt
		result.solvedAt = &at
	}
}

// scaleScore converts the judge's 0-100 result onto the problem's point scale.
// Rounding is toward zero so a partial result can never be inflated into a
// full score.
func scaleScore(judgeScore, maxPoints int) int {
	if judgeScore <= 0 || maxPoints <= 0 {
		return 0
	}
	if judgeScore >= 100 {
		return maxPoints
	}
	return judgeScore * maxPoints / 100
}

// RowTotals is a contestant's aggregated standing across all problems.
type RowTotals struct {
	Solved     int
	Score      int
	PenaltySec int
	// LastAcceptedAt breaks IOI/OI ties: reaching the same score earlier ranks
	// ahead.
	LastAcceptedAt *time.Time
}

// Totals aggregates one contestant's cells. jury selects which of the two
// stored views to read, so the frozen board and the jury board share this code.
func Totals(cells []Cell, jury bool) RowTotals {
	totals := RowTotals{}
	for _, cell := range cells {
		solvedAt, penalty, score := cell.PublicSolvedAt, cell.PublicPenaltySec, cell.PublicScore
		if jury {
			solvedAt, penalty, score = cell.SolvedAt, cell.PenaltySec, cell.Score
		}
		totals.Score += score
		if solvedAt == nil {
			continue
		}
		totals.Solved++
		totals.PenaltySec += penalty
		if totals.LastAcceptedAt == nil || solvedAt.After(*totals.LastAcceptedAt) {
			at := *solvedAt
			totals.LastAcceptedAt = &at
		}
	}
	return totals
}

// Less reports whether row a outranks row b under the given format. Ties are
// broken by username so the order is total and stable across requests.
func Less(format string, a, b RankRow) bool {
	switch NormalizeFormat(format) {
	case FormatIOI, FormatOI:
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		// Equal scores: whoever got there first ranks ahead. A contestant with
		// no accepted submission at all sorts last among equals.
		switch {
		case a.LastAcceptedAt == nil && b.LastAcceptedAt != nil:
			return false
		case a.LastAcceptedAt != nil && b.LastAcceptedAt == nil:
			return true
		case a.LastAcceptedAt != nil && b.LastAcceptedAt != nil &&
			!a.LastAcceptedAt.Equal(*b.LastAcceptedAt):
			return a.LastAcceptedAt.Before(*b.LastAcceptedAt)
		}
	default:
		if a.Solved != b.Solved {
			return a.Solved > b.Solved
		}
		if a.Penalty != b.Penalty {
			return a.Penalty < b.Penalty
		}
	}
	return a.Username < b.Username
}

// AssignRanks sorts rows in place and numbers them, giving tied rows the same
// rank. Ties are compared on the ranking keys only, not on the username used to
// make the sort deterministic.
func AssignRanks(format string, rows []RankRow) {
	sort.SliceStable(rows, func(i, j int) bool { return Less(format, rows[i], rows[j]) })
	for i := range rows {
		if i > 0 && tiedForRank(format, rows[i-1], rows[i]) {
			rows[i].Rank = rows[i-1].Rank
			continue
		}
		rows[i].Rank = i + 1
	}
}

func tiedForRank(format string, a, b RankRow) bool {
	switch NormalizeFormat(format) {
	case FormatIOI, FormatOI:
		return a.Score == b.Score
	default:
		return a.Solved == b.Solved && a.Penalty == b.Penalty
	}
}
