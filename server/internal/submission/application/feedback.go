package application

import (
	"context"

	contestdomain "github.com/RimuruChan/Vertex/server/internal/contest/domain"
	submissiondomain "github.com/RimuruChan/Vertex/server/internal/submission/domain"
)

// FeedbackReader tells the submission service how much a contestant is allowed
// to learn about their own submission while a contest is still running.
type FeedbackReader interface {
	Viewer(ctx context.Context, contestID string, userID string) (contestdomain.Viewer, error)
	Feedback(ctx context.Context, contestID string, viewer contestdomain.Viewer) (string, error)
}

// feedbackFor resolves the redaction level for one submission and viewer.
// Practice submissions and staff always get the full picture.
func (s *Service) feedbackFor(ctx context.Context, item *submissiondomain.Submission, userID, role string) (string, error) {
	if item.ContestID == nil || *item.ContestID == "" {
		return contestdomain.FeedbackFull, nil
	}
	if s.feedback == nil {
		return "", submissiondomain.ErrContestUnavailable
	}
	viewer, err := s.feedback.Viewer(ctx, *item.ContestID, userID)
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
func (s *Service) RedactForViewer(ctx context.Context, item *submissiondomain.Submission, userID, role string) error {
	level, err := s.feedbackFor(ctx, item, userID, role)
	if err != nil {
		return err
	}
	submissiondomain.Redact(item, level)
	return nil
}

// RedactListForViewer applies the policy to a list, caching the level per
// contest so a page of submissions costs one lookup per contest, not one per
// row.
func (s *Service) RedactListForViewer(ctx context.Context, items []submissiondomain.Submission, userID, role string) error {
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
		submissiondomain.Redact(item, level)
	}
	return nil
}
