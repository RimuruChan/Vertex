package application

import (
	"context"

	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"

	contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	submissiondomain "github.com/RimuruChan/Vertex/server/internal/modules/submission/domain"
)

// FeedbackReader tells the submission service how much a contestant is allowed
// to learn about their own submission while a contest is still running.
type FeedbackReader interface {
	Viewer(ctx context.Context, contestID string, userID string) (contestdomain.Viewer, error)
	Feedback(ctx context.Context, contestID string, viewer contestdomain.Viewer) (string, error)
}

// feedbackFor resolves the redaction level for one submission and viewer.
// Practice submissions and staff always get the full picture.
func (s *Service) feedbackFor(ctx context.Context, item *submissiondomain.SubmissionRecord, userID, role string) (string, error) {
	if item.ContestID == nil || *item.ContestID == "" {
		return contestdomain.FeedbackFull, nil
	}
	if s.feedback == nil {
		return "", submissiondomain.ErrContestUnavailable
	}
	if !item.AsOf.IsZero() {
		ctx = tenancy.WithReadTime(ctx, item.AsOf)
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

// ViewForViewer projects facts without modifying the repository's record.
func (s *Service) ViewForViewer(ctx context.Context, item submissiondomain.SubmissionRecord, userID, role string) (submissiondomain.SubmissionView, error) {
	level, err := s.feedbackFor(ctx, &item, userID, role)
	if err != nil {
		return submissiondomain.SubmissionView{}, err
	}
	return submissiondomain.Project(item, submissiondomain.Disclosure{ReadSource: item.UserID == userID || item.CanReadSource, Feedback: level, Frozen: item.FrozenResult}), nil
}

func (s *Service) ViewsForViewer(ctx context.Context, items []submissiondomain.SubmissionRecord, userID, role string) ([]submissiondomain.SubmissionView, error) {
	levels := make(map[string]string)
	views := make([]submissiondomain.SubmissionView, 0, len(items))
	for _, item := range items {
		level := contestdomain.FeedbackFull
		if item.ContestID != nil && *item.ContestID != "" {
			var cached bool
			level, cached = levels[*item.ContestID]
			if !cached {
				var err error
				level, err = s.feedbackFor(ctx, &item, userID, role)
				if err != nil {
					return nil, err
				}
				levels[*item.ContestID] = level
			}
		}
		views = append(views, submissiondomain.Project(item, submissiondomain.Disclosure{ReadSource: false, Feedback: level, Frozen: item.FrozenResult}))
	}
	return views, nil
}
