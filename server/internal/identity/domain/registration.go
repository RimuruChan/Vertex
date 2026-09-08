package domain

import (
	"errors"
	"regexp"
)

var (
	ErrInvalidInput       = errors.New("invalid input")
	ErrInvalidCredentials = errors.New("invalid username or password")
	usernamePattern       = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)
	emailPattern          = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
)

const (
	maxEmailBytes    = 254
	MaxPasswordBytes = 72 // bcrypt rejects longer inputs.
)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
func (e *ValidationError) Unwrap() error { return ErrInvalidInput }

func ValidateRegistration(username, email, password string) error {
	if !usernamePattern.MatchString(username) {
		return &ValidationError{Message: "username must be 3-32 chars of letters, digits or underscore"}
	}
	if len(email) > maxEmailBytes || !emailPattern.MatchString(email) {
		return &ValidationError{Message: "invalid email"}
	}
	if len(password) < 6 {
		return &ValidationError{Message: "password must be at least 6 chars"}
	}
	if len(password) > MaxPasswordBytes {
		return &ValidationError{Message: "password must be at most 72 bytes"}
	}
	return nil
}
