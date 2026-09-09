package domain

import contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"

// HiddenStatus is what a contestant sees instead of a verdict in a contest
// that withholds feedback. It is deliberately not one of the judge verdicts.
const HiddenStatus = "Submitted"

// Redact strips the parts of a judged submission that the contest's feedback
// level withholds. It never hides the submission itself: a contestant must
// always be able to see that their code arrived and what they sent.
//
//	full     nothing is hidden
//	summary  the verdict stays, per-test details and resource usage go
//	none     even the verdict is replaced by "Submitted"
func Redact(item *Submission, level string) {
	switch level {
	case "frozen":
		item.Status = StatusPending
		item.Score = 0
		item.TotalTimeMs = 0
		item.PeakMemoryKb = 0
		item.CompileResult = ""
		item.CaseResults = nil
		item.JudgedCases = 0
		item.TotalCases = 0
		item.JudgedAt = nil
		item.SourceCode = ""
	case contestdomain.FeedbackSummary:
		item.JudgedAt = nil
		item.CaseResults = nil
		item.CompileResult = ""
		item.TotalTimeMs = 0
		item.PeakMemoryKb = 0
		// Progress counters would leak how far a hidden test set got.
		item.JudgedCases = 0
		item.TotalCases = 0
	case contestdomain.FeedbackNone:
		item.JudgedAt = nil
		if isTerminal(item.Status) {
			item.Status = HiddenStatus
		}
		item.CaseResults = nil
		item.CompileResult = ""
		item.Score = 0
		item.TotalTimeMs = 0
		item.PeakMemoryKb = 0
		item.JudgedCases = 0
		item.TotalCases = 0
	}
}

// isTerminal reports whether judging has produced a verdict. Pending and
// Judging are left alone so a contestant can still tell their submission is in
// the queue rather than lost.
func isTerminal(status string) bool {
	return status != StatusPending && status != StatusJudging
}
