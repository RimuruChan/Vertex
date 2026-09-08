// Package domain owns tenant spaces, membership, roles and user groups.
package domain

import (
	"errors"
	"time"
)

const OfficialID = "00000000-0000-4000-8000-000000000001"
const OfficialSlug = "official"

var (
	ErrNotFound        = errors.New("domain resource not found")
	ErrForbidden       = errors.New("domain permission denied")
	ErrUnauthenticated = errors.New("authentication required")
	ErrConflict        = errors.New("domain operation conflicts with existing state")
	ErrInvalid         = errors.New("invalid domain input")
)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
func (e *ValidationError) Unwrap() error { return ErrInvalid }
func Invalid(message string) error       { return &ValidationError{Message: message} }

type Domain struct {
	ID          string
	Slug        string
	Name        string
	Description string
	OwnerID     *string
	OwnerName   string
	Official    bool
	Visibility  string
	JoinPolicy  string
	Archived    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Role struct {
	Key         string
	Name        string
	Permissions []Permission
	Builtin     bool
}

type Member struct {
	UserID   string
	Username string
	RoleKey  string
	Status   string
	JoinedAt time.Time
}

type Group struct {
	ID          string
	PublicID    string
	DomainID    string
	Name        string
	Description string
	OwnerID     string
	OwnerName   string
	MemberCount int
	ViewerRole  string
	CreatedAt   time.Time
}

type GroupMember struct {
	UserID   string
	Username string
	Role     string
}

// Scope is a freshly resolved authorization input, never a client-supplied role.
type Scope struct {
	Domain          Domain
	UserID          string
	SiteAdmin       bool
	MemberRole      string
	MemberStatus    string
	RolePermissions []Permission
}

type Filters struct {
	Keyword       string
	Limit, Offset int
}

func (f Filters) Normalized() Filters {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	return f
}

type CreateInput struct{ Slug, Name, Description, Visibility, JoinPolicy string }
type UpdateInput struct{ Name, Description, Visibility, JoinPolicy string }
type RoleInput struct {
	Key, Name   string
	Permissions []Permission
}
type MemberInput struct{ Username, RoleKey, Status string }
type GroupInput struct{ Name, Description, OwnerUsername string }
