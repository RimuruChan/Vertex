package submission

import (
	"context"

	"github.com/RimuruChan/Vertex/server/internal/contest"
)

// FeedbackReader tells the submission service how much a contestant is allowed
// to learn about their own submission while a contest is still running.
type FeedbackReader interface {
	Viewer(ctx context.Context, contestID, userID, role string) (contest.Viewer, error)
	Feedback(ctx context.Context, contestID string, viewer contest.Viewer) (string, error)
}

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
	case contest.FeedbackSummary:
		item.CaseResults = nil
		item.CompileResult = ""
		item.TotalTimeMs = 0
		item.PeakMemoryKb = 0
		// Progress counters would leak how far a hidden test set got.
		item.JudgedCases = 0
		item.TotalCases = 0
	case contest.FeedbackNone:
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

// feedbackFor resolves the redaction level for one submission and viewer.
// Practice submissions and staff always get the full picture.
func (s *Service) feedbackFor(ctx context.Context, item *Submission, userID, role string) (string, error) {
	if item.ContestID == nil || *item.ContestID == "" {
		return contest.FeedbackFull, nil
	}
	if s.feedback == nil {
		return "", ErrContestUnavailable
	}
	viewer, err := s.feedback.Viewer(ctx, *item.ContestID, userID, role)
	if err != nil {
		return "", err
	}
	level, err := s.feedback.Feedback(ctx, *item.ContestID, viewer)
	if err != nil {
		return "", err
	}
	return level, nil
}

// RedactForViewer applies the contest feedback policy to one submission.
func (s *Service) RedactForViewer(ctx context.Context, item *Submission, userID, role string) error {
	level, err := s.feedbackFor(ctx, item, userID, role)
	if err != nil {
		return err
	}
	Redact(item, level)
	return nil
}

// RedactListForViewer applies the policy to a list, caching the level per
// contest so a page of submissions costs one lookup per contest, not one per
// row.
func (s *Service) RedactListForViewer(ctx context.Context, items []Submission, userID, role string) error {
	levels := make(map[string]string, 2)
	for index := range items {
		item := &items[index]
		if item.ContestID == nil || *item.ContestID == "" {
			continue
		}
		level, cached := levels[*item.ContestID]
		if !cached {
			var err error
			level, err = s.feedbackFor(ctx, item, userID, role)
			if err != nil {
				return err
			}
			levels[*item.ContestID] = level
		}
		Redact(item, level)
	}
	return nil
}
