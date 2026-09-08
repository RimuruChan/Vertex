package domain

import "errors"

const MaxSourceBytes = 256 * 1024

var (
	ErrInvalidInput        = errors.New("invalid submission input")
	ErrUnsupportedLanguage = errors.New("unsupported language")
	ErrSourceTooLarge      = errors.New("source code too large")
	ErrRateLimited         = errors.New("submission rate limit exceeded")
	ErrProblemForbidden    = errors.New("problem not accessible")
	ErrProblemUnpublished  = errors.New("problem has no published judgeable version")
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
}
