package submission

import (
	"context"
	"errors"
	"fmt"

	"github.com/RimuruChan/Vertex/web/internal/contest"
	"github.com/RimuruChan/Vertex/web/internal/problem"
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

type Repository interface {
	Create(ctx context.Context, submission *Submission) (*Submission, error)
	List(ctx context.Context, filters Filters) ([]Submission, int, error)
	Get(ctx context.Context, id string) (*Submission, error)
	Rejudge(ctx context.Context, id string) error
}

type ProblemReader interface {
	Get(ctx context.Context, id string) (*problem.Problem, error)
}

type ContestValidator interface {
	ValidateSubmission(ctx context.Context, contestID, userID, problemID string) error
}

type Limiter interface {
	Allow(key string) bool
}

type Service struct {
	repository Repository
	problems   ProblemReader
	contests   ContestValidator
	limiter    Limiter
	notify     func(submissionID string)
}

func NewService(repository Repository, problems ProblemReader, contests ContestValidator, limiter Limiter, notify func(string)) *Service {
	return &Service{
		repository: repository, problems: problems, contests: contests, limiter: limiter, notify: notify,
	}
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
	if input.ContestID != nil {
		if s.contests == nil {
			return nil, ErrContestUnavailable
		}
		if err := s.contests.ValidateSubmission(ctx, *input.ContestID, userID, input.ProblemID); err != nil {
			if errors.Is(err, contest.ErrNotFound) {
				return nil, ErrNotFound
			}
			return nil, err
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

func (s *Service) List(ctx context.Context, filters Filters) ([]Submission, int, error) {
	return s.repository.List(ctx, filters)
}

func (s *Service) Get(ctx context.Context, id, userID, role string) (*Submission, bool, error) {
	item, err := s.repository.Get(ctx, id)
	if err != nil {
		return nil, false, err
	}
	return item, item.UserID == userID || role == "admin", nil
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
