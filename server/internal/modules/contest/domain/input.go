package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidInput             = errors.New("invalid contest input")
	ErrNotActive                = errors.New("contest is not active")
	ErrNotParticipant           = errors.New("user is not registered for contest")
	ErrProblemNotInContest      = errors.New("problem is not in contest")
	ErrRegistrationClosed       = errors.New("contest registration is closed")
	ErrSelfRegistrationDisabled = errors.New("self-service registration is disabled for this contest")
	ErrInvalidPassword          = errors.New("invalid contest password")
	ErrRegistrationNeeded       = errors.New("contest registration required")
	ErrRankboardHidden          = errors.New("rankboard is hidden")
	ErrNotFound                 = errors.New("contest not found")
	ErrForbidden                = errors.New("contest forbidden")
	ErrVersionConflict          = errors.New("contest problem version changed; refresh before switching")
)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
func (e *ValidationError) Unwrap() error { return ErrInvalidInput }

// UpsertInput is the jury-facing contest configuration.
type UpsertInput struct {
	Medals                     *MedalConfig
	Admission                  string
	AllowSelfRegistration      *bool
	AllowLateRegistration      *bool
	Title                      string
	Description                string
	Rule                       string
	BeginAt                    time.Time
	EndAt                      time.Time
	FreezeAt                   *time.Time
	UnfreezeAt                 *time.Time
	PenaltyMinutes             int
	PenalizeCompileError       bool
	Feedback                   string
	Visibility                 string
	Password                   string
	RankboardVisible           bool
	ShowProblemMetadata        bool
	SubmissionVisibility       string
	SourceCodeVisibility       string
	FrozenSubmissionVisibility string
}

// PersistInput contains only values that may cross the persistence boundary.
// Plain-text contest passwords are deliberately excluded.
type PersistInput struct {
	Medals                     *MedalConfig
	Admission                  string
	AllowSelfRegistration      *bool
	AllowLateRegistration      *bool
	Title                      string
	Description                string
	Rule                       string
	BeginAt                    time.Time
	EndAt                      time.Time
	FreezeAt                   *time.Time
	UnfreezeAt                 *time.Time
	PenaltyMinutes             int
	PenalizeCompileError       bool
	Feedback                   string
	Visibility                 string
	PasswordHash               string
	RankboardVisible           bool
	ShowProblemMetadata        bool
	SubmissionVisibility       string
	SourceCodeVisibility       string
	FrozenSubmissionVisibility string
}

type Details struct {
	Contest  *Contest
	Problems []Problem
	// Staff is the caller's contest-scoped role, empty for a plain contestant.
	Staff string
}

func Invalid(message string) error { return &ValidationError{Message: message} }
