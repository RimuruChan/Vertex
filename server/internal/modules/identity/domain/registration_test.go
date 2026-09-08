package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestRegistrationBoundaries(t *testing.T) {
	for _, test := range []struct {
		name, username, email, password string
		valid                           bool
	}{
		{"minimum", "abc", "a@example.test", "123456", true},
		{"short username", "ab", "a@example.test", "123456", false},
		{"invalid email", "alice", "invalid", "123456", false},
		{"short password", "alice", "a@example.test", "12345", false},
		{"72 bytes", "alice", "a@example.test", strings.Repeat("字", 24), true},
		{"over 72 bytes", "alice", "a@example.test", strings.Repeat("字", 25), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateRegistration(test.username, test.email, test.password)
			if test.valid && err != nil {
				t.Fatal(err)
			}
			if !test.valid && !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("expected validation error, got %v", err)
			}
		})
	}
}
