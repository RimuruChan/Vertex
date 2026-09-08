package application

import (
	"context"
	"errors"
	"fmt"

	problemdomain "github.com/RimuruChan/Vertex/server/internal/problem/domain"
	submissiondomain "github.com/RimuruChan/Vertex/server/internal/submission/domain"

	contestdomain "github.com/RimuruChan/Vertex/server/internal/contest/domain"
)

type Service struct {
	repository submissiondomain.Repository
	problems   submissiondomain.ProblemReader
	contests   submissiondomain.ContestValidator
	feedback   FeedbackReader
	limiter    submissiondomain.Limiter
	notify     func(submissionID string)
}

// NewService wires the submission domain. When the contest validator also
// knows how to answer feedback questions, contest submissions are redacted
// according to the contest's own policy.
func NewService(repository submissiondomain.Repository, problems submissiondomain.ProblemReader, contests submissiondomain.ContestValidator, limiter submissiondomain.Limiter, notify func(string)) *Service {
	service := &Service{
		repository: repository, problems: problems, contests: contests, limiter: limiter, notify: notify,
	}
	if feedback, ok := contests.(FeedbackReader); ok {
		service.feedback = feedback
	}
	return service
}

func (s *Service) Submit(ctx context.Context, userID, role string, input submissiondomain.CreateInput) (*submissiondomain.Submission, error) {
	if input.ProblemID == "" || input.Language == "" || input.SourceCode == "" {
		return nil, &submissiondomain.ValidationError{Message: "problemId, language and sourceCode are required"}
	}
	if !supportedLanguage(input.Language) {
		return nil, fmt.Errorf("%w: %s", submissiondomain.ErrUnsupportedLanguage, input.Language)
	}
	if len(input.SourceCode) > submissiondomain.MaxSourceBytes {
		return nil, submissiondomain.ErrSourceTooLarge
	}
	if s.limiter != nil && !s.limiter.Allow(userID) {
		return nil, submissiondomain.ErrRateLimited
	}

	if input.ContestID != nil {
		if s.contests == nil {
			return nil, submissiondomain.ErrContestUnavailable
		}
		if err := s.contests.ValidateSubmission(ctx, *input.ContestID, userID, role, input.ProblemID); err != nil {
			if errors.Is(err, contestdomain.ErrNotFound) {
				return nil, submissiondomain.ErrNotFound
			}
			return nil, err
		}
	} else {
		access, err := s.problems.Access(ctx, input.ProblemID, userID)
		if err != nil {
			if errors.Is(err, problemdomain.ErrNotFound) {
				return nil, submissiondomain.ErrNotFound
			}
			return nil, err
		}
		if !access.Permissions.View {
			return nil, submissiondomain.ErrProblemForbidden
		}
	}

	created, err := s.repository.Create(ctx, &submissiondomain.Submission{
		UserID: userID, ProblemID: input.ProblemID, Language: input.Language,
		SourceCode: input.SourceCode, ContestID: input.ContestID,
	})
	if err != nil {
		return nil, err
	}
	if s.notify != nil {
		s.notify(created.ID)
	}
	return created, nil
}

// List returns submissions with the contest feedback policy already applied,
// so a caller cannot forget to redact.
func (s *Service) List(ctx context.Context, filters submissiondomain.Filters, userID, role string) ([]submissiondomain.Submission, int, error) {
	items, total, err := s.repository.List(ctx, filters, submissiondomain.Viewer{UserID: userID})
	if err != nil {
		return nil, 0, err
	}
	if err := s.RedactListForViewer(ctx, items, userID, role); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Get returns one submission and whether the caller may read its source code.
// The returned submission is already redacted for the caller.
func (s *Service) Get(ctx context.Context, id, userID, role string) (*submissiondomain.Submission, bool, error) {
	item, err := s.repository.Get(ctx, id, submissiondomain.Viewer{UserID: userID})
	if err != nil {
		return nil, false, err
	}
	owned := item.UserID == userID || item.CanReadSource
	if err := s.RedactForViewer(ctx, item, userID, role); err != nil {
		return nil, false, err
	}
	return item, owned, nil
}

// Progress returns the minimal polling view through the same row visibility
// boundary as detail; it never includes source or participant metadata.
func (s *Service) Progress(ctx context.Context, id, userID, role string) (*submissiondomain.SubmissionProgress, error) {
	item, err := s.repository.Progress(ctx, id, submissiondomain.Viewer{UserID: userID})
	if err != nil {
		return nil, err
	}

	redactionTarget := item.ForRedaction()
	if err := s.RedactForViewer(ctx, &redactionTarget, userID, role); err != nil {
		return nil, err
	}
	item.ApplyRedaction(redactionTarget)
	return item, nil
}

func (s *Service) Rejudge(ctx context.Context, id string) error {
	if err := s.repository.Rejudge(ctx, id); err != nil {
		return err
	}
	if s.notify != nil {
		s.notify(id)
	}
	return nil
}

// ---------- rejudging ----------

// CreateRejudging queues a batch re-judge. The selector must restrict the set:
// an unrestricted batch would re-judge every submission ever made.
func (s *Service) CreateRejudging(ctx context.Context, selector submissiondomain.RejudgeSelector, createdBy string) (*submissiondomain.Rejudging, error) {
	if selector.IsEmpty() {
		return nil, submissiondomain.ErrRejudgeEmpty
	}
	if selector.Status != "" && !validStatus(selector.Status) {
		return nil, &submissiondomain.ValidationError{Message: "unsupported status filter: " + selector.Status}
	}
	if len(selector.SubmissionIDs) > submissiondomain.MaxRejudgeBatch {
		return nil, &submissiondomain.ValidationError{Message: "too many submissions in one rejudge batch"}
	}
	if len(selector.Reason) > 500 {
		return nil, &submissiondomain.ValidationError{Message: "reason must be at most 500 characters"}
	}
	batch, err := s.repository.CreateRejudging(ctx, selector, createdBy)
	if err != nil {
		return nil, err
	}
	if s.notify != nil {
		s.notify(batch.ID)
	}
	return batch, nil
}

func (s *Service) Rejudging(ctx context.Context, id string) (*submissiondomain.Rejudging, error) {
	return s.repository.Rejudging(ctx, id)
}

func (s *Service) ListRejudgings(ctx context.Context, contestID string, limit int) ([]submissiondomain.Rejudging, error) {
	return s.repository.ListRejudgings(ctx, contestID, limit)
}

func (s *Service) CancelRejudging(ctx context.Context, id string) error {
	return s.repository.CancelRejudging(ctx, id)
}

func (s *Service) RejudgingChanges(ctx context.Context, id string, limit int) ([]submissiondomain.RejudgingChange, error) {
	return s.repository.RejudgingChanges(ctx, id, limit)
}

// validStatus keeps a selector from silently matching nothing because of a
// typo in a verdict name.
func validStatus(status string) bool {
	switch status {
	case submissiondomain.StatusPending, submissiondomain.StatusJudging, submissiondomain.StatusAccepted, submissiondomain.StatusWrongAnswer, submissiondomain.StatusTLE, submissiondomain.StatusMLE, submissiondomain.StatusRE, submissiondomain.StatusCE, submissiondomain.StatusOLE, submissiondomain.StatusSE, submissiondomain.StatusSkipped:
		return true
	default:
		return false
	}
}

func IsContestRuleError(err error) bool {
	return errors.Is(err, contestdomain.ErrNotActive) ||
		errors.Is(err, contestdomain.ErrNotParticipant) ||
		errors.Is(err, contestdomain.ErrProblemNotInContest)
}

func supportedLanguage(language string) bool {
	switch language {
	case "cpp", "c", "python":
		return true
	default:
		return false
	}
}
