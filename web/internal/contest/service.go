package contest

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidInput        = errors.New("invalid contest input")
	ErrNotActive           = errors.New("contest is not active")
	ErrNotParticipant      = errors.New("user is not registered for contest")
	ErrProblemNotInContest = errors.New("problem is not in contest")
	ErrRegistrationClosed  = errors.New("contest already started")
	ErrInvalidPassword     = errors.New("invalid contest password")
	ErrRegistrationNeeded  = errors.New("contest registration required")
	ErrRankboardHidden     = errors.New("rankboard is hidden")
	ErrNotFound            = errors.New("contest not found")
	ErrForbidden           = errors.New("contest forbidden")
)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
func (e *ValidationError) Unwrap() error { return ErrInvalidInput }

type UpsertInput struct {
	Title            string
	Description      string
	Rule             string
	BeginAt          time.Time
	EndAt            time.Time
	FreezeAt         *time.Time
	Visibility       string
	Password         string
	RankboardVisible bool
}

// PersistInput contains only values that may cross the persistence boundary.
// Plain-text contest passwords are deliberately excluded.
type PersistInput struct {
	Title            string
	Description      string
	Rule             string
	BeginAt          time.Time
	EndAt            time.Time
	FreezeAt         *time.Time
	Visibility       string
	PasswordHash     string
	RankboardVisible bool
}

type Details struct {
	Contest  *Contest
	Problems []Problem
}

type ACMCell struct {
	Attempts     int
	PenaltySec   int
	SolvedAt     *time.Time
	PendingCount int
}

type RankRow struct {
	Rank         int
	Username     string
	UserID       string
	Solved       int
	Penalty      int
	Cells        []ACMCell
	HasFreezeHit bool
}

type Rankboard struct {
	ProblemCount int
	ProblemIDs   []string
	Rows         []RankRow
	Frozen       bool
	FrozenAt     *time.Time
}

type Repository interface {
	Create(ctx context.Context, createdBy string, input *PersistInput) (*Contest, error)
	Update(ctx context.Context, id string, input *PersistInput) (*Contest, error)
	List(ctx context.Context, limit, offset int) ([]Contest, int, error)
	ListAdmin(ctx context.Context, limit, offset int) ([]Contest, int, error)
	Get(ctx context.Context, id string) (*Contest, error)
	Problems(ctx context.Context, contestID string) ([]Problem, error)
	SetProblems(ctx context.Context, contestID string, problemIDs []string) error
	IsParticipant(ctx context.Context, contestID, userID string) (bool, error)
	Register(ctx context.Context, contestID, userID string) error
	HasProblem(ctx context.Context, contestID, problemID string) (bool, error)
	Rankboard(ctx context.Context, contestID string, frozen bool) (*Rankboard, error)
}

type PasswordManager interface {
	HashPassword(password string) (string, error)
	CheckPassword(hash, password string) bool
}

type Service struct {
	repository Repository
	passwords  PasswordManager
	now        func() time.Time
}

func NewService(repository Repository, passwords PasswordManager) *Service {
	return &Service{repository: repository, passwords: passwords, now: time.Now}
}

func (s *Service) List(ctx context.Context, limit, offset int, admin bool) ([]Contest, int, error) {
	if admin {
		return s.repository.ListAdmin(ctx, limit, offset)
	}
	return s.repository.List(ctx, limit, offset)
}

func (s *Service) Details(ctx context.Context, id, userID, role string, adminView bool) (*Details, error) {
	item, err := s.repository.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	isAdmin := role == "admin"
	if !adminView && item.Visibility == "private" && !isAdmin {
		return nil, ErrNotFound
	}

	problems, err := s.repository.Problems(ctx, item.ID)
	if err != nil {
		return nil, err
	}
	if !adminView && item.Visibility == "password" && !isAdmin {
		registered, err := s.isParticipant(ctx, item.ID, userID)
		if err != nil {
			return nil, err
		}
		if !registered {
			problems = nil
		}
	}
	if !adminView {
		public := make([]Problem, 0, len(problems))
		for _, problem := range problems {
			if problem.Visibility == "public" {
				public = append(public, problem)
			}
		}
		problems = public
	}
	return &Details{Contest: item, Problems: problems}, nil
}

func (s *Service) Create(ctx context.Context, createdBy string, input UpsertInput) (*Contest, error) {
	persisted, err := prepareInput(input, "", s.passwords)
	if err != nil {
		return nil, err
	}
	return s.repository.Create(ctx, createdBy, persisted)
}

func (s *Service) Update(ctx context.Context, id string, input UpsertInput) (*Contest, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	persisted, err := prepareInput(input, current.PasswordHash, s.passwords)
	if err != nil {
		return nil, err
	}
	return s.repository.Update(ctx, id, persisted)
}

func (s *Service) SetProblems(ctx context.Context, contestID string, problemIDs []string) error {
	return s.repository.SetProblems(ctx, contestID, problemIDs)
}

func (s *Service) Register(ctx context.Context, contestID, userID, role, password string) error {
	item, err := s.repository.Get(ctx, contestID)
	if err != nil {
		return err
	}
	if item.Visibility == "private" && role != "admin" {
		return ErrForbidden
	}
	if !s.now().Before(item.BeginAt) {
		return ErrRegistrationClosed
	}
	registered, err := s.repository.IsParticipant(ctx, item.ID, userID)
	if err != nil || registered {
		return err
	}
	if item.Visibility == "password" && !s.passwords.CheckPassword(item.PasswordHash, password) {
		return ErrInvalidPassword
	}
	return s.repository.Register(ctx, item.ID, userID)
}

func (s *Service) Registration(ctx context.Context, contestID, userID, role string) (bool, error) {
	item, err := s.repository.Get(ctx, contestID)
	if err != nil {
		return false, err
	}
	if item.Visibility == "private" && role != "admin" {
		return false, ErrNotFound
	}
	return s.repository.IsParticipant(ctx, item.ID, userID)
}

func (s *Service) Rankboard(ctx context.Context, contestID, userID, role string, frozenOverride *bool) (*Rankboard, error) {
	item, err := s.repository.Get(ctx, contestID)
	if err != nil {
		return nil, err
	}
	if item.Visibility == "private" && role != "admin" {
		return nil, ErrNotFound
	}
	if item.Visibility == "password" && role != "admin" {
		registered, err := s.isParticipant(ctx, item.ID, userID)
		if err != nil {
			return nil, err
		}
		if !registered {
			return nil, ErrRegistrationNeeded
		}
	}
	if !item.RankboardVisible {
		return nil, ErrRankboardHidden
	}

	frozen := item.FreezeAt != nil && s.now().After(*item.FreezeAt)
	if frozenOverride != nil {
		frozen = *frozenOverride
	}
	return s.repository.Rankboard(ctx, item.ID, frozen)
}

func (s *Service) ValidateSubmission(ctx context.Context, contestID, userID, problemID string) error {
	item, err := s.repository.Get(ctx, contestID)
	if err != nil {
		return err
	}
	now := s.now()
	if now.Before(item.BeginAt) || now.After(item.EndAt) {
		return ErrNotActive
	}
	registered, err := s.repository.IsParticipant(ctx, item.ID, userID)
	if err != nil {
		return err
	}
	if !registered {
		return ErrNotParticipant
	}
	hasProblem, err := s.repository.HasProblem(ctx, item.ID, problemID)
	if err != nil {
		return err
	}
	if !hasProblem {
		return ErrProblemNotInContest
	}
	return nil
}

func (s *Service) isParticipant(ctx context.Context, contestID, userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}
	return s.repository.IsParticipant(ctx, contestID, userID)
}

func prepareInput(input UpsertInput, existingPasswordHash string, passwords PasswordManager) (*PersistInput, error) {
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		return nil, invalid("title required")
	}
	if input.Rule == "" {
		input.Rule = "acm"
	}
	if input.Rule != "acm" {
		return nil, invalid("only ACM rule is currently supported")
	}
	if input.Visibility == "" {
		input.Visibility = "public"
	}
	if input.Visibility != "public" && input.Visibility != "private" && input.Visibility != "password" {
		return nil, invalid("invalid visibility")
	}
	if !input.EndAt.After(input.BeginAt) {
		return nil, invalid("end time must be after begin time")
	}
	if input.FreezeAt != nil && (!input.FreezeAt.After(input.BeginAt) || !input.FreezeAt.Before(input.EndAt)) {
		return nil, invalid("freeze time must be within contest time")
	}

	passwordHash := ""
	if input.Visibility == "password" {
		if input.Password == "" {
			if existingPasswordHash == "" {
				return nil, invalid("password required")
			}
			passwordHash = existingPasswordHash
		} else {
			var err error
			passwordHash, err = passwords.HashPassword(input.Password)
			if err != nil {
				return nil, err
			}
		}
	}

	return &PersistInput{
		Title: input.Title, Description: input.Description, Rule: input.Rule,
		BeginAt: input.BeginAt, EndAt: input.EndAt, FreezeAt: input.FreezeAt,
		Visibility: input.Visibility, PasswordHash: passwordHash,
		RankboardVisible: input.RankboardVisible,
	}, nil
}

func invalid(message string) error { return &ValidationError{Message: message} }
