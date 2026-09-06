package submission

import (
	"context"
	"errors"
	"fmt"

	"github.com/RimuruChan/Vertex/server/internal/contest"
	"github.com/RimuruChan/Vertex/server/internal/problem"
)

const MaxSourceBytes = 256 * 1024

var (
	ErrInvalidInput        = errors.New("invalid submission input")
	ErrUnsupportedLanguage = errors.New("unsupported language")
	ErrSourceTooLarge      = errors.New("source code too large")
	ErrRateLimited         = errors.New("submission rate limit exceeded")
	ErrProblemForbidden    = errors.New("problem not accessible")
	ErrContestUnavailable  = errors.New("contest service unavailable")
	ErrNotFound            = errors.New("submission not found")
)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
func (e *ValidationError) Unwrap() error { return ErrInvalidInput }

type CreateInput struct {
	ProblemID  string
	Language   string
	SourceCode string
	ContestID  *string
}

type Filters struct {
	UserID    string
	ProblemID string
	ContestID string
	Language  string
	Status    string
	Limit     int
	Offset    int
}

// Viewer is the minimum caller identity the submission store needs to enforce
// row visibility before pagination or detail data leaves PostgreSQL.
type Viewer struct {
	UserID string
	Admin  bool
}

type Repository interface {
	Create(ctx context.Context, submission *Submission) (*Submission, error)
	List(ctx context.Context, filters Filters, viewer Viewer) ([]Submission, int, error)
	Get(ctx context.Context, id string, viewer Viewer) (*Submission, error)
	Progress(ctx context.Context, id string, viewer Viewer) (*SubmissionProgress, error)
	Rejudge(ctx context.Context, id string) error
	CreateRejudging(ctx context.Context, selector RejudgeSelector, createdBy string) (*Rejudging, error)
	Rejudging(ctx context.Context, id string) (*Rejudging, error)
	ListRejudgings(ctx context.Context, contestID string, limit int) ([]Rejudging, error)
	CancelRejudging(ctx context.Context, id string) error
	RejudgingChanges(ctx context.Context, id string, limit int) ([]RejudgingChange, error)
}

type ProblemReader interface {
	Get(ctx context.Context, id string) (*problem.Problem, error)
}

type ContestValidator interface {
	ValidateSubmission(ctx context.Context, contestID, userID, role, problemID string) error
}

type Limiter interface {
	Allow(key string) bool
}

type Service struct {
	repository Repository
	problems   ProblemReader
	contests   ContestValidator
	feedback   FeedbackReader
	limiter    Limiter
	notify     func(submissionID string)
}

// NewService wires the submission domain. When the contest validator also
// knows how to answer feedback questions, contest submissions are redacted
// according to the contest's own policy.
func NewService(repository Repository, problems ProblemReader, contests ContestValidator, limiter Limiter, notify func(string)) *Service {
	service := &Service{
		repository: repository, problems: problems, contests: contests, limiter: limiter, notify: notify,
	}
	if feedback, ok := contests.(FeedbackReader); ok {
		service.feedback = feedback
	}
	return service
}

func (s *Service) Submit(ctx context.Context, userID, role string, input CreateInput) (*Submission, error) {
	if input.ProblemID == "" || input.Language == "" || input.SourceCode == "" {
		return nil, &ValidationError{Message: "problemId, language and sourceCode are required"}
	}
	if !supportedLanguage(input.Language) {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedLanguage, input.Language)
	}
	if len(input.SourceCode) > MaxSourceBytes {
		return nil, ErrSourceTooLarge
	}
	if s.limiter != nil && !s.limiter.Allow(userID) {
		return nil, ErrRateLimited
	}

	if input.ContestID != nil {
		if s.contests == nil {
			return nil, ErrContestUnavailable
		}
		if err := s.contests.ValidateSubmission(ctx, *input.ContestID, userID, role, input.ProblemID); err != nil {
			if errors.Is(err, contest.ErrNotFound) {
				return nil, ErrNotFound
			}
			return nil, err
		}
	} else {
		problemItem, err := s.problems.Get(ctx, input.ProblemID)
		if err != nil {
			if errors.Is(err, problem.ErrNotFound) {
				return nil, ErrNotFound
			}
			return nil, err
		}
		if problemItem.Visibility != "public" && role != "admin" {
			return nil, ErrProblemForbidden
		}
	}

	created, err := s.repository.Create(ctx, &Submission{
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
func (s *Service) List(ctx context.Context, filters Filters, userID, role string) ([]Submission, int, error) {
	items, total, err := s.repository.List(ctx, filters, viewer(userID, role))
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
func (s *Service) Get(ctx context.Context, id, userID, role string) (*Submission, bool, error) {
	item, err := s.repository.Get(ctx, id, viewer(userID, role))
	if err != nil {
		return nil, false, err
	}
	owned := item.UserID == userID || role == "admin"
	if err := s.RedactForViewer(ctx, item, userID, role); err != nil {
		return nil, false, err
	}
	return item, owned, nil
}

func viewer(userID, role string) Viewer {
	return Viewer{UserID: userID, Admin: role == "admin"}
}

// Progress returns the minimal polling view through the same row visibility
// boundary as detail; it never includes source or participant metadata.
func (s *Service) Progress(ctx context.Context, id, userID, role string) (*SubmissionProgress, error) {
	item, err := s.repository.Progress(ctx, id, viewer(userID, role))
	if err != nil {
		return nil, err
	}

	redactionTarget := item.submissionForRedaction()
	if err := s.RedactForViewer(ctx, &redactionTarget, userID, role); err != nil {
		return nil, err
	}
	item.applyRedaction(redactionTarget)
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
func (s *Service) CreateRejudging(ctx context.Context, selector RejudgeSelector, createdBy string) (*Rejudging, error) {
	if selector.IsEmpty() {
		return nil, ErrRejudgeEmpty
	}
	if selector.Status != "" && !validStatus(selector.Status) {
		return nil, &ValidationError{Message: "unsupported status filter: " + selector.Status}
	}
	if len(selector.SubmissionIDs) > maxRejudgeBatch {
		return nil, &ValidationError{Message: "too many submissions in one rejudge batch"}
	}
	if len(selector.Reason) > 500 {
		return nil, &ValidationError{Message: "reason must be at most 500 characters"}
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

func (s *Service) Rejudging(ctx context.Context, id string) (*Rejudging, error) {
	return s.repository.Rejudging(ctx, id)
}

func (s *Service) ListRejudgings(ctx context.Context, contestID string, limit int) ([]Rejudging, error) {
	return s.repository.ListRejudgings(ctx, contestID, limit)
}

func (s *Service) CancelRejudging(ctx context.Context, id string) error {
	return s.repository.CancelRejudging(ctx, id)
}

func (s *Service) RejudgingChanges(ctx context.Context, id string, limit int) ([]RejudgingChange, error) {
	return s.repository.RejudgingChanges(ctx, id, limit)
}

// validStatus keeps a selector from silently matching nothing because of a
// typo in a verdict name.
func validStatus(status string) bool {
	switch status {
	case StatusPending, StatusJudging, StatusAccepted, StatusWrongAnswer,
		StatusTLE, StatusMLE, StatusRE, StatusCE, StatusOLE, StatusSE, StatusSkipped:
		return true
	default:
		return false
	}
}

func IsContestRuleError(err error) bool {
	return errors.Is(err, contest.ErrNotActive) ||
		errors.Is(err, contest.ErrNotParticipant) ||
		errors.Is(err, contest.ErrProblemNotInContest)
}

func supportedLanguage(language string) bool {
	switch language {
	case "cpp", "c", "python":
		return true
	default:
		return false
	}
}
